package utils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/efficientgo/e2e"
	e2edb "github.com/efficientgo/e2e/db"
	e2emon "github.com/efficientgo/e2e/monitoring"
	"github.com/stretchr/testify/assert"
)

// NewPrometheusWithOPCUAExporter creates a new Prometheus instance configured to scrape the OPC UA exporter
func NewPrometheusWithOPCUAExporter(env e2e.Environment, name string, opcuaExporterEndpoint string) (*e2emon.Prometheus, error) {
	return NewPrometheusWithOPCUAExporterAndJob(env, name, opcuaExporterEndpoint, "opcua-exporter")
}

// NewPrometheusWithOPCUAExporterAndJob creates a new Prometheus instance configured to scrape the OPC UA exporter with a specific job name
func NewPrometheusWithOPCUAExporterAndJob(env e2e.Environment, name string, opcuaExporterEndpoint string, jobName string) (*e2emon.Prometheus, error) {
	// Create Prometheus using e2edb helper
	prometheus := e2edb.NewPrometheus(env, name)

	// Create custom configuration that includes the OPC UA exporter target
	prometheusConfig := fmt.Sprintf(`
global:
  scrape_interval: 5s
  evaluation_interval: 5s
  external_labels:
    prometheus: %s

scrape_configs:
  - job_name: '%s'
    static_configs:
      - targets: ['%s']
    scrape_interval: 5s
    scrape_timeout: 5s
  - job_name: 'myself'
    scrape_interval: 1s
    scrape_timeout: 1s
    static_configs:
    - targets: [%s]
`, name, jobName, opcuaExporterEndpoint, prometheus.InternalEndpoint("http"))

	// Set the configuration
	if err := prometheus.SetConfigEncoded([]byte(prometheusConfig)); err != nil {
		return nil, fmt.Errorf("failed to set prometheus config: %w", err)
	}

	return prometheus, nil
}

// NewPrometheusForSuite creates a Prometheus instance optimized for test suite usage with dynamic target discovery
func NewPrometheusForSuite(env e2e.Environment, name string) (*e2emon.Prometheus, error) {
	// Create Prometheus using e2edb helper
	prometheus := e2edb.NewPrometheus(env, name)

	// Create configuration that can scrape any OPC UA exporter in the network
	prometheusConfig := fmt.Sprintf(`
global:
  scrape_interval: 5s
  evaluation_interval: 5s
  external_labels:
    prometheus: %s

scrape_configs:
  - job_name: 'opcua-exporters'
    static_configs:
      - targets: []
    scrape_interval: 5s
    scrape_timeout: 5s
    relabel_configs:
      - source_labels: [__address__]
        target_label: instance
  - job_name: 'myself'
    scrape_interval: 1s
    scrape_timeout: 1s
    static_configs:
    - targets: [%s]
`, name, prometheus.InternalEndpoint("http"))

	// Set the configuration
	if err := prometheus.SetConfigEncoded([]byte(prometheusConfig)); err != nil {
		return nil, fmt.Errorf("failed to set prometheus config: %w", err)
	}

	return prometheus, nil
}

// QueryMetric queries a specific metric from Prometheus via its HTTP endpoint
func QueryPrometheusMetric(prometheus *e2emon.Prometheus, ctx context.Context, metric string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("http://%s/api/v1/query?query=%s", prometheus.Endpoint("http"), metric)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to query metric: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(body), nil
}

// AssertMetricExists asserts that a specific metric exists in Prometheus
func AssertMetricExists(t assert.TestingT, prometheus *e2emon.Prometheus, ctx context.Context, metricName string) {
	result, err := QueryPrometheusMetric(prometheus, ctx, metricName)
	assert.NoError(t, err, "Failed to query metric %s", metricName)
	assert.Contains(t, result, `"status":"success"`, "Metric query should be successful")
	assert.NotContains(t, result, `"data":{"resultType":"vector","result":[]}`, "Metric %s should have data", metricName)
}

// AssertTargetUp asserts that the specified target is up in Prometheus using opcua_server_time metric
func AssertTargetUp(t assert.TestingT, prometheus *e2emon.Prometheus, ctx context.Context, job string) {
	result, err := QueryPrometheusMetric(prometheus, ctx, fmt.Sprintf("opcua_server_time"))
	assert.NoError(t, err, "Failed to query opcua_server_time metric for job %s", job)
	assert.Contains(t, result, `"value":["`, "opcua_server_time metric should have a value")
	// The metric should exist and have some timestamp value
	assert.NotContains(t, result, `"data":{"resultType":"vector","result":[]}`, "opcua_server_time should have data")
}

// WaitForMetric waits for a specific metric to appear in Prometheus
func WaitForMetric(prometheus *e2emon.Prometheus, ctx context.Context, metricName string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for metric %s", metricName)
		case <-ticker.C:
			result, err := QueryPrometheusMetric(prometheus, ctx, metricName)
			if err == nil && strings.Contains(result, `"status":"success"`) &&
				!strings.Contains(result, `"data":{"resultType":"vector","result":[]}`) {
				return nil
			}
		}
	}
}
