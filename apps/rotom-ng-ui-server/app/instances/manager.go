// Package instances tracks the rotom-ng servers this service fronts: their
// reachability, the configuration each one reports, and which upstream a
// request should be sent to.
package instances

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/UnownHash/RotomNG/libs/auth"
)

// APIPathPrefix is appended to an instance's configured base URL to reach its
// REST API. Every upstream request is built as base_url + APIPathPrefix + rest.
const APIPathPrefix = "/api"

// configPath is the endpoint probed for reachability. It doubles as the source
// of the instance name and of the configuration the UI gates its features on,
// so one request keeps everything current.
const configPath = APIPathPrefix + "/config"

var errNoInstances = errors.New("no instances are configured")

// NewErrNoInstances returns the error reported when the service has no
// instances configured at all.
func NewErrNoInstances() error { return errNoInstances }

// IsErrNoInstances reports whether err indicates no configured instances.
func IsErrNoInstances(err error) bool { return errors.Is(err, errNoInstances) }

var errInstanceNotFound = errors.New("instance not found")

// NewErrInstanceNotFound returns the error reported when a request names an
// instance this service does not have.
func NewErrInstanceNotFound() error { return errInstanceNotFound }

// IsErrInstanceNotFound reports whether err indicates an unknown instance.
func IsErrInstanceNotFound(err error) bool { return errors.Is(err, errInstanceNotFound) }

var errNoInstanceReachable = errors.New("no instance is reachable")

// NewErrNoInstanceReachable returns the error reported when a request did not
// name an instance and none of the configured ones are currently reachable.
func NewErrNoInstanceReachable() error { return errNoInstanceReachable }

// IsErrNoInstanceReachable reports whether err indicates nothing reachable.
func IsErrNoInstanceReachable(err error) bool { return errors.Is(err, errNoInstanceReachable) }

// InstanceConfig is one configured upstream.
type InstanceConfig struct {
	BaseURL   string
	APISecret string
}

// Settings are the parts of the manager's configuration that a config reload
// can change.
type Settings struct {
	Instances []InstanceConfig
	Interval  time.Duration
	Timeout   time.Duration
}

// Validate validates the settings.
func (s Settings) Validate() error {
	if s.Interval <= 0 {
		return errors.New("instance monitor interval must be positive")
	}
	if s.Timeout <= 0 {
		return errors.New("instance monitor timeout must be positive")
	}
	seen := make(map[string]struct{}, len(s.Instances))
	for _, instance := range s.Instances {
		if instance.BaseURL == "" {
			return errors.New("instance base url must not be empty")
		}
		if _, dup := seen[instance.BaseURL]; dup {
			return fmt.Errorf("duplicate instance base url %q", instance.BaseURL)
		}
		seen[instance.BaseURL] = struct{}{}
	}
	return nil
}

// State is the public view of one instance, as reported by GET /api/config.
type State struct {
	// Instance is the name the upstream reports in its own config. Empty when
	// that instance has no instance name set, or has not been reached yet.
	Instance string `json:"instance"`
	// URL is the configured base URL. It is this service's stable identifier
	// for the instance: it is what a client sends back to select one.
	URL string `json:"url"`
	// Reachable is true only while the most recent probe succeeded, which
	// implies Config is populated.
	Reachable bool `json:"reachable"`
	// Config is the config object from the instance's last successful
	// /api/config response, passed through verbatim so the UI reads exactly
	// what it would read when pointed at that instance directly. Retained
	// while the instance is unreachable, and omitted until first contact.
	Config json.RawMessage `json:"config,omitempty"`
}

// Target is where a proxied request should be sent.
type Target struct {
	// BaseURL is the instance root, without the /api prefix.
	BaseURL string
	// APISecret is sent upstream as X-Rotom-Secret; empty when unset.
	APISecret string
	// Instance is the upstream's reported name, for logging.
	Instance string
}

// instanceState is the manager's private record for one upstream.
type instanceState struct {
	config    InstanceConfig
	name      string
	reachable bool
	rawConfig json.RawMessage
}

// ManagerConfig holds the dependencies for a Manager.
type ManagerConfig struct {
	Logger *slog.Logger
	// HTTPClient probes instances. Defaults to a client with no global
	// timeout: each probe carries its own deadline via the request context,
	// which is what the configured timeout adjusts.
	HTTPClient *http.Client
	// UserAgent identifies this service to the instances it probes.
	UserAgent string
}

// Manager tracks instance reachability and resolves proxy targets.
type Manager struct {
	logger     *slog.Logger
	httpClient *http.Client
	userAgent  string

	mu       sync.RWMutex
	settings Settings
	// order holds base URLs in configuration order, so the list the UI renders
	// matches the order the operator wrote them in.
	order []string
	byURL map[string]*instanceState

	// changed is closed and replaced whenever a probe round alters any
	// instance's reachability, so callers can wait for a settled state
	// without polling.
	changed chan struct{}
}

// NewManager creates a Manager with the given settings.
func NewManager(cfg ManagerConfig, settings Settings) (*Manager, error) {
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	manager := &Manager{
		logger:     cfg.Logger,
		httpClient: httpClient,
		userAgent:  cfg.UserAgent,
		byURL:      make(map[string]*instanceState),
		changed:    make(chan struct{}),
	}
	manager.SetSettings(settings)
	return manager, nil
}

// SetSettings replaces the instance list and monitor timings. Instances that
// survive the change keep their cached name, config, and reachability, so a
// reload that only adds or removes entries does not blank out the UI for the
// ones it left alone.
func (m *Manager) SetSettings(settings Settings) {
	m.mu.Lock()
	defer m.mu.Unlock()

	order := make([]string, 0, len(settings.Instances))
	byURL := make(map[string]*instanceState, len(settings.Instances))
	for _, instanceCfg := range settings.Instances {
		state := m.byURL[instanceCfg.BaseURL]
		if state == nil {
			state = &instanceState{}
		}
		// The secret may have been rotated in the config file; the cached
		// name and config stay valid either way.
		state.config = instanceCfg
		order = append(order, instanceCfg.BaseURL)
		byURL[instanceCfg.BaseURL] = state
	}

	m.settings = settings
	m.order = order
	m.byURL = byURL
}

// Snapshot returns the current state of every configured instance, in
// configuration order.
func (m *Manager) Snapshot() []State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make([]State, 0, len(m.order))
	for _, baseURL := range m.order {
		state := m.byURL[baseURL]
		states = append(states, State{
			Instance:  state.name,
			URL:       baseURL,
			Reachable: state.reachable,
			Config:    state.rawConfig,
		})
	}
	return states
}

// Resolve picks the upstream for a request.
//
// key is the client's instance selection: a base URL, or an instance name for
// clients that find that more convenient. Base URLs are matched first because
// they are guaranteed unique, while two instances can report the same name.
//
// An empty key selects the first reachable instance, which is what lets an
// API client that does not care about instance selection work unconfigured.
func (m *Manager) Resolve(key string) (Target, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.order) == 0 {
		return Target{}, errNoInstances
	}

	if key == "" {
		for _, baseURL := range m.order {
			if state := m.byURL[baseURL]; state.reachable {
				return targetFor(state), nil
			}
		}
		return Target{}, errNoInstanceReachable
	}

	if state, ok := m.byURL[key]; ok {
		return targetFor(state), nil
	}
	for _, baseURL := range m.order {
		if state := m.byURL[baseURL]; state.name == key {
			return targetFor(state), nil
		}
	}
	return Target{}, fmt.Errorf("%w: %q", errInstanceNotFound, key)
}

func targetFor(state *instanceState) Target {
	return Target{
		BaseURL:   state.config.BaseURL,
		APISecret: state.config.APISecret,
		Instance:  state.name,
	}
}

// Changed returns a channel closed the next time a probe round changes any
// instance's reachability.
func (m *Manager) Changed() <-chan struct{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.changed
}

// Run probes every instance immediately, then once per configured interval,
// until ctx is cancelled.
func (m *Manager) Run(ctx context.Context) {
	for {
		m.probeAll(ctx)

		m.mu.RLock()
		interval := m.settings.Interval
		m.mu.RUnlock()

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// probeAll probes all instances concurrently and returns once they have all
// settled. Probing in parallel keeps one unreachable instance from delaying
// the reachability updates of the rest.
func (m *Manager) probeAll(ctx context.Context) {
	m.mu.RLock()
	timeout := m.settings.Timeout
	targets := make([]InstanceConfig, 0, len(m.order))
	for _, baseURL := range m.order {
		targets = append(targets, m.byURL[baseURL].config)
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, target := range targets {
		wg.Go(func() {
			m.probe(ctx, target, timeout)
		})
	}
	wg.Wait()
}

func (m *Manager) probe(ctx context.Context, instanceCfg InstanceConfig, timeout time.Duration) {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	name, rawConfig, err := m.fetchConfig(probeCtx, instanceCfg)
	if err != nil {
		// A cancelled context means we are shutting down, not that the
		// instance went away; leaving the state untouched avoids a spurious
		// "unreachable" log line on every restart.
		if ctx.Err() != nil {
			return
		}
		m.setUnreachable(ctx, instanceCfg.BaseURL, err)
		return
	}
	m.setReachable(ctx, instanceCfg.BaseURL, name, rawConfig)
}

// configEnvelope is the shape of a rotom-ng GET /api/config reply. The config
// object is kept raw so this service does not have to track every field
// rotom-ng may add to it.
type configEnvelope struct {
	Config json.RawMessage `json:"config"`
}

// fetchConfig retrieves an instance's config, returning its instance name and
// the raw config object.
func (m *Manager) fetchConfig(ctx context.Context, instanceCfg InstanceConfig) (string, json.RawMessage, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, instanceCfg.BaseURL+configPath, nil)
	if err != nil {
		return "", nil, fmt.Errorf("build config request: %w", err)
	}
	if instanceCfg.APISecret != "" {
		request.Header.Set(auth.SecretRequestHeader, instanceCfg.APISecret)
	}
	if m.userAgent != "" {
		request.Header.Set("User-Agent", m.userAgent)
	}

	response, err := m.httpClient.Do(request)
	if err != nil {
		return "", nil, fmt.Errorf("request config: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("config request returned %s", response.Status)
	}

	var envelope configEnvelope
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return "", nil, fmt.Errorf("decode config response: %w", err)
	}
	if len(envelope.Config) == 0 {
		return "", nil, errors.New("config response has no config object")
	}

	// The instance name is optional upstream: rotom-ng omits it entirely when
	// unset, so an absent field is normal rather than an error.
	var named struct {
		Instance string `json:"instance"`
	}
	if err := json.Unmarshal(envelope.Config, &named); err != nil {
		return "", nil, fmt.Errorf("decode config object: %w", err)
	}

	return named.Instance, envelope.Config, nil
}

func (m *Manager) setReachable(ctx context.Context, baseURL, name string, rawConfig json.RawMessage) {
	m.mu.Lock()
	state, ok := m.byURL[baseURL]
	if !ok {
		// Removed by a config reload while the probe was in flight.
		m.mu.Unlock()
		return
	}
	wasReachable := state.reachable
	previousName := state.name
	state.reachable = true
	state.name = name
	state.rawConfig = rawConfig
	if !wasReachable || previousName != name {
		m.signalChangedLocked()
	}
	m.mu.Unlock()

	if !wasReachable {
		m.logger.LogAttrs(ctx, slog.LevelInfo, "instance is reachable",
			slog.String("url", baseURL), slog.String("instance", name))
	}
}

func (m *Manager) setUnreachable(ctx context.Context, baseURL string, cause error) {
	m.mu.Lock()
	state, ok := m.byURL[baseURL]
	if !ok {
		m.mu.Unlock()
		return
	}
	wasReachable := state.reachable
	name := state.name
	state.reachable = false
	if wasReachable {
		m.signalChangedLocked()
	}
	m.mu.Unlock()

	// Only the transition is logged at warn: an instance that is down stays
	// down, and one line per probe interval would drown the log.
	if wasReachable {
		m.logger.LogAttrs(ctx, slog.LevelWarn, "instance is unreachable",
			slog.String("url", baseURL), slog.String("instance", name), slog.String("error", cause.Error()))
	} else {
		m.logger.LogAttrs(ctx, slog.LevelDebug, "instance probe failed",
			slog.String("url", baseURL), slog.String("error", cause.Error()))
	}
}

// signalChangedLocked wakes everything waiting on Changed. Callers hold m.mu.
func (m *Manager) signalChangedLocked() {
	close(m.changed)
	m.changed = make(chan struct{})
}
