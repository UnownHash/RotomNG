package prom

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNamespaceCollector(t *testing.T) {
	// Create a simple counter to test with
	counter := prometheus.NewCounter(prometheus.CounterOpts{ //nolint:promlinter
		Name: "test_counter",
		Help: "A test counter",
	})
	counter.Inc()

	// Create a gauge to test with
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{ //nolint:promlinter
		Name: "test_gauge",
		Help: "A test gauge",
	})
	gauge.Set(42)

	// Create a registry and register the original metrics
	originalRegistry := prometheus.NewRegistry()
	originalRegistry.MustRegister(counter, gauge)

	// Create a namespaced collector
	namespacedCollector := NewNamespaceCollector("myapp", originalRegistry)

	// Create a new registry for the namespaced collector
	namespacedRegistry := prometheus.NewRegistry()
	namespacedRegistry.MustRegister(namespacedCollector)

	// Gather metrics from the namespaced registry
	metricFamilies, err := namespacedRegistry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Check that we have the expected metrics with namespace
	expectedMetrics := map[string]bool{
		"myapp_test_counter": false,
		"myapp_test_gauge":   false,
	}

	for _, mf := range metricFamilies {
		name := mf.GetName()
		if _, exists := expectedMetrics[name]; exists {
			expectedMetrics[name] = true
			t.Logf("Found expected metric: %s", name)
		} else {
			t.Logf("Found unexpected metric: %s", name)
		}
	}

	// Verify all expected metrics were found
	for name, found := range expectedMetrics {
		if !found {
			t.Errorf("Expected metric %s not found", name)
		}
	}

	// Test the actual metric values
	expected := `
# HELP myapp_test_counter A test counter
# TYPE myapp_test_counter counter
myapp_test_counter 1
# HELP myapp_test_gauge A test gauge
# TYPE myapp_test_gauge gauge
myapp_test_gauge 42
`
	if err := testutil.GatherAndCompare(namespacedRegistry, strings.NewReader(expected)); err != nil {
		t.Errorf("Metric comparison failed: %v", err)
	}
}

func TestNamespaceCollectorWithExistingNamespace(t *testing.T) {
	// Create a counter that already has a namespace
	counter := prometheus.NewCounter(prometheus.CounterOpts{ //nolint:promlinter
		Namespace: "existing",
		Name:      "test_counter",
		Help:      "A test counter with existing namespace",
	})
	counter.Inc()

	// Create a registry and register the original metric
	originalRegistry := prometheus.NewRegistry()
	originalRegistry.MustRegister(counter)

	// Create a namespaced collector
	namespacedCollector := NewNamespaceCollector("myapp", originalRegistry)

	// Create a new registry for the namespaced collector
	namespacedRegistry := prometheus.NewRegistry()
	namespacedRegistry.MustRegister(namespacedCollector)

	// Gather metrics from the namespaced registry
	metricFamilies, err := namespacedRegistry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// The metric should now have the new namespace prepended
	found := false
	for _, mf := range metricFamilies {
		name := mf.GetName()
		if name == "myapp_existing_test_counter" {
			found = true
			break
		}
		t.Logf("Found metric: %s", name)
	}

	if !found {
		t.Error("Expected metric myapp_existing_test_counter not found")
	}
}

func TestNamespaceCollectorEmpty(t *testing.T) {
	// Create an empty registry
	emptyRegistry := prometheus.NewRegistry()

	// Create a namespaced collector
	namespacedCollector := NewNamespaceCollector("myapp", emptyRegistry)

	// Create a new registry for the namespaced collector
	namespacedRegistry := prometheus.NewRegistry()
	namespacedRegistry.MustRegister(namespacedCollector)

	// Gather metrics from the namespaced registry
	metricFamilies, err := namespacedRegistry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Should have no metrics
	if len(metricFamilies) != 0 {
		t.Errorf("Expected no metrics, got %d", len(metricFamilies))
	}
}
