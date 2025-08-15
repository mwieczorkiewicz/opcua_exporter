package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/efficientgo/e2e"
	e2edb "github.com/efficientgo/e2e/db"
	e2emon "github.com/efficientgo/e2e/monitoring"
	"github.com/stretchr/testify/assert"
)

// PrometheusResponse represents the response from Prometheus /api/v1/query endpoint
type PrometheusResponse struct {
	Status string                 `json:"status"`
	Data   PrometheusResponseData `json:"data"`
}

// PrometheusResponseData represents the data section of a Prometheus API response
type PrometheusResponseData struct {
	ResultType string                  `json:"resultType"`
	Result     []PrometheusQueryResult `json:"result"`
}

// PrometheusQueryResult represents a single query result from Prometheus
type PrometheusQueryResult struct {
	Metric map[string]string `json:"metric"`
	Value  []any             `json:"value"`
}

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

// QueryPrometheusMetric queries a specific metric from Prometheus via its HTTP endpoint and returns the parsed response
func QueryPrometheusMetric(ctx context.Context, prometheus *e2emon.Prometheus, metric string) (*PrometheusResponse, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := "http://" + prometheus.Endpoint("http") + "/api/v1/query?query=" + metric
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query metric: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var promResponse PrometheusResponse
	if err := json.Unmarshal(body, &promResponse); err != nil {
		return nil, fmt.Errorf("failed to parse prometheus response: %w", err)
	}

	return &promResponse, nil
}

// AssertMetricExists asserts that a specific metric exists in Prometheus
func AssertMetricExists(ctx context.Context, t assert.TestingT, prometheus *e2emon.Prometheus, metricName string) {
	result, err := QueryPrometheusMetric(ctx, prometheus, metricName)
	assert.NoError(t, err, "Failed to query metric %s", metricName)
	assert.Equal(t, "success", result.Status, "Metric query should be successful")
	assert.NotEmpty(t, result.Data.Result, "Metric %s should have data", metricName)
}

// AssertTargetUp asserts that the specified target is up in Prometheus using opcua_server_time metric
// Uses exponential backoff with proper retry logic
func AssertTargetUp(ctx context.Context, t assert.TestingT, prometheus *e2emon.Prometheus, job string) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	operation := func() (struct{}, error) {
		result, err := QueryPrometheusMetric(timeoutCtx, prometheus, "opcua_server_time")
		if err != nil {
			return struct{}{}, err
		}
		if result.Status != "success" || len(result.Data.Result) == 0 {
			return struct{}{}, fmt.Errorf("opcua_server_time metric not available for job %s", job)
		}
		if len(result.Data.Result[0].Value) == 0 {
			return struct{}{}, fmt.Errorf("opcua_server_time metric has no value for job %s", job)
		}
		return struct{}{}, nil
	}

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 500 * time.Millisecond
	bo.MaxInterval = 10 * time.Second

	_, err := backoff.Retry(timeoutCtx, operation, backoff.WithBackOff(bo))
	if err != nil {
		assert.Fail(t, "AssertTargetUp failed after exponential backoff retries for job: %s, error: %v", job, err)
	}
}

// WaitForMetric waits for a specific metric to appear in Prometheus
func WaitForMetric(ctx context.Context, prometheus *e2emon.Prometheus, metricName string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for metric %s", metricName)
		case <-ticker.C:
			result, err := QueryPrometheusMetric(ctx, prometheus, metricName)
			if err == nil && result.Status == "success" && len(result.Data.Result) > 0 {
				return nil
			}
		}
	}
}

// WaitForScrapeTarget waits for Prometheus to successfully scrape a target
func WaitForScrapeTarget(ctx context.Context, prometheus *e2emon.Prometheus, job string, timeout time.Duration) error {
	return WaitForMetric(ctx, prometheus, fmt.Sprintf("up{job=\"%s\"}", job), timeout)
}

// NewPrometheusForTestSuite creates a Prometheus instance configured to scrape all test exporters
func NewPrometheusForTestSuite(env e2e.Environment, name string) (*e2emon.Prometheus, error) {
	// Create Prometheus using e2edb helper
	prometheus := e2edb.NewPrometheus(env, name)

	// Create configuration that includes all test exporter targets
	prometheusConfig := fmt.Sprintf(`
global:
  scrape_interval: 5s
  evaluation_interval: 5s
  external_labels:
    prometheus: %s

scrape_configs:
  - job_name: 'opcua-exporter-basic'
    static_configs:
      - targets: ['opcua-exporter-basic:9686']
    scrape_interval: 5s
    scrape_timeout: 5s
  - job_name: 'opcua-exporter-content'
    static_configs:
      - targets: ['opcua-exporter-content:9686']
    scrape_interval: 5s
    scrape_timeout: 5s
  - job_name: 'opcua-exporter-integration'
    static_configs:
      - targets: ['opcua-exporter-integration:9686']
    scrape_interval: 5s
    scrape_timeout: 5s
  - job_name: 'opcua-exporter-resilience'
    static_configs:
      - targets: ['opcua-exporter-resilience:9686']
    scrape_interval: 5s
    scrape_timeout: 5s
  - job_name: 'opcua-exporter-flags'
    static_configs:
      - targets: ['opcua-exporter-flags:9686']
    scrape_interval: 5s
    scrape_timeout: 5s
  - job_name: 'opcua-exporter-env'
    static_configs:
      - targets: ['opcua-exporter-env:9686']
    scrape_interval: 5s
    scrape_timeout: 5s
  - job_name: 'opcua-exporter-config'
    static_configs:
      - targets: ['opcua-exporter-config:9686']
    scrape_interval: 5s
    scrape_timeout: 5s
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
