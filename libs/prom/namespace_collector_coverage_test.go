package prom

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNamespaceCollector_Histogram(t *testing.T) {
	hist := prometheus.NewHistogram(prometheus.HistogramOpts{Name: "request_duration",
		Help:    "Request duration in seconds",
		Buckets: []float64{0.1, 0.5, 1.0, 5.0},
	})
	hist.Observe(0.3)
	hist.Observe(0.7)
	hist.Observe(2.0)

	originalRegistry := prometheus.NewRegistry()
	originalRegistry.MustRegister(hist)

	namespacedCollector := NewNamespaceCollector("myapp", originalRegistry)
	namespacedRegistry := prometheus.NewRegistry()
	namespacedRegistry.MustRegister(namespacedCollector)

	metricFamilies, err := namespacedRegistry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "myapp_request_duration" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected metric myapp_request_duration not found")
	}

	expected := `
# HELP myapp_request_duration Request duration in seconds
# TYPE myapp_request_duration histogram
myapp_request_duration_bucket{le="0.1"} 0
myapp_request_duration_bucket{le="0.5"} 1
myapp_request_duration_bucket{le="1"} 2
myapp_request_duration_bucket{le="5"} 3
myapp_request_duration_bucket{le="+Inf"} 3
myapp_request_duration_sum 3
myapp_request_duration_count 3
`
	if err := testutil.GatherAndCompare(namespacedRegistry, strings.NewReader(expected)); err != nil {
		t.Errorf("Histogram comparison failed: %v", err)
	}
}

func TestNamespaceCollector_Summary(t *testing.T) {
	summary := prometheus.NewSummary(prometheus.SummaryOpts{Name: "response_size",
		Help:       "Response size in bytes",
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	})
	summary.Observe(100)
	summary.Observe(200)
	summary.Observe(300)

	originalRegistry := prometheus.NewRegistry()
	originalRegistry.MustRegister(summary)

	namespacedCollector := NewNamespaceCollector("myapp", originalRegistry)
	namespacedRegistry := prometheus.NewRegistry()
	namespacedRegistry.MustRegister(namespacedCollector)

	metricFamilies, err := namespacedRegistry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "myapp_response_size" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected metric myapp_response_size not found")
	}
}

func TestNamespaceCollector_WithLabels(t *testing.T) {
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "http_requests_total",
		Help: "Total HTTP requests",
	}, []string{"method", "status"})
	counter.WithLabelValues("GET", "200").Inc()
	counter.WithLabelValues("POST", "201").Add(5)

	originalRegistry := prometheus.NewRegistry()
	originalRegistry.MustRegister(counter)

	namespacedCollector := NewNamespaceCollector("myapp", originalRegistry)
	namespacedRegistry := prometheus.NewRegistry()
	namespacedRegistry.MustRegister(namespacedCollector)

	expected := `
# HELP myapp_http_requests_total Total HTTP requests
# TYPE myapp_http_requests_total counter
myapp_http_requests_total{method="GET",status="200"} 1
myapp_http_requests_total{method="POST",status="201"} 5
`
	if err := testutil.GatherAndCompare(namespacedRegistry, strings.NewReader(expected)); err != nil {
		t.Errorf("Labeled counter comparison failed: %v", err)
	}
}

func TestNamespaceCollector_GaugeWithLabels(t *testing.T) {
	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "connections_active",
		Help: "Active connections",
	}, []string{"protocol"})
	gauge.WithLabelValues("tcp").Set(10)
	gauge.WithLabelValues("udp").Set(3)

	originalRegistry := prometheus.NewRegistry()
	originalRegistry.MustRegister(gauge)

	namespacedCollector := NewNamespaceCollector("myapp", originalRegistry)
	namespacedRegistry := prometheus.NewRegistry()
	namespacedRegistry.MustRegister(namespacedCollector)

	expected := `
# HELP myapp_connections_active Active connections
# TYPE myapp_connections_active gauge
myapp_connections_active{protocol="tcp"} 10
myapp_connections_active{protocol="udp"} 3
`
	if err := testutil.GatherAndCompare(namespacedRegistry, strings.NewReader(expected)); err != nil {
		t.Errorf("Labeled gauge comparison failed: %v", err)
	}
}

func TestNamespaceCollector_HistogramWithLabels(t *testing.T) {
	hist := prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "request_duration_seconds",
		Help:    "Request duration",
		Buckets: []float64{0.01, 0.1, 1.0},
	}, []string{"handler"})
	hist.WithLabelValues("/api").Observe(0.05)
	hist.WithLabelValues("/api").Observe(0.5)

	originalRegistry := prometheus.NewRegistry()
	originalRegistry.MustRegister(hist)

	namespacedCollector := NewNamespaceCollector("myapp", originalRegistry)
	namespacedRegistry := prometheus.NewRegistry()
	namespacedRegistry.MustRegister(namespacedCollector)

	metricFamilies, err := namespacedRegistry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "myapp_request_duration_seconds" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected metric myapp_request_duration_seconds not found")
	}
}
