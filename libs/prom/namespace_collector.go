// Package prom provides Prometheus metric utilities including namespace wrapping.
package prom

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// NamespaceCollector wraps an existing prometheus.Collector and adds a namespace prefix
// to all metrics exposed by the wrapped collector by modifying the metric family names
// during the gathering process.
type NamespaceCollector struct {
	collector prometheus.Collector
	namespace string
	registry  *prometheus.Registry
}

// NewNamespaceCollector creates a new NamespaceCollector that wraps the given collector
// and adds the specified namespace to all metrics.
func NewNamespaceCollector(namespace string, collector prometheus.Collector) *NamespaceCollector {
	tempRegistry := prometheus.NewRegistry()
	tempRegistry.MustRegister(collector)
	return &NamespaceCollector{
		collector: collector,
		namespace: namespace,
		registry:  tempRegistry,
	}
}

// Describe implements the prometheus.Collector interface.
func (nc *NamespaceCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(nc, ch)
}

// Collect implements prometheus.Collector interface.
// It collects metrics from the wrapped collector and modifies their names to include the namespace.
func (nc *NamespaceCollector) Collect(ch chan<- prometheus.Metric) {
	// Gather the metrics
	metricFamilies, err := nc.registry.Gather()
	if err != nil {
		// If there's an error, we can't do much, so just return
		return
	}

	// Process each metric family and add namespace to the name
	for _, mf := range metricFamilies {
		// Create a new metric family with the namespaced name
		namespacedName := prometheus.BuildFQName(nc.namespace, "", mf.GetName())
		namespacedMF := &dto.MetricFamily{
			Name:   &namespacedName,
			Help:   mf.Help,
			Type:   mf.Type,
			Metric: mf.Metric,
		}

		// Convert each metric in the family to a const metric and send it
		for _, metric := range namespacedMF.Metric {
			var constMetric prometheus.Metric
			var err error

			labelNames, labelValues := getLabelNamesAndValues(metric)

			switch namespacedMF.GetType() { //nolint:exhaustive // default case handles remaining types
			case dto.MetricType_COUNTER:
				constMetric, err = prometheus.NewConstMetric(
					prometheus.NewDesc(namespacedMF.GetName(), namespacedMF.GetHelp(), labelNames, nil),
					prometheus.CounterValue,
					metric.GetCounter().GetValue(),
					labelValues...,
				)
			case dto.MetricType_GAUGE:
				constMetric, err = prometheus.NewConstMetric(
					prometheus.NewDesc(namespacedMF.GetName(), namespacedMF.GetHelp(), labelNames, nil),
					prometheus.GaugeValue,
					metric.GetGauge().GetValue(),
					labelValues...,
				)
			case dto.MetricType_HISTOGRAM:
				// For histograms, we need to handle buckets
				hist := metric.GetHistogram()
				buckets := make(map[float64]uint64)
				for _, bucket := range hist.GetBucket() {
					buckets[bucket.GetUpperBound()] = bucket.GetCumulativeCount()
				}
				constMetric, err = prometheus.NewConstHistogram(
					prometheus.NewDesc(namespacedMF.GetName(), namespacedMF.GetHelp(), labelNames, nil),
					hist.GetSampleCount(),
					hist.GetSampleSum(),
					buckets,
					labelValues...,
				)
			case dto.MetricType_SUMMARY:
				// For summaries, we need to handle quantiles
				summ := metric.GetSummary()
				quantiles := make(map[float64]float64)
				for _, quantile := range summ.GetQuantile() {
					quantiles[quantile.GetQuantile()] = quantile.GetValue()
				}
				constMetric, err = prometheus.NewConstSummary(
					prometheus.NewDesc(namespacedMF.GetName(), namespacedMF.GetHelp(), labelNames, nil),
					summ.GetSampleCount(),
					summ.GetSampleSum(),
					quantiles,
					labelValues...,
				)
			default:
				// For untyped metrics, treat as gauge
				constMetric, err = prometheus.NewConstMetric(
					prometheus.NewDesc(namespacedMF.GetName(), namespacedMF.GetHelp(), labelNames, nil),
					prometheus.UntypedValue,
					metric.GetUntyped().GetValue(),
					labelValues...,
				)
			}

			if err == nil {
				ch <- constMetric
			}
		}
	}
}

// Helper function to extract label names from a metric.
func getLabelNamesAndValues(metric *dto.Metric) ([]string, []string) {
	labels := metric.GetLabel()
	l := len(labels)
	names := make([]string, l)
	values := make([]string, l)
	for i, label := range labels {
		names[i] = label.GetName()
		values[i] = label.GetValue()
	}
	return names, values
}
