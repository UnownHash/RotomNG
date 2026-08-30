// Package proxy forwards API requests to the rotom-ng instance the caller
// selected.
package proxy

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/UnownHash/RotomNG/libs/auth"

	"github.com/UnownHash/RotomNG/apps/rotom-ng-ui-server/app/instances"
)

// InstanceHeader names the instance a request is for. Its value is that
// instance's configured base URL -- unique by construction -- though an
// instance name is accepted too. Absent, the first reachable instance is used,
// so a plain API client that does not care which instance answers still works.
const InstanceHeader = "X-Rotom-Instance"

// Log/response field keys.
const (
	fieldStatus = "status"
	fieldError  = "error"
	statusError = "error"
)

// Resolver picks the upstream for an instance key.
type Resolver interface {
	Resolve(key string) (instances.Target, error)
}

// Config holds the dependencies for a Proxy.
type Config struct {
	Logger   *slog.Logger
	Resolver Resolver
	// UserAgent identifies this service to the instances it proxies to. Left
	// empty, the caller's own User-Agent is passed through untouched.
	UserAgent string
	// Transport is the round tripper used for upstream requests. Defaults to
	// http.DefaultTransport.
	Transport http.RoundTripper
}

// targetContextKey types the context value carrying the resolved target from
// the gin handler into the ReverseProxy rewrite hook.
type targetContextKey struct{}

// Proxy reverse-proxies API requests to a selected rotom-ng instance.
type Proxy struct {
	logger   *slog.Logger
	resolver Resolver
	reverse  *httputil.ReverseProxy
}

// New creates a Proxy.
func New(cfg Config) *Proxy {
	p := &Proxy{
		logger:   cfg.Logger,
		resolver: cfg.Resolver,
	}
	p.reverse = &httputil.ReverseProxy{
		Transport: cfg.Transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			rewrite(pr, cfg.UserAgent)
		},
		ErrorHandler: p.handleUpstreamError,
	}
	return p
}

// Handler proxies the current request to the selected instance.
//
// It is installed as the web server's API fallback, so it sees every /api
// path this service does not serve itself. That is deliberate: rotom-ng can
// grow endpoints without this service needing to learn about them.
func (p *Proxy) Handler(c *gin.Context) {
	key := c.GetHeader(InstanceHeader)

	target, err := p.resolver.Resolve(key)
	if err != nil {
		p.rejectUnresolved(c, key, err)
		return
	}

	upstream, err := url.Parse(target.BaseURL)
	if err != nil {
		// Base URLs are validated at config load, so this is not reachable
		// from a valid configuration -- but a 502 beats a panic if it ever is.
		p.logger.LogAttrs(c.Request.Context(), slog.LevelError, "instance base url is unusable",
			slog.String("url", target.BaseURL), slog.String(fieldError, err.Error()))
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			fieldStatus: statusError,
			fieldError:  "instance base url is unusable",
		})
		return
	}

	resolved := &resolvedTarget{upstream: upstream, secret: target.APISecret}
	ctx := context.WithValue(c.Request.Context(), targetContextKey{}, resolved)
	p.reverse.ServeHTTP(c.Writer, c.Request.WithContext(ctx))

	p.logger.LogAttrs(c.Request.Context(), slog.LevelDebug, "proxied request",
		slog.String("method", c.Request.Method),
		slog.String("path", c.Request.URL.Path),
		slog.String("url", target.BaseURL),
		slog.String("instance", target.Instance),
		slog.Int(fieldStatus, c.Writer.Status()),
	)
}

// resolvedTarget is what rewrite needs, precomputed by the handler.
type resolvedTarget struct {
	upstream *url.URL
	secret   string
}

// rejectUnresolved answers a request that names no usable instance. The three
// causes are distinguished because they mean different things to an operator:
// nothing configured, a name that does not exist, or everything down.
func (p *Proxy) rejectUnresolved(c *gin.Context, key string, err error) {
	status := http.StatusServiceUnavailable
	if instances.IsErrInstanceNotFound(err) {
		status = http.StatusNotFound
	}
	p.logger.LogAttrs(c.Request.Context(), slog.LevelWarn, "cannot route request to an instance",
		slog.String("path", c.Request.URL.Path),
		slog.String("requested_instance", key),
		slog.String(fieldError, err.Error()),
	)
	c.AbortWithStatusJSON(status, gin.H{fieldStatus: statusError, fieldError: err.Error()})
}

func (p *Proxy) handleUpstreamError(w http.ResponseWriter, r *http.Request, err error) {
	// A cancelled client connection is not an upstream failure and writing to
	// the (gone) response writer would only add noise.
	if r.Context().Err() != nil {
		return
	}
	p.logger.LogAttrs(r.Context(), slog.LevelWarn, "upstream request failed",
		slog.String("method", r.Method),
		slog.String("url", r.URL.String()),
		slog.String(fieldError, err.Error()),
	)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusBadGateway)
	// Errors from the request itself are not worth handling: the connection
	// is already in a bad way and there is nothing left to fall back to.
	_, _ = w.Write([]byte(`{"status":"error","error":"instance request failed"}`))
}

// rewrite retargets the outbound request at the selected instance and strips
// the credentials that belong to this service rather than to the instance.
func rewrite(pr *httputil.ProxyRequest, userAgent string) {
	resolved, ok := pr.In.Context().Value(targetContextKey{}).(*resolvedTarget)
	if !ok {
		// Handler always sets the value, so reaching this means a programming
		// error rather than a bad request; leaving the URL alone makes the
		// transport fail loudly into ErrorHandler.
		return
	}

	pr.Out.URL.Scheme = resolved.upstream.Scheme
	pr.Out.URL.Host = resolved.upstream.Host
	// A base URL may carry a path prefix when rotom-ng sits behind a
	// path-routing proxy; the incoming path (already /api/...) hangs off it.
	pr.Out.URL.Path = strings.TrimRight(resolved.upstream.Path, "/") + pr.In.URL.Path
	pr.Out.URL.RawPath = ""
	pr.Out.Host = resolved.upstream.Host

	pr.SetXForwarded()

	// This service's own credentials must not travel upstream: the session
	// cookie and bearer token are signed with the admin secret and are
	// meaningless -- but still sensitive -- to an instance. The instance's own
	// secret is what authenticates us.
	pr.Out.Header.Del("Cookie")
	pr.Out.Header.Del("Authorization")
	pr.Out.Header.Del(auth.SessionRequestHeader)
	pr.Out.Header.Del(InstanceHeader)

	if resolved.secret == "" {
		pr.Out.Header.Del(auth.SecretRequestHeader)
	} else {
		pr.Out.Header.Set(auth.SecretRequestHeader, resolved.secret)
	}

	if userAgent != "" {
		pr.Out.Header.Set("User-Agent", userAgent)
	}
}
