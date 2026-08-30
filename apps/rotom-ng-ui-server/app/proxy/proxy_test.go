package proxy

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/UnownHash/RotomNG/apps/rotom-ng-ui-server/app/instances"
)

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// stubResolver returns a fixed target, or a fixed error.
type stubResolver struct {
	target instances.Target
	err    error
}

func (s stubResolver) Resolve(string) (instances.Target, error) {
	return s.target, s.err
}

// recordingResolver records the key it was asked about.
type recordingResolver struct {
	target  instances.Target
	lastKey string
}

func (r *recordingResolver) Resolve(key string) (instances.Target, error) {
	r.lastKey = key
	return r.target, nil
}

// upstreamRecorder stands in for a rotom-ng instance and captures the request
// it was sent.
type upstreamRecorder struct {
	server  *httptest.Server
	request *http.Request
	body    string
}

func newUpstream(t *testing.T) *upstreamRecorder {
	t.Helper()
	rec := &upstreamRecorder{}
	rec.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec.body = string(body)
		rec.request = r.Clone(r.Context())
		w.Header().Set("X-Upstream", "yes")
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	}))
	t.Cleanup(rec.server.Close)
	return rec
}

// newTestServer serves the proxy the way the app does: as the fallback for
// every API path no route claimed. It is a real server rather than a
// ResponseRecorder because ReverseProxy takes a different code path when the
// request context has no Done channel, which a synthetic request would not
// exercise the same way.
func newTestServer(t *testing.T, p *Proxy) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.NoRoute(p.Handler)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	return server
}

// result is the part of a response the assertions care about.
type result struct {
	status int
	header http.Header
	body   string
}

func doRequest(t *testing.T, server *httptest.Server, request *http.Request) result {
	t.Helper()
	request.URL.Scheme = "http"
	request.URL.Host = strings.TrimPrefix(server.URL, "http://")
	request.RequestURI = ""

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return result{status: response.StatusCode, header: response.Header, body: string(body)}
}

func TestHandlerProxiesToSelectedInstance(t *testing.T) {
	upstream := newUpstream(t)
	resolver := &recordingResolver{target: instances.Target{
		BaseURL:   upstream.server.URL,
		APISecret: "instance-secret",
		Instance:  "east",
	}}
	server := newTestServer(t, New(Config{Logger: testLogger(), Resolver: resolver, UserAgent: "RotomNG-UI/test"}))

	request := httptest.NewRequest(http.MethodPut, "/api/device/abc/action/reboot?include_workers=true", strings.NewReader(`{"a":1}`))
	request.Header.Set(InstanceHeader, "east")
	response := doRequest(t, server, request)

	if response.status != http.StatusTeapot {
		t.Errorf("status = %d, want %d (the upstream's own status)", response.status, http.StatusTeapot)
	}
	if got := response.header.Get("X-Upstream"); got != "yes" {
		t.Errorf("upstream response headers were not passed through: %v", response.header)
	}
	if response.body != `{"status":"ok"}` {
		t.Errorf("body = %q, want the upstream's body", response.body)
	}

	if resolver.lastKey != "east" {
		t.Errorf("resolver key = %q, want the value of the instance header", resolver.lastKey)
	}
	if upstream.request == nil {
		t.Fatal("upstream received no request")
	}
	// The /api prefix is part of the path the instance serves, so it must
	// survive the hop intact -- along with the method, query, and body.
	if got, want := upstream.request.URL.Path, "/api/device/abc/action/reboot"; got != want {
		t.Errorf("upstream path = %q, want %q", got, want)
	}
	if got, want := upstream.request.URL.RawQuery, "include_workers=true"; got != want {
		t.Errorf("upstream query = %q, want %q", got, want)
	}
	if upstream.request.Method != http.MethodPut {
		t.Errorf("upstream method = %q, want PUT", upstream.request.Method)
	}
	if upstream.body != `{"a":1}` {
		t.Errorf("upstream body = %q, want the request body", upstream.body)
	}
}

func TestHandlerSwapsCredentials(t *testing.T) {
	upstream := newUpstream(t)
	resolver := stubResolver{target: instances.Target{BaseURL: upstream.server.URL, APISecret: "instance-secret"}}
	server := newTestServer(t, New(Config{Logger: testLogger(), Resolver: resolver}))

	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	// Everything the admin UI sends to authenticate against THIS service.
	request.Header.Set("Cookie", "rotom_session=admin-token")
	request.Header.Set("Authorization", "Bearer admin-token")
	request.Header.Set("X-Rotom-Session", "1")
	request.Header.Set("X-Rotom-Secret", "admin-secret")
	request.Header.Set(InstanceHeader, "east")
	doRequest(t, server, request)

	if upstream.request == nil {
		t.Fatal("upstream received no request")
	}
	// The admin's own credentials are signed with the admin secret: upstream
	// they are useless and leaking them would widen the blast radius of a
	// compromised instance.
	for _, header := range []string{"Cookie", "Authorization", "X-Rotom-Session", InstanceHeader} {
		if got := upstream.request.Header.Get(header); got != "" {
			t.Errorf("upstream saw %s = %q, want it stripped", header, got)
		}
	}
	if got := upstream.request.Header.Get("X-Rotom-Secret"); got != "instance-secret" {
		t.Errorf("upstream saw secret %q, want the instance's own secret", got)
	}
}

func TestHandlerDropsSecretHeaderForUnsecuredInstance(t *testing.T) {
	upstream := newUpstream(t)
	resolver := stubResolver{target: instances.Target{BaseURL: upstream.server.URL}}
	server := newTestServer(t, New(Config{Logger: testLogger(), Resolver: resolver}))

	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	request.Header.Set("X-Rotom-Secret", "admin-secret")
	doRequest(t, server, request)

	if got := upstream.request.Header.Get("X-Rotom-Secret"); got != "" {
		t.Errorf("upstream saw secret %q, want none for an instance with no secret", got)
	}
}

func TestHandlerHonoursBaseURLPathPrefix(t *testing.T) {
	upstream := newUpstream(t)
	resolver := stubResolver{target: instances.Target{BaseURL: upstream.server.URL + "/rotom"}}
	server := newTestServer(t, New(Config{Logger: testLogger(), Resolver: resolver}))

	doRequest(t, server, httptest.NewRequest(http.MethodGet, "/api/status", nil))

	if got, want := upstream.request.URL.Path, "/rotom/api/status"; got != want {
		t.Errorf("upstream path = %q, want %q", got, want)
	}
}

func TestHandlerPassesEmptyKeyWhenNoInstanceHeader(t *testing.T) {
	upstream := newUpstream(t)
	resolver := &recordingResolver{target: instances.Target{BaseURL: upstream.server.URL}}
	server := newTestServer(t, New(Config{Logger: testLogger(), Resolver: resolver}))

	doRequest(t, server, httptest.NewRequest(http.MethodGet, "/api/status", nil))

	// An empty key is what tells the manager to pick the first reachable
	// instance, so a plain API client needs no instance configuration.
	if resolver.lastKey != "" {
		t.Errorf("resolver key = %q, want empty", resolver.lastKey)
	}
}

func TestHandlerRejectsUnresolvableInstances(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "unknown instance", err: instances.NewErrInstanceNotFound(), wantStatus: http.StatusNotFound},
		{name: "none configured", err: instances.NewErrNoInstances(), wantStatus: http.StatusServiceUnavailable},
		{name: "none reachable", err: instances.NewErrNoInstanceReachable(), wantStatus: http.StatusServiceUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newTestServer(t, New(Config{
				Logger:   testLogger(),
				Resolver: stubResolver{err: test.err},
			}))

			response := doRequest(t, server, httptest.NewRequest(http.MethodGet, "/api/status", nil))

			if response.status != test.wantStatus {
				t.Errorf("status = %d, want %d", response.status, test.wantStatus)
			}
			if !strings.Contains(response.body, test.err.Error()) {
				t.Errorf("body = %q, want it to explain %q", response.body, test.err)
			}
		})
	}
}

func TestHandlerReportsUpstreamFailureAsBadGateway(t *testing.T) {
	// A URL that parses but cannot be dialled: the instance is configured but
	// is not answering.
	resolver := stubResolver{target: instances.Target{BaseURL: "http://127.0.0.1:1"}}
	server := newTestServer(t, New(Config{Logger: testLogger(), Resolver: resolver}))

	response := doRequest(t, server, httptest.NewRequest(http.MethodGet, "/api/status", nil))

	if response.status != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", response.status, http.StatusBadGateway)
	}
}

func TestHandlerRejectsUnusableBaseURL(t *testing.T) {
	resolver := stubResolver{target: instances.Target{BaseURL: "http://[::1]:namedport"}}
	server := newTestServer(t, New(Config{Logger: testLogger(), Resolver: resolver}))

	response := doRequest(t, server, httptest.NewRequest(http.MethodGet, "/api/status", nil))

	if response.status != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", response.status, http.StatusBadGateway)
	}
}

func TestHandlerPropagatesResolverErrorsVerbatim(t *testing.T) {
	// An error that is none of the three known kinds still has to produce a
	// response rather than a panic or an empty body.
	server := newTestServer(t, New(Config{
		Logger:   testLogger(),
		Resolver: stubResolver{err: errors.New("something else went wrong")},
	}))

	response := doRequest(t, server, httptest.NewRequest(http.MethodGet, "/api/status", nil))

	if response.status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", response.status, http.StatusServiceUnavailable)
	}
	if !strings.Contains(response.body, "something else went wrong") {
		t.Errorf("body = %q", response.body)
	}
}
