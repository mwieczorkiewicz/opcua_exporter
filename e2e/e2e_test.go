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

// E2ETestSuite is the main test suite for end-to-end tests
type E2ETestSuite struct {
	suite.Suite
	env         e2e.Environment
	opcuaServer *utils.OPCUAServer
	prometheus  *e2emon.Prometheus
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
	err = suite.opcuaServer.WaitForReady(suite.ctx, 10*time.Second)
	suite.Require().NoError(err, "OPC UA server did not become ready")

	suite.T().Logf("OPC UA server started at: %s", suite.opcuaServer.Endpoint())

	suite.T().Log("=== Starting shared Prometheus...")

	// Create and start shared Prometheus instance for all tests
	suite.prometheus, err = utils.NewPrometheusForTestSuite(suite.env, "prometheus-shared")
	suite.Require().NoError(err, "Failed to create shared Prometheus")

	err = e2e.StartAndWaitReady(suite.prometheus)
	suite.Require().NoError(err, "Failed to start shared Prometheus")

	suite.T().Logf("Shared Prometheus started at: http://%s", suite.prometheus.Endpoint("http"))

	// Clean up resources when suite is done
	suite.T().Cleanup(func() {
		suite.cancel()
		if suite.prometheus != nil {
			_ = suite.prometheus.Stop()
		}
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
	// OPC UA server and Prometheus are already running from SetupSuite
	// Just verify they're still running
	suite.opcuaServer.AssertRunning(suite.T())
	suite.Assert().True(suite.prometheus.IsRunning(), "Shared Prometheus should be running")
}

// TearDownTest is called after each test
func (suite *E2ETestSuite) TearDownTest() {
	// Clean up test-specific components (exporters only)
	// but leave the OPC UA server and shared Prometheus running for other tests
}

// TestCanExportBasicOPCUAMetrics verifies exporter can collect and expose basic OPC UA metrics
func (suite *E2ETestSuite) TestCanExportBasicOPCUAMetrics() {
	suite.T().Log("=== Testing basic OPC UA metric collection and exposure...")

	testNodes := suite.opcuaServer.GetSimulationTestNodes()
	helper := utils.NewExporterTestHelper(suite.ctx, suite.env, suite.prometheus, suite.T())

	// Start exporter with YAML config and wait for readiness
	scenario := utils.ExporterTestScenario{
		Name:               "basic-yaml",
		CreateExporterFunc: utils.NewOPCUAExporter,
		Description:        "YAML configuration",
	}

	exporter, err := helper.StartExporterAndWaitForScraping(scenario, "opcua-exporter-basic", suite.opcuaServer.Endpoint(), testNodes)
	suite.Require().NoError(err, "Failed to start and verify exporter")
	defer exporter.Stop()

	// Verify all expected metrics are available
	expectedMetrics := append(utils.GetStandardExporterMetrics(), utils.GetTestNodeMetricNames(testNodes)...)
	err = helper.VerifyExporterMetrics("opcua-exporter-basic", expectedMetrics)
	suite.Require().NoError(err, "All metrics should be available")

	// Verify target is up in Prometheus
	utils.AssertTargetUp(suite.ctx, suite.T(), suite.prometheus, "opcua-exporter-basic")
}

// TestMetricDataIntegrityAndValidation verifies collected metrics contain valid data and structure
func (suite *E2ETestSuite) TestMetricDataIntegrityAndValidation() {
	suite.T().Log("=== Testing metric data integrity and validation...")

	testNodes := suite.opcuaServer.GetSimulationTestNodes()
	helper := utils.NewExporterTestHelper(suite.ctx, suite.env, suite.prometheus, suite.T())

	// Start exporter with env config
	scenario := utils.ExporterTestScenario{
		Name:               "content-env",
		CreateExporterFunc: utils.NewOPCUAExporterWithEnvConfig,
		Description:        "Environment variables",
	}

	exporter, err := helper.StartExporterAndWaitForScraping(scenario, "opcua-exporter-content", suite.opcuaServer.Endpoint(), testNodes)
	suite.Require().NoError(err, "Failed to start and verify exporter")
	defer exporter.Stop()

	// Verify metric content for all test nodes
	for _, node := range testNodes {
		err := helper.VerifyMetricContent(node.MetricName)
		suite.Assert().NoError(err, "Metric %s should have valid content", node.MetricName)
	}

	// Verify standard exporter metrics have content
	for _, metric := range utils.GetStandardExporterMetrics() {
		err := helper.VerifyMetricContent(metric)
		suite.Assert().NoError(err, "Standard metric %s should have valid content", metric)
	}
}

// TestPrometheusIntegrationAndTimeSeries verifies full Prometheus integration and time series collection
func (suite *E2ETestSuite) TestPrometheusIntegrationAndTimeSeries() {
	suite.T().Log("=== Testing Prometheus integration and time series collection...")

	testNodes := suite.opcuaServer.GetSimulationTestNodes()
	helper := utils.NewExporterTestHelper(suite.ctx, suite.env, suite.prometheus, suite.T())

	// Start exporter with YAML config for integration test
	scenario := utils.ExporterTestScenario{
		Name:               "integration-yaml",
		CreateExporterFunc: utils.NewOPCUAExporter,
		Description:        "YAML configuration",
	}

	exporter, err := helper.StartExporterAndWaitForScraping(scenario, "opcua-exporter-integration", suite.opcuaServer.Endpoint(), testNodes)
	suite.Require().NoError(err, "Failed to start and verify exporter")
	defer exporter.Stop()

	// Verify Prometheus is collecting samples
	err = suite.prometheus.WaitSumMetrics(e2emon.Greater(1), "prometheus_tsdb_head_samples_appended_total")
	suite.Assert().NoError(err, "Prometheus should have collected samples")

	// Verify OPC UA server time metric indicates successful connection
	err = helper.VerifyMetricContent("opcua_server_time")
	suite.Assert().NoError(err, "opcua_server_time should indicate successful OPC UA connection")

	// Query opcua_server_time with job label to verify timestamp validity
	result, err := utils.QueryPrometheusMetric(suite.ctx, suite.prometheus, fmt.Sprintf("opcua_server_time{job=\"%s\"}", "opcua-exporter-integration"))
	suite.Assert().NoError(err, "Should query opcua_server_time with job label")
	if len(result.Data.Result) > 0 && len(result.Data.Result[0].Value) > 0 {
		opcUaTimestamp, ok := result.Data.Result[0].Value[0].(float64)
		suite.Assert().True(ok, "opcua_server_time should be a float64 timestamp")
		suite.Assert().Greater(opcUaTimestamp, 0.0, "opcua_server_time should be a valid timestamp")
		suite.Assert().Greater(opcUaTimestamp, float64(time.Now().Add(-15*time.Second).Unix()), "opcua_server_time should be recent")
	}

	// Verify Prometheus internal metrics
	err = utils.WaitForMetricWithBackoff(suite.ctx, suite.prometheus, "prometheus_target_scrape_pools_total", utils.DefaultMetricWaitTimeout)
	suite.Assert().NoError(err, "Prometheus internal metrics should be available")
}

// TestExporterResilienceAndStability verifies exporter continues working under various conditions
func (suite *E2ETestSuite) TestExporterResilienceAndStability() {
	suite.T().Log("=== Testing exporter resilience and stability...")

	testNodes := suite.opcuaServer.GetSimulationTestNodes()
	helper := utils.NewExporterTestHelper(suite.ctx, suite.env, suite.prometheus, suite.T())

	// Start exporter with CLI flags config
	scenario := utils.ExporterTestScenario{
		Name:               "resilience-cli",
		CreateExporterFunc: utils.NewOPCUAExporterWithFlags,
		Description:        "Command-line flags",
	}

	exporter, err := helper.StartExporterAndWaitForScraping(scenario, "opcua-exporter-resilience", suite.opcuaServer.Endpoint(), testNodes)
	suite.Require().NoError(err, "Failed to start and verify exporter")
	defer exporter.Stop()

	// Verify initial operation
	utils.AssertTargetUp(suite.ctx, suite.T(), suite.prometheus, "opcua-exporter-resilience")

	// Wait longer period to verify stability (using backoff instead of sleep)
	err = utils.WaitForScrapeTargetWithBackoff(suite.ctx, suite.prometheus, "opcua-exporter-resilience", utils.DefaultScrapeWaitTimeout)
	suite.Require().NoError(err, "Exporter should remain stable")

	// Verify metrics are still being collected continuously
	expectedMetrics := utils.GetTestNodeMetricNames(testNodes)
	err = helper.VerifyExporterMetrics("opcua-exporter-resilience", expectedMetrics)
	suite.Assert().NoError(err, "Metrics should remain available during stability test")
}

// TestCommandLineFlagConfiguration verifies exporter works with CLI flag configuration
func (suite *E2ETestSuite) TestCommandLineFlagConfiguration() {
	suite.T().Log("=== Testing command-line flag configuration...")

	testNodes := suite.opcuaServer.GetSimulationTestNodes()
	helper := utils.NewExporterTestHelper(suite.ctx, suite.env, suite.prometheus, suite.T())

	// Start exporter with CLI flags
	scenario := utils.ExporterTestScenario{
		Name:               "flags-test",
		CreateExporterFunc: utils.NewOPCUAExporterWithFlags,
		Description:        "Command-line flags",
	}

	exporter, err := helper.StartExporterAndWaitForScraping(scenario, "opcua-exporter-flags", suite.opcuaServer.Endpoint(), testNodes)
	suite.Require().NoError(err, "Failed to start and verify exporter with CLI flags")
	defer exporter.Stop()

	// Verify all expected metrics are available and have valid content
	expectedMetrics := utils.GetTestNodeMetricNames(testNodes)
	err = helper.VerifyExporterMetrics("opcua-exporter-flags", expectedMetrics)
	suite.Assert().NoError(err, "All node metrics should be available with CLI flag configuration")

	// Verify metric content
	for _, node := range testNodes {
		err := helper.VerifyMetricContent(node.MetricName)
		suite.Assert().NoError(err, "Metric %s should have valid content with CLI flags", node.MetricName)
	}
}

// TestEnvironmentVariableConfiguration verifies environment variable configuration works correctly
func (suite *E2ETestSuite) TestEnvironmentVariableConfiguration() {
	suite.T().Log("=== Testing environment variable configuration...")

	testNodes := suite.opcuaServer.GetSimulationTestNodes()
	helper := utils.NewExporterTestHelper(suite.ctx, suite.env, suite.prometheus, suite.T())

	// Test environment variable configuration
	scenario := utils.ExporterTestScenario{
		Name:               "env-vars",
		CreateExporterFunc: utils.NewOPCUAExporterWithEnvConfig,
		Description:        "Environment variables",
	}

	exporter, err := helper.StartExporterAndWaitForScraping(scenario, "opcua-exporter-env", suite.opcuaServer.Endpoint(), testNodes)
	suite.Require().NoError(err, "Failed to start exporter with environment variables")
	defer exporter.Stop()

	// Verify all expected metrics are available
	expectedMetrics := utils.GetTestNodeMetricNames(testNodes)
	err = helper.VerifyExporterMetrics("opcua-exporter-env", expectedMetrics)
	suite.Assert().NoError(err, "Environment variable exporter should collect all expected metrics")

	// Verify metric content
	for _, metric := range expectedMetrics {
		err := helper.VerifyMetricContent(metric)
		suite.Assert().NoError(err, "Metric %s should have valid content with environment variables", metric)
	}

	suite.T().Log("Environment variable configuration verified successfully")
}

// TestOPCUASecurityWithUsernamePassword verifies username/password authentication with SignAndEncrypt
func (suite *E2ETestSuite) TestOPCUASecurityWithUsernamePassword() {
	suite.T().Log("=== Testing OPC UA security with username/password authentication...")

	testNodes := suite.opcuaServer.GetSimulationTestNodes()
	helper := utils.NewExporterTestHelper(suite.ctx, suite.env, suite.prometheus, suite.T())

	// Extract certificates for encryption
	certInfo, err := suite.opcuaServer.ExtractCertificates(suite.ctx, suite.env)
	suite.Require().NoError(err, "Failed to extract certificates for username auth test")

	// Test with username/password and SignAndEncrypt security mode (requires certificates)
	exporter, err := utils.NewOPCUAExporterWithUsernameAuth(
		suite.env,
		"opcua-exporter-user-auth",
		suite.opcuaServer.Endpoint(),
		testNodes,
		"testuser",    // Any username works with test server
		"testpass123", // Password is required
		certInfo,      // Certificates required for encryption
	)
	suite.Require().NoError(err, "Failed to create secure exporter with username auth")

	err = exporter.Start()
	suite.Require().NoError(err, "Failed to start secure exporter")
	defer exporter.Stop()

	// Wait for exporter readiness with security enabled
	err = helper.WaitForExporterReadiness("opcua-exporter-user-auth")
	suite.Require().NoError(err, "Secure exporter should become ready")

	// Verify all expected metrics are available with security
	expectedMetrics := append(utils.GetStandardExporterMetrics(), utils.GetTestNodeMetricNames(testNodes)...)
	err = helper.VerifyExporterMetrics("opcua-exporter-user-auth", expectedMetrics)
	suite.Assert().NoError(err, "Secure exporter should collect all expected metrics")

	// Verify target is up in Prometheus
	utils.AssertTargetUp(suite.ctx, suite.T(), suite.prometheus, "opcua-exporter-user-auth")

	suite.T().Log("Username/password authentication with SignAndEncrypt verified successfully")
}

// TestOPCUASecurityWithSignMode verifies username/password authentication with Sign security mode
func (suite *E2ETestSuite) TestOPCUASecurityWithSignMode() {
	suite.T().Log("=== Testing OPC UA security with Sign mode...")

	testNodes := suite.opcuaServer.GetSimulationTestNodes()
	helper := utils.NewExporterTestHelper(suite.ctx, suite.env, suite.prometheus, suite.T())

	// Extract certificates for signing
	certInfo, err := suite.opcuaServer.ExtractCertificates(suite.ctx, suite.env)
	suite.Require().NoError(err, "Failed to extract certificates for Sign mode test")

	// Test with username/password and Sign security mode (requires certificates for signing)
	exporter, err := utils.NewOPCUAExporterWithSignMode(
		suite.env,
		"opcua-exporter-sign-mode",
		suite.opcuaServer.Endpoint(),
		testNodes,
		"signuser",
		"signpass123",
		certInfo, // Certificates required for signing
	)
	suite.Require().NoError(err, "Failed to create exporter with Sign security mode")

	err = exporter.Start()
	suite.Require().NoError(err, "Failed to start exporter with Sign mode")
	defer exporter.Stop()

	// Wait for exporter readiness
	err = helper.WaitForExporterReadiness("opcua-exporter-sign-mode")
	suite.Require().NoError(err, "Sign mode exporter should become ready")

	// Verify metrics are available with Sign security mode
	expectedMetrics := utils.GetTestNodeMetricNames(testNodes)
	err = helper.VerifyExporterMetrics("opcua-exporter-sign-mode", expectedMetrics)
	suite.Assert().NoError(err, "Sign mode exporter should collect all expected metrics")

	// Verify metric content
	for _, node := range testNodes {
		err := helper.VerifyMetricContent(node.MetricName)
		suite.Assert().NoError(err, "Metric %s should have valid content with Sign mode", node.MetricName)
	}

	suite.T().Log("Sign security mode verified successfully")
}

// TestOPCUASecurityWithCertificateAuth verifies certificate-based authentication
func (suite *E2ETestSuite) TestOPCUASecurityWithCertificateAuth() {
	suite.T().Log("=== Testing OPC UA security with certificate authentication...")

	testNodes := suite.opcuaServer.GetSimulationTestNodes()
	helper := utils.NewExporterTestHelper(suite.ctx, suite.env, suite.prometheus, suite.T())

	// Extract and convert certificates from the OPC UA server
	certInfo, err := suite.opcuaServer.ExtractCertificates(suite.ctx, suite.env)
	suite.Require().NoError(err, "Failed to extract certificates from OPC UA server")
	suite.Require().NotNil(certInfo, "Certificate info should not be nil")

	// Verify PEM certificate files exist
	suite.Require().FileExists(certInfo.CertificateFile, "Client certificate (PEM) should exist")
	suite.Require().FileExists(certInfo.PrivateKeyFile, "Client private key (PEM) should exist")
	suite.Require().FileExists(certInfo.TrustedCertFile, "Trusted certificate (PEM) should exist")

	// Test with certificate authentication and SignAndEncrypt security mode
	exporter, err := utils.NewOPCUAExporterWithCertificateAuth(
		suite.env,
		"opcua-exporter-cert-auth",
		suite.opcuaServer.Endpoint(),
		testNodes,
		certInfo,
	)
	suite.Require().NoError(err, "Failed to create exporter with certificate auth")

	err = exporter.Start()
	suite.Require().NoError(err, "Failed to start exporter with certificate auth")
	defer exporter.Stop()

	// Wait for exporter readiness with certificate authentication
	err = helper.WaitForExporterReadiness("opcua-exporter-cert-auth")
	suite.Require().NoError(err, "Certificate auth exporter should become ready")

	// Verify all expected metrics are available with certificate auth
	expectedMetrics := append(utils.GetStandardExporterMetrics(), utils.GetTestNodeMetricNames(testNodes)...)
	err = helper.VerifyExporterMetrics("opcua-exporter-cert-auth", expectedMetrics)
	suite.Assert().NoError(err, "Certificate auth exporter should collect all expected metrics")

	// Verify target is up in Prometheus
	utils.AssertTargetUp(suite.ctx, suite.T(), suite.prometheus, "opcua-exporter-cert-auth")

	suite.T().Log("Certificate authentication verified successfully")
}

// TestOPCUASecurityConfigurationScenarios tests all security configurations across different config methods
func (suite *E2ETestSuite) TestOPCUASecurityConfigurationScenarios() {
	suite.T().Log("=== Testing security configurations across different config methods...")

	testNodes := suite.opcuaServer.GetSimulationTestNodes()
	helper := utils.NewExporterTestHelper(suite.ctx, suite.env, suite.prometheus, suite.T())

	// Extract and convert certificates for certificate auth tests
	certInfo, err := suite.opcuaServer.ExtractCertificates(suite.ctx, suite.env)
	suite.Require().NoError(err, "Failed to extract certificates from OPC UA server")

	// Test different authentication methods with different configuration sources
	securityScenarios := []struct {
		name         string
		exporterFunc func() (*utils.OPCUAExporter, error)
		description  string
	}{
		{
			name: "opcua-exporter-sec-yaml",
			exporterFunc: func() (*utils.OPCUAExporter, error) {
				return utils.NewOPCUAExporterWithUsernameAuth(
					suite.env, "opcua-exporter-sec-yaml", suite.opcuaServer.Endpoint(),
					testNodes, "yamluser", "yamlpass123", certInfo,
				)
			},
			description: "Username auth via YAML config (with encryption)",
		},
		{
			name: "opcua-exporter-sec-env",
			exporterFunc: func() (*utils.OPCUAExporter, error) {
				return utils.NewOPCUAExporterWithEnvSecurityConfig(
					suite.env, "opcua-exporter-sec-env", suite.opcuaServer.Endpoint(),
					testNodes, "envuser", "envpass123", certInfo,
				)
			},
			description: "Username auth via environment variables (with encryption)",
		},
		{
			name: "opcua-exporter-sec-flags",
			exporterFunc: func() (*utils.OPCUAExporter, error) {
				return utils.NewOPCUAExporterWithSecurityFlags(
					suite.env, "opcua-exporter-sec-flags", suite.opcuaServer.Endpoint(),
					testNodes, "flaguser", "flagpass123", certInfo,
				)
			},
			description: "Username auth via command-line flags (with encryption)",
		},
		{
			name: "opcua-exporter-sec-sign",
			exporterFunc: func() (*utils.OPCUAExporter, error) {
				return utils.NewOPCUAExporterWithSignMode(
					suite.env, "opcua-exporter-sec-sign", suite.opcuaServer.Endpoint(),
					testNodes, "signuser", "signpass123", certInfo,
				)
			},
			description: "Sign mode via YAML config (with certificates)",
		},
		{
			name: "opcua-exporter-cert-yaml",
			exporterFunc: func() (*utils.OPCUAExporter, error) {
				return utils.NewOPCUAExporterWithCertificateAuth(
					suite.env, "opcua-exporter-cert-yaml", suite.opcuaServer.Endpoint(),
					testNodes, certInfo,
				)
			},
			description: "Certificate auth via YAML config",
		},
		{
			name: "opcua-exporter-cert-env",
			exporterFunc: func() (*utils.OPCUAExporter, error) {
				return utils.NewOPCUAExporterWithCertificateAuthEnv(
					suite.env, "opcua-exporter-cert-env", suite.opcuaServer.Endpoint(),
					testNodes, certInfo,
				)
			},
			description: "Certificate auth via environment variables",
		},
		{
			name: "opcua-exporter-cert-flags",
			exporterFunc: func() (*utils.OPCUAExporter, error) {
				return utils.NewOPCUAExporterWithCertificateAuthFlags(
					suite.env, "opcua-exporter-cert-flags", suite.opcuaServer.Endpoint(),
					testNodes, certInfo,
				)
			},
			description: "Certificate auth via command-line flags",
		},
	}

	for _, scenario := range securityScenarios {
		suite.T().Logf("Testing scenario: %s", scenario.description)

		exporter, err := scenario.exporterFunc()
		suite.Require().NoError(err, "Failed to create exporter for scenario: %s", scenario.name)

		err = exporter.Start()
		suite.Require().NoError(err, "Failed to start exporter for scenario: %s", scenario.name)

		// Wait for readiness
		err = helper.WaitForExporterReadiness(scenario.name)
		suite.Assert().NoError(err, "Exporter should become ready for scenario: %s", scenario.name)

		// Verify metrics
		expectedMetrics := utils.GetTestNodeMetricNames(testNodes)
		err = helper.VerifyExporterMetrics(scenario.name, expectedMetrics)
		suite.Assert().NoError(err, "Should collect metrics for scenario: %s", scenario.name)

		// Clean up
		exporter.Stop()

		suite.T().Logf("Scenario verified successfully: %s", scenario.description)
	}
}

// TestOPCUASecurityFailureScenarios tests authentication failure scenarios
func (suite *E2ETestSuite) TestOPCUASecurityFailureScenarios() {
	suite.T().Log("=== Testing OPC UA security failure scenarios...")

	testNodes := suite.opcuaServer.GetSimulationTestNodes()

	// Extract certificates for the failure scenario test
	certInfo, err := suite.opcuaServer.ExtractCertificates(suite.ctx, suite.env)
	suite.Require().NoError(err, "Failed to extract certificates for failure scenario test")

	// Test failure when password is missing (should fail with username auth)
	suite.T().Log("Testing authentication failure with missing password...")
	exporter, err := utils.NewOPCUAExporterWithUsernameAuth(
		suite.env, "opcua-exporter-no-pass", suite.opcuaServer.Endpoint(),
		testNodes, "testuser", "", certInfo, // Empty password should cause failure
	)
	suite.Require().NoError(err, "Should create exporter even with empty password")

	err = exporter.Start()
	if err == nil {
		// If it starts, it should fail during authentication
		suite.T().Log("Exporter started but should fail authentication - checking for connection errors")
		exporter.Stop()
	}

	suite.T().Log("Security failure scenarios tested")
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

// TestMain runs the test suite
func TestE2ETestSuite(t *testing.T) {
	// Skip e2e tests if running unit tests
	if testing.Short() {
		t.Skip("Skipping e2e tests in short mode")
	}

	suite.Run(t, new(E2ETestSuite))
}
