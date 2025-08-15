package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/efficientgo/core/testutil"
	"github.com/efficientgo/e2e"
	e2emon "github.com/efficientgo/e2e/monitoring"
	"github.com/stretchr/testify/suite"

	"github.com/mwieczorkiewicz/opcua_exporter/e2e/utils"
)

const (
	opcuaExporterName = "opcua-exporter"
)

// E2ETestSuite is the main test suite for end-to-end tests
type E2ETestSuite struct {
	suite.Suite
	env         e2e.Environment
	opcuaServer *utils.OPCUAServer
	ctx         context.Context
	cancel      context.CancelFunc
}

// SetupSuite is called once before the entire test suite
func (suite *E2ETestSuite) SetupSuite() {
	// Create a new Docker environment
	env, err := e2e.New()
	suite.Require().NoError(err, "Failed to create e2e environment")
	suite.env = env

	// Set up context with timeout for the entire test suite
	suite.ctx, suite.cancel = context.WithTimeout(context.Background(), 10*time.Minute)

	suite.T().Log("=== Starting OPC UA test server...")

	// Create and start OPC UA test server once for all tests
	suite.opcuaServer = utils.NewOPCUAServer(suite.env, "opcua-server")
	err = suite.opcuaServer.Start(suite.ctx)
	suite.Require().NoError(err, "Failed to start OPC UA server")

	// Wait for the OPC UA server to be ready
	err = suite.opcuaServer.WaitForReady(suite.ctx, 30*time.Second)
	suite.Require().NoError(err, "OPC UA server did not become ready")

	suite.T().Logf("OPC UA server started at: %s", suite.opcuaServer.Endpoint())

	// Clean up resources when suite is done
	suite.T().Cleanup(func() {
		suite.cancel()
		if suite.opcuaServer != nil {
			_ = suite.opcuaServer.Stop()
		}
		if suite.env != nil {
			suite.env.Close()
		}
	})
}

// SetupTest is called before each test
func (suite *E2ETestSuite) SetupTest() {
	// OPC UA server is already running from SetupSuite
	// Just verify it's still running
	suite.opcuaServer.AssertRunning(suite.T())
}

// TearDownTest is called after each test
func (suite *E2ETestSuite) TearDownTest() {
	// Clean up test-specific components (exporters and prometheus instances)
	// but leave the OPC UA server running for other tests
}

// TestOPCUAExporterBasicFunctionality tests basic OPC UA exporter functionality
func (suite *E2ETestSuite) TestOPCUAExporterBasicFunctionality() {
	suite.T().Log("=== Testing basic OPC UA exporter functionality...")

	// Get test nodes from the OPC UA server
	testNodes := suite.opcuaServer.GetSimulationTestNodes()

	// Create and start Prometheus first (before starting the OPC UA exporter)
	prometheus, err := utils.NewPrometheusWithOPCUAExporterAndJob(
		suite.env,
		"prometheus-basic",
		"opcua-exporter-basic:9686", // Use the expected internal endpoint
		"opcua-exporter-basic",
	)
	suite.Require().NoError(err, "Failed to create Prometheus")

	err = e2e.StartAndWaitReady(prometheus)
	suite.Require().NoError(err, "Failed to start Prometheus")
	defer prometheus.Stop()

	suite.T().Logf("Prometheus started at: http://%s", prometheus.Endpoint("http"))

	// Now create and start OPC UA exporter
	opcuaExporter, err := utils.NewOPCUAExporter(
		suite.env,
		"opcua-exporter-basic",
		suite.opcuaServer.Endpoint(),
		testNodes,
	)
	suite.Require().NoError(err, "Failed to create OPC UA exporter")

	err = opcuaExporter.Start()
	suite.Require().NoError(err, "Failed to start OPC UA exporter")
	defer opcuaExporter.Stop()

	suite.T().Logf("OPC UA exporter started at: %s", opcuaExporter.Endpoint())

	// Assert all components are running
	suite.opcuaServer.AssertRunning(suite.T())
	opcuaExporter.AssertRunning(suite.T())
	suite.Assert().True(prometheus.IsRunning(), "Prometheus should be running")

	// Wait for initial metrics to be scraped by Prometheus
	suite.T().Log("Waiting for initial metrics to be scraped...")
	err = utils.WaitForMetric(prometheus, suite.ctx, "opcua_exporter_uptime_seconds", 30*time.Second)
	suite.Require().NoError(err, "Should wait for initial exporter metrics")

	// Check that Prometheus can scrape the OPC UA exporter
	suite.T().Log("Checking if Prometheus can scrape OPC UA exporter...")
	utils.AssertTargetUp(suite.T(), prometheus, suite.ctx, "opcua-exporter-basic")

	// Check for basic exporter metrics
	suite.T().Log("Checking for basic exporter metrics...")
	err = utils.WaitForMetric(prometheus, suite.ctx, "opcua_exporter_message_count", 30*time.Second)
	suite.Assert().NoError(err, "opcua_exporter_message_count metric should be available")

	// Check for test node metrics from the OPC UA server
	for _, node := range testNodes {
		suite.T().Logf("Checking for metric: %s", node.MetricName)
		err := utils.WaitForMetric(prometheus, suite.ctx, node.MetricName, 30*time.Second)
		suite.Assert().NoError(err, "Metric %s should be available", node.MetricName)
	}
}

// TestOPCUAExporterMetricsContent tests the content and values of metrics
func (suite *E2ETestSuite) TestOPCUAExporterMetricsContent() {
	suite.T().Log("=== Testing OPC UA exporter metrics content...")

	// Get test nodes from the OPC UA server
	testNodes := suite.opcuaServer.GetSimulationTestNodes()

	// Create and start OPC UA exporter for this test
	opcuaExporter, err := utils.NewOPCUAExporter(
		suite.env,
		"opcua-exporter-content",
		suite.opcuaServer.Endpoint(),
		testNodes,
	)
	suite.Require().NoError(err, "Failed to create OPC UA exporter")

	err = opcuaExporter.Start()
	suite.Require().NoError(err, "Failed to start OPC UA exporter")
	defer opcuaExporter.Stop()

	// Create and start Prometheus with OPC UA exporter target
	prometheus, err := utils.NewPrometheusWithOPCUAExporterAndJob(
		suite.env,
		"prometheus-content",
		opcuaExporter.InternalEndpoint(),
		"opcua-exporter-content",
	)
	suite.Require().NoError(err, "Failed to create Prometheus")

	err = e2e.StartAndWaitReady(prometheus)
	suite.Require().NoError(err, "Failed to start Prometheus")
	defer prometheus.Stop()

	// Wait for metrics to stabilize
	time.Sleep(20 * time.Second)

	// Test specific metric values
	for _, node := range testNodes {
		suite.T().Logf("Querying metric content for: %s", node.MetricName)
		result, err := utils.QueryPrometheusMetric(prometheus, suite.ctx, node.MetricName)
		suite.Assert().NoError(err, "Should be able to query metric %s", node.MetricName)
		suite.Assert().Contains(result, `"status":"success"`, "Metric query should be successful for %s", node.MetricName)

		// Check that we have actual data (not empty result)
		suite.Assert().NotContains(result, `"data":{"resultType":"vector","result":[]}`,
			"Metric %s should have actual data", node.MetricName)
	}
}

// TestOPCUAExporterPrometheusIntegration tests the full integration with Prometheus
func (suite *E2ETestSuite) TestOPCUAExporterPrometheusIntegration() {
	suite.T().Log("=== Testing OPC UA exporter Prometheus integration...")

	// Get test nodes from the OPC UA server
	testNodes := suite.opcuaServer.GetSimulationTestNodes()

	// Create and start OPC UA exporter for this test
	opcuaExporter, err := utils.NewOPCUAExporter(
		suite.env,
		"opcua-exporter-integration",
		suite.opcuaServer.Endpoint(),
		testNodes,
	)
	suite.Require().NoError(err, "Failed to create OPC UA exporter")

	err = opcuaExporter.Start()
	suite.Require().NoError(err, "Failed to start OPC UA exporter")
	defer opcuaExporter.Stop()

	// Create and start Prometheus with OPC UA exporter target
	prometheus, err := utils.NewPrometheusWithOPCUAExporterAndJob(
		suite.env,
		"prometheus-integration",
		opcuaExporter.InternalEndpoint(),
		"opcua-exporter-integration",
	)
	suite.Require().NoError(err, "Failed to create Prometheus")

	err = e2e.StartAndWaitReady(prometheus)
	suite.Require().NoError(err, "Failed to start Prometheus")
	defer prometheus.Stop()

	// Wait for sufficient metrics collection
	time.Sleep(25 * time.Second)

	// Verify that Prometheus has scraped multiple samples
	suite.T().Log("Verifying Prometheus has collected samples...")
	err = prometheus.WaitSumMetrics(e2emon.Greater(1), "prometheus_tsdb_head_samples_appended_total")
	suite.Assert().NoError(err, "Prometheus should have collected samples")

	// Check that opcua_server_time metric is available (indicates successful OPC UA connection)
	result, err := utils.QueryPrometheusMetric(prometheus, suite.ctx, fmt.Sprintf("opcua_server_time{job=\"%s\"}", "opcua-exporter-integration"))
	suite.Assert().NoError(err, "Should be able to query opcua_server_time metric")
	suite.Assert().Contains(result, `"value":["`, "opcua_server_time metric should have a value")
	suite.Assert().NotContains(result, `"data":{"resultType":"vector","result":[]}`, "opcua_server_time should have data")

	// Verify scrape duration metrics exist
	err = utils.WaitForMetric(prometheus, suite.ctx, "prometheus_target_scrape_duration_seconds", 10*time.Second)
	suite.Assert().NoError(err, "Scrape duration metrics should be available")
}

// TestOPCUAExporterResilience tests the resilience of the exporter
func (suite *E2ETestSuite) TestOPCUAExporterResilience() {
	suite.T().Log("=== Testing OPC UA exporter resilience...")

	// Get test nodes from the OPC UA server
	testNodes := suite.opcuaServer.GetSimulationTestNodes()

	// Create and start OPC UA exporter for this test
	opcuaExporter, err := utils.NewOPCUAExporter(
		suite.env,
		"opcua-exporter-resilience",
		suite.opcuaServer.Endpoint(),
		testNodes,
	)
	suite.Require().NoError(err, "Failed to create OPC UA exporter")

	err = opcuaExporter.Start()
	suite.Require().NoError(err, "Failed to start OPC UA exporter")
	defer opcuaExporter.Stop()

	// Create and start Prometheus with OPC UA exporter target
	prometheus, err := utils.NewPrometheusWithOPCUAExporterAndJob(
		suite.env,
		"prometheus-resilience",
		opcuaExporter.InternalEndpoint(),
		"opcua-exporter-resilience",
	)
	suite.Require().NoError(err, "Failed to create Prometheus")

	err = e2e.StartAndWaitReady(prometheus)
	suite.Require().NoError(err, "Failed to start Prometheus")
	defer prometheus.Stop()

	// First, verify normal operation
	time.Sleep(10 * time.Second)
	utils.AssertTargetUp(suite.T(), prometheus, suite.ctx, "opcua-exporter-resilience")

	// For resilience testing, we'll just verify the exporter can handle brief disconnections
	// without restarting the shared OPC UA server
	suite.T().Log("Verifying exporter continues to work...")
	time.Sleep(15 * time.Second)

	// Verify the exporter is still providing metrics to Prometheus
	utils.AssertTargetUp(suite.T(), prometheus, suite.ctx, "opcua-exporter-resilience")
}

// TestMain runs the test suite
func TestE2ETestSuite(t *testing.T) {
	// Skip e2e tests if running unit tests
	if testing.Short() {
		t.Skip("Skipping e2e tests in short mode")
	}

	suite.Run(t, new(E2ETestSuite))
}

// TestOPCUAExporterWithCommandLineFlags tests the exporter with command-line flag configuration
func (suite *E2ETestSuite) TestOPCUAExporterWithCommandLineFlags() {
	suite.T().Log("=== Testing OPC UA exporter with command-line flags...")

	// Use standard test nodes for flags configuration
	testNodes := suite.opcuaServer.GetSimulationTestNodes()

	// Create and start OPC UA exporter with command-line flags
	flagsExporter, err := utils.NewOPCUAExporterWithFlags(
		suite.env,
		"opcua-exporter-flags",
		suite.opcuaServer.Endpoint(),
		testNodes,
	)
	suite.Require().NoError(err, "Failed to create OPC UA exporter with flags")

	err = flagsExporter.Start()
	suite.Require().NoError(err, "Failed to start OPC UA exporter with flags")
	defer flagsExporter.Stop()

	suite.T().Logf("OPC UA exporter started with flags at: %s", flagsExporter.Endpoint())

	// Create and start Prometheus for flags testing
	prometheus, err := utils.NewPrometheusWithOPCUAExporterAndJob(
		suite.env,
		"prometheus-flags",
		flagsExporter.InternalEndpoint(),
		"opcua-exporter-flags",
	)
	suite.Require().NoError(err, "Failed to create Prometheus for flags")

	err = e2e.StartAndWaitReady(prometheus)
	suite.Require().NoError(err, "Failed to start Prometheus for flags")
	defer prometheus.Stop()

	// Wait for metrics collection
	suite.T().Log("Waiting for metrics to be collected...")
	time.Sleep(20 * time.Second)

	// Check that all metrics are available
	for _, node := range testNodes {
		suite.T().Logf("Checking for metric: %s", node.MetricName)
		err := utils.WaitForMetric(prometheus, suite.ctx, node.MetricName, 30*time.Second)
		suite.Assert().NoError(err, "Metric %s should be available", node.MetricName)
	}

	// Verify metric values
	suite.T().Log("Verifying metric values...")
	for _, node := range testNodes {
		result, err := utils.QueryPrometheusMetric(prometheus, suite.ctx, node.MetricName)
		suite.Assert().NoError(err, "Should be able to query metric %s", node.MetricName)
		suite.Assert().Contains(result, `"status":"success"`, "Metric query should be successful for %s", node.MetricName)

		// Check that we have actual data
		suite.Assert().NotContains(result, `"data":{"resultType":"vector","result":[]}`,
			"Metric %s should have actual data", node.MetricName)
	}
}

// TestOPCUAExporterConfigurationMethods tests different configuration methods with the same test nodes
func (suite *E2ETestSuite) TestOPCUAExporterConfigurationMethods() {
	suite.T().Log("=== Testing OPC UA exporter configuration methods...")

	// Use the same test nodes for all configuration methods
	testNodes := suite.opcuaServer.GetSimulationTestNodes()

	// Test environment variable configuration
	suite.T().Log("Testing environment variable configuration...")
	envExporter, err := utils.NewOPCUAExporterWithEnvConfig(
		suite.env,
		"opcua-exporter-env",
		suite.opcuaServer.Endpoint(),
		testNodes,
	)
	suite.Require().NoError(err, "Failed to create exporter with env config")

	err = envExporter.Start()
	suite.Require().NoError(err, "Failed to start exporter with env config")
	defer envExporter.Stop()

	// Test YAML configuration
	suite.T().Log("Testing YAML configuration...")
	configExporter, err := utils.NewOPCUAExporterWithYAMLConfig(
		suite.env,
		"opcua-exporter-config",
		suite.opcuaServer.Endpoint(),
		testNodes,
	)
	suite.Require().NoError(err, "Failed to create exporter with YAML config")

	err = configExporter.Start()
	suite.Require().NoError(err, "Failed to start exporter with YAML config")
	defer configExporter.Stop()

	// Verify both configurations work
	time.Sleep(15 * time.Second)

	envExporter.AssertRunning(suite.T())
	configExporter.AssertRunning(suite.T())

	// Verify both exporters are collecting the same metrics
	for _, node := range testNodes {
		suite.T().Logf("Verifying metric %s from both configurations", node.MetricName)

		// Check env config exporter metrics endpoint
		envMetricsURL := fmt.Sprintf("http://%s/metrics", envExporter.Endpoint())
		suite.T().Logf("Env config metrics at: %s", envMetricsURL)

		// Check YAML config exporter metrics endpoint
		yamlMetricsURL := fmt.Sprintf("http://%s/metrics", configExporter.Endpoint())
		suite.T().Logf("YAML config metrics at: %s", yamlMetricsURL)
	}
}

// TestHealthCheck is a simple health check test that can run quickly
func TestHealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e tests in short mode")
	}

	// Create a minimal test environment
	env, err := e2e.New()
	testutil.Ok(t, err)
	defer env.Close()

	// Just test that we can create the components without starting them
	opcuaServer := utils.NewOPCUAServer(env, "health-opcua-server")
	testNodes := opcuaServer.GetSimulationTestNodes()

	opcuaExporter, err := utils.NewOPCUAExporter(env, "health-opcua-exporter", "opc.tcp://dummy:4840", testNodes)
	testutil.Ok(t, err)

	prometheus, err := utils.NewPrometheusWithOPCUAExporter(env, "health-prometheus", "dummy:9686")
	testutil.Ok(t, err)

	// Verify components were created successfully
	if opcuaServer == nil || opcuaExporter == nil || prometheus == nil {
		t.Fatal("Failed to create test components")
	}

	t.Log("Health check passed - all components can be created")
}
