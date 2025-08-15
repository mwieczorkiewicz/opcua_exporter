package utils

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/efficientgo/core/errcapture"
	"github.com/efficientgo/core/errors"
	"github.com/efficientgo/e2e"
	e2emon "github.com/efficientgo/e2e/monitoring"
	"github.com/stretchr/testify/require"
)

// Test timing constants for consistency
const (
	DefaultMetricWaitTimeout     = 20 * time.Second
	DefaultScrapeWaitTimeout     = 20 * time.Second
	DefaultPrometheusWaitTimeout = 20 * time.Second
	DefaultTestTimeout           = 5 * time.Minute

	// Backoff configuration
	DefaultInitialInterval = 500 * time.Millisecond
	DefaultMaxInterval     = 10 * time.Second
	DefaultMaxRetries      = 10
	DefaultRandomization   = 0.1 // 10% jitter
)

// ExporterTestHelper provides clean abstractions for e2e testing
type ExporterTestHelper struct {
	ctx        context.Context
	env        e2e.Environment
	prometheus *e2emon.Prometheus
	t          *testing.T
}

// NewExporterTestHelper creates a new test helper with clean abstractions
func NewExporterTestHelper(ctx context.Context, env e2e.Environment, prometheus *e2emon.Prometheus, t *testing.T) *ExporterTestHelper {
	return &ExporterTestHelper{
		ctx:        ctx,
		env:        env,
		prometheus: prometheus,
		t:          t,
	}
}

// ExporterTestScenario abstracts different configuration methods
type ExporterTestScenario struct {
	Name               string
	CreateExporterFunc func(env e2e.Environment, name string, endpoint string, nodes []TestNode) (*OPCUAExporter, error)
	Description        string
}

// GetAllConfigurationScenarios returns all supported configuration scenarios
func GetAllConfigurationScenarios() []ExporterTestScenario {
	return []ExporterTestScenario{
		{
			Name:               "yaml-config",
			CreateExporterFunc: NewOPCUAExporter,
			Description:        "YAML configuration file",
		},
		{
			Name:               "env-vars",
			CreateExporterFunc: NewOPCUAExporterWithEnvConfig,
			Description:        "Environment variables",
		},
		{
			Name:               "cli-flags",
			CreateExporterFunc: NewOPCUAExporterWithFlags,
			Description:        "Command-line flags",
		},
	}
}

// StartExporterAndWaitForScraping starts exporter and waits for Prometheus scraping
func (h *ExporterTestHelper) StartExporterAndWaitForScraping(scenario ExporterTestScenario, exporterName string, opcuaEndpoint string, testNodes []TestNode) (*OPCUAExporter, error) {
	h.t.Logf("Starting exporter '%s' with %s", exporterName, scenario.Description)

	exporter, err := scenario.CreateExporterFunc(h.env, exporterName, opcuaEndpoint, testNodes)
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	if err := exporter.Start(); err != nil {
		return nil, fmt.Errorf("failed to start exporter: %w", err)
	}

	h.t.Logf("Exporter started at: %s", exporter.Endpoint())

	// Wait for Prometheus to start scraping
	if err := h.WaitForExporterReadiness(exporterName); err != nil {
		return nil, fmt.Errorf("exporter readiness check failed: %w", err)
	}

	return exporter, nil
}

// VerifyExporterMetrics validates all expected metrics are available
func (h *ExporterTestHelper) VerifyExporterMetrics(exporterName string, expectedMetrics []string) error {
	h.t.Logf("Verifying metrics for exporter: %s", exporterName)

	for _, metric := range expectedMetrics {
		h.t.Logf("Checking metric: %s", metric)
		if err := WaitForMetricWithBackoff(h.ctx, h.prometheus, metric, DefaultMetricWaitTimeout); err != nil {
			return fmt.Errorf("metric %s not available: %w", metric, err)
		}
	}

	h.t.Logf("All metrics verified successfully for exporter: %s", exporterName)
	return nil
}

// VerifyMetricContent checks metric data validity and structure
func (h *ExporterTestHelper) VerifyMetricContent(metricName string) error {
	h.t.Logf("Verifying content for metric: %s", metricName)

	result, err := QueryPrometheusMetric(h.ctx, h.prometheus, metricName)
	if err != nil {
		return fmt.Errorf("failed to query metric %s: %w", metricName, err)
	}

	if result.Status != "success" {
		return fmt.Errorf("metric query unsuccessful for %s: %s", metricName, result.Status)
	}

	if len(result.Data.Result) == 0 {
		return fmt.Errorf("metric %s has no data", metricName)
	}

	h.t.Logf("Metric content verified successfully: %s", metricName)
	return nil
}

// WaitForExporterReadiness implements proper waiting with exponential backoff
func (h *ExporterTestHelper) WaitForExporterReadiness(exporterName string) error {
	h.t.Logf("Waiting for exporter readiness: %s", exporterName)

	// First wait for basic exporter metrics
	if err := WaitForMetricWithBackoff(h.ctx, h.prometheus, "opcua_exporter_uptime_seconds", DefaultMetricWaitTimeout); err != nil {
		return fmt.Errorf("uptime metric not available: %w", err)
	}

	// Then wait for scrape target to be up
	if err := WaitForScrapeTargetWithBackoff(h.ctx, h.prometheus, exporterName, DefaultScrapeWaitTimeout); err != nil {
		return fmt.Errorf("scrape target not ready: %w", err)
	}

	h.t.Logf("Exporter readiness confirmed: %s", exporterName)
	return nil
}

// WaitForMetricWithBackoff waits for a metric using exponential backoff
func WaitForMetricWithBackoff(ctx context.Context, prometheus *e2emon.Prometheus, metricName string, timeout time.Duration) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	operation := func() (struct{}, error) {
		result, err := QueryPrometheusMetric(timeoutCtx, prometheus, metricName)
		if err != nil {
			return struct{}{}, err
		}
		if result.Status != "success" || len(result.Data.Result) == 0 {
			return struct{}{}, fmt.Errorf("metric %s not available yet", metricName)
		}
		return struct{}{}, nil
	}

	bo := createStandardBackoffConfig()
	_, err := backoff.Retry(timeoutCtx, operation, backoff.WithBackOff(bo))
	if err != nil {
		return fmt.Errorf("timeout waiting for metric %s after %v: %w", metricName, timeout, err)
	}
	return nil
}

// WaitForScrapeTargetWithBackoff waits for Prometheus to scrape target using exponential backoff
func WaitForScrapeTargetWithBackoff(ctx context.Context, prometheus *e2emon.Prometheus, job string, timeout time.Duration) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	operation := func() (struct{}, error) {
		result, err := QueryPrometheusMetric(timeoutCtx, prometheus, fmt.Sprintf("up{job=\"%s\"}", job))
		if err != nil {
			return struct{}{}, err
		}
		if result.Status != "success" || len(result.Data.Result) == 0 {
			return struct{}{}, fmt.Errorf("scrape target %s not available yet", job)
		}
		return struct{}{}, nil
	}

	bo := createStandardBackoffConfig()
	_, err := backoff.Retry(timeoutCtx, operation, backoff.WithBackOff(bo))
	if err != nil {
		return fmt.Errorf("timeout waiting for scrape target %s after %v: %w", job, timeout, err)
	}
	return nil
}

// createStandardBackoffConfig creates consistent backoff configuration
func createStandardBackoffConfig() backoff.BackOff {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = DefaultInitialInterval
	bo.MaxInterval = DefaultMaxInterval
	bo.RandomizationFactor = DefaultRandomization
	return bo
}

// waitForTCPPortWithBackoff waits for a TCP port using exponential backoff
func waitForTCPPortWithBackoff(address string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	operation := func() (struct{}, error) {
		conn, err := net.DialTimeout("tcp", address, 1*time.Second)
		if err != nil {
			return struct{}{}, err
		}
		conn.Close()
		return struct{}{}, nil
	}

	bo := createStandardBackoffConfig()
	_, err := backoff.Retry(ctx, operation, backoff.WithBackOff(bo))
	if err != nil {
		return fmt.Errorf("timeout waiting for TCP port %s after %v: %w", address, timeout, err)
	}
	return nil
}

// AssertExporterRunning asserts that an exporter is running with descriptive error
func AssertExporterRunning(t *testing.T, exporter *OPCUAExporter, exporterName string) {
	require.NotNil(t, exporter, "Exporter %s should not be nil", exporterName)
	exporter.AssertRunning(t)
}

// AssertAllExportersRunning validates multiple exporters are running
func AssertAllExportersRunning(t *testing.T, exporters map[string]*OPCUAExporter) {
	for name, exporter := range exporters {
		AssertExporterRunning(t, exporter, name)
	}
}

// GetTestNodeMetricNames extracts metric names from test nodes for validation
func GetTestNodeMetricNames(nodes []TestNode) []string {
	metrics := make([]string, len(nodes))
	for i, node := range nodes {
		metrics[i] = node.MetricName
	}
	return metrics
}

// GetStandardExporterMetrics returns the list of standard exporter metrics
func GetStandardExporterMetrics() []string {
	return []string{
		"opcua_exporter_uptime_seconds",
		"opcua_exporter_message_count",
		"opcua_server_time",
	}
}

// waitForTCPPort waits for a TCP port to be available (deprecated - use waitForTCPPortWithBackoff)
func waitForTCPPort(address string, timeout time.Duration) error {
	return waitForTCPPortWithBackoff(address, timeout)
}

type httpReadinessProbeWithExponentialBackoff struct {
	portName                 string
	path                     string
	scheme                   string
	expectedStatusRangeStart int
	expectedStatusRangeEnd   int
	expectedContent          []string
	fallbackTimeout          time.Duration
}

// NewHTTPReadinessProbeWithExponentialBackoff creates a readiness probe for HTTP services with exponential backoff
func NewHTTPReadinessProbeWithExponentialBackoff(portName, path, scheme string,
	expectedStatusRangeStart, expectedStatusRangeEnd int,
	fallbackTimeout time.Duration, expectedContent ...string) e2e.ReadinessProbe {
	return &httpReadinessProbeWithExponentialBackoff{
		portName:                 portName,
		path:                     path,
		scheme:                   scheme,
		expectedStatusRangeStart: expectedStatusRangeStart,
		expectedStatusRangeEnd:   expectedStatusRangeEnd,
		expectedContent:          expectedContent,
		fallbackTimeout:          fallbackTimeout,
	}
}

func (p *httpReadinessProbeWithExponentialBackoff) Ready(runnable e2e.Runnable) error {
	endpoint := runnable.Endpoint(p.portName)
	if endpoint == "" {
		return errors.Newf("cannot get service endpoint for port %s", p.portName)
	}
	if endpoint == "stopped" {
		return errors.New("service has stopped")
	}

	httpClient := p.buildHTTPClient()

	ctx, cancel := context.WithTimeout(context.Background(), p.fallbackTimeout)
	defer cancel()

	operation := func() (struct{}, error) {
		if p.checkReadiness(httpClient, endpoint) {
			return struct{}{}, nil
		}
		return struct{}{}, fmt.Errorf("HTTP readiness probe failed for %s%s", p.scheme, endpoint)
	}

	bo := createStandardBackoffConfig()
	_, err := backoff.Retry(ctx, operation, backoff.WithBackOff(bo))
	if err != nil {
		return fmt.Errorf("timeout waiting for HTTP readiness probe on %s%s after %v: %w",
			p.scheme, endpoint, p.fallbackTimeout, err)
	}
	return nil
}

func (p *httpReadinessProbeWithExponentialBackoff) buildHTTPClient() *http.Client {
	if p.scheme == "HTTPS" {
		return &http.Client{
			Timeout: 1 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	}
	return &http.Client{Timeout: 1 * time.Second}
}

func (p *httpReadinessProbeWithExponentialBackoff) checkReadiness(httpClient *http.Client, endpoint string) bool {
	res, err := httpClient.Get(p.scheme + "://" + endpoint + p.path)
	if err != nil {
		return false
	}
	defer errcapture.ExhaustClose(&err, res.Body, "response readiness")

	body, _ := io.ReadAll(res.Body)
	if res.StatusCode < p.expectedStatusRangeStart || res.StatusCode > p.expectedStatusRangeEnd {
		return false
	}

	if len(p.expectedContent) == 0 {
		return true
	}
	for _, expected := range p.expectedContent {
		if strings.Contains(string(body), expected) {
			return true
		}
	}
	return false
}
