package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/efficientgo/e2e"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

const (
	// OPCUAExporterDefaultImage is the default Docker image for the OPC UA exporter
	OPCUAExporterDefaultImage = "ghcr.io/mwieczorkiewicz/opcua_exporter:latest"
	// OPCUAExporterDefaultPort is the default port for the OPC UA exporter
	OPCUAExporterDefaultPort = 9686
	// OPCUAExporterBinaryPath is the path to the OPC UA exporter binary in the container
	OPCUAExporterBinaryPath = "/opcua_exporter"
	// MetricsEndpointPath is the path for the metrics endpoint
	MetricsEndpointPath = "/metrics"
)

// getOPCUAExporterImage returns the Docker image to use for the OPC UA exporter
// It checks the OPCUA_EXPORTER_E2E_IMAGE environment variable first, then falls back to the default
func getOPCUAExporterImage() string {
	if image := os.Getenv("OPCUA_EXPORTER_E2E_IMAGE"); image != "" {
		return image
	}
	return OPCUAExporterDefaultImage
}

// OPCUAExporter represents an OPC UA exporter instance
type OPCUAExporter struct {
	runnable e2e.Runnable
}

// OPCUAExporterConfig represents the configuration for the OPC UA exporter
type OPCUAExporterConfig struct {
	Port                int        `yaml:"port"`
	Endpoint            string     `yaml:"endpoint"`
	Debug               bool       `yaml:"debug"`
	SubscribeToTimeNode bool       `yaml:"subscribe-to-time-node"`
	Nodes               []TestNode `yaml:"nodes"`
}

// NewOPCUAExporter creates a new OPC UA exporter instance
func NewOPCUAExporter(env e2e.Environment, name string, opcuaServerEndpoint string, testNodes []TestNode) (*OPCUAExporter, error) {
	// Create the configuration with the provided endpoint and nodes
	config := OPCUAExporterConfig{
		Port:                OPCUAExporterDefaultPort,
		Endpoint:            opcuaServerEndpoint,
		Debug:               true,
		SubscribeToTimeNode: true,
		Nodes:               testNodes,
	}

	// Get the future runnable to access its directory
	futureRunnable := env.Runnable(name).
		WithPorts(map[string]int{
			"http": OPCUAExporterDefaultPort,
		}).
		Future()

	// Marshal the config to YAML
	configData, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write config file to the runnable directory - this gets mounted as /shared in the container
	configPath := filepath.Join(futureRunnable.Dir(), "config.yaml")
	if err := os.WriteFile(configPath, configData, 0600); err != nil {
		return nil, fmt.Errorf("failed to write config file: %w", err)
	}

	// Initialize the runnable with the config file path (mounted at the container path)
	// Clear any environment variables that might interfere with YAML config
	runnable := futureRunnable.Init(e2e.StartOptions{
		Image: getOPCUAExporterImage(),
		Command: e2e.NewCommand(
			OPCUAExporterBinaryPath,
			"--config", configPath, // Use actual container path (same as host)
		),
		Readiness: NewHTTPReadinessProbeWithExponentialBackoff("http", MetricsEndpointPath, "HTTP", 200, 299, DefaultTestTimeout),
		EnvVars: map[string]string{
			// Clear any potential environment variables that might be set
			"OPCUA_EXPORTER_NODES_0_NODENAME":   "",
			"OPCUA_EXPORTER_NODES_0_METRICNAME": "",
			"OPCUA_EXPORTER_NODES_1_NODENAME":   "",
			"OPCUA_EXPORTER_NODES_1_METRICNAME": "",
			"OPCUA_EXPORTER_NODES_2_NODENAME":   "",
			"OPCUA_EXPORTER_NODES_2_METRICNAME": "",
		},
	})

	return &OPCUAExporter{
		runnable: runnable,
	}, nil
}

// Start starts the OPC UA exporter
func (e *OPCUAExporter) Start() error {
	return e2e.StartAndWaitReady(e.runnable)
}

// Stop stops the OPC UA exporter
func (e *OPCUAExporter) Stop() error {
	return e.runnable.Stop()
}

// InternalEndpoint returns the internal endpoint for the OPC UA exporter
func (e *OPCUAExporter) InternalEndpoint() string {
	return e.runnable.InternalEndpoint("http")
}

// Endpoint returns the external endpoint for the OPC UA exporter
func (e *OPCUAExporter) Endpoint() string {
	return e.runnable.Endpoint("http")
}

// AssertRunning asserts that the OPC UA exporter is running
func (e *OPCUAExporter) AssertRunning(t assert.TestingT) {
	assert.True(t, e.runnable.IsRunning(), "OPC UA exporter should be running")
}

// GetMetricsEndpoint returns the metrics endpoint URL
func (e *OPCUAExporter) GetMetricsEndpoint() string {
	return fmt.Sprintf("http://%s%s", e.Endpoint(), MetricsEndpointPath)
}

// GenerateConfigForNodes generates a YAML configuration string for the given nodes
func GenerateConfigForNodes(endpoint string, nodes []TestNode) string {
	config := OPCUAExporterConfig{
		Port:                OPCUAExporterDefaultPort,
		Endpoint:            endpoint,
		Debug:               true,
		SubscribeToTimeNode: true,
		Nodes:               nodes,
	}

	configData, err := yaml.Marshal(config)
	if err != nil {
		return ""
	}

	return string(configData)
}

// NewOPCUAExporterWithEnvConfig creates a new OPC UA exporter using environment variables
func NewOPCUAExporterWithEnvConfig(env e2e.Environment, name string, opcuaServerEndpoint string, testNodes []TestNode) (*OPCUAExporter, error) {
	// Build environment variables for the configuration
	envVars := map[string]string{
		"OPCUA_EXPORTER_ENDPOINT":               opcuaServerEndpoint,
		"OPCUA_EXPORTER_PORT":                   fmt.Sprintf("%d", OPCUAExporterDefaultPort),
		"OPCUA_EXPORTER_DEBUG":                  "true",
		"OPCUA_EXPORTER_SUBSCRIBE_TO_TIME_NODE": "true",
	}

	// Add node configurations as environment variables
	for i, node := range testNodes {
		envVars[fmt.Sprintf("OPCUA_EXPORTER_NODES_%d_NODENAME", i)] = node.NodeName
		envVars[fmt.Sprintf("OPCUA_EXPORTER_NODES_%d_METRICNAME", i)] = node.MetricName
		if node.ExtractBit != nil {
			envVars[fmt.Sprintf("OPCUA_EXPORTER_NODES_%d_EXTRACTBIT", i)] = fmt.Sprintf("%d", *node.ExtractBit)
		}
	}

	runnable := env.Runnable(name).
		WithPorts(map[string]int{
			"http": OPCUAExporterDefaultPort,
		}).
		Init(e2e.StartOptions{
			Image:     getOPCUAExporterImage(),
			Command:   e2e.NewCommand(OPCUAExporterBinaryPath), // No --config flag = use env vars only
			Readiness: e2e.NewHTTPReadinessProbe("http", MetricsEndpointPath, 200, 299),
			EnvVars:   envVars,
		})

	return &OPCUAExporter{
		runnable: runnable,
	}, nil
}

// NewOPCUAExporterWithYAMLConfig creates a new OPC UA exporter using YAML config file
func NewOPCUAExporterWithYAMLConfig(env e2e.Environment, name string, opcuaServerEndpoint string, testNodes []TestNode) (*OPCUAExporter, error) {
	// Create the configuration
	config := OPCUAExporterConfig{
		Port:                OPCUAExporterDefaultPort,
		Endpoint:            opcuaServerEndpoint,
		Debug:               true,
		SubscribeToTimeNode: true,
		Nodes:               testNodes,
	}

	// Get the future runnable to access its directory
	futureRunnable := env.Runnable(name).
		WithPorts(map[string]int{
			"http": OPCUAExporterDefaultPort,
		}).
		Future()

	// Marshal the config to YAML
	configData, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write config file to the runnable directory
	configPath := filepath.Join(futureRunnable.Dir(), "simulation_config.yaml")
	if err := os.WriteFile(configPath, configData, 0600); err != nil {
		return nil, fmt.Errorf("failed to write config file: %w", err)
	}

	// Initialize the runnable with the config file path
	runnable := futureRunnable.Init(e2e.StartOptions{
		Image: getOPCUAExporterImage(),
		Command: e2e.NewCommand(
			OPCUAExporterBinaryPath,
			"--config", configPath, // Use actual container path (same as host)
		),
		Readiness: e2e.NewHTTPReadinessProbe("http", MetricsEndpointPath, 200, 299),
	})

	return &OPCUAExporter{
		runnable: runnable,
	}, nil
}

// NewOPCUAExporterWithFlags creates a new OPC UA exporter using command-line flags
func NewOPCUAExporterWithFlags(env e2e.Environment, name string, opcuaServerEndpoint string, testNodes []TestNode) (*OPCUAExporter, error) {
	// Build command-line arguments for the configuration
	args := []string{
		"--endpoint", opcuaServerEndpoint,
		"--port", fmt.Sprintf("%d", OPCUAExporterDefaultPort),
		"--debug",
		"--subscribe-to-time-node",
	}

	// Add node configurations as command-line flags
	for _, node := range testNodes {
		nodeFlag := fmt.Sprintf("%s,%s", node.NodeName, node.MetricName)
		if node.ExtractBit != nil {
			nodeFlag = fmt.Sprintf("%s,%d", nodeFlag, *node.ExtractBit)
		}
		args = append(args, "--node", nodeFlag)
	}

	runnable := env.Runnable(name).
		WithPorts(map[string]int{
			"http": OPCUAExporterDefaultPort,
		}).
		Init(e2e.StartOptions{
			Image:     getOPCUAExporterImage(),
			Command:   e2e.NewCommand(OPCUAExporterBinaryPath, args...),
			Readiness: e2e.NewHTTPReadinessProbe("http", MetricsEndpointPath, 200, 299),
		})

	return &OPCUAExporter{
		runnable: runnable,
	}, nil
}

// ParseOPCUAExporterLogs extracts useful information from OPC UA exporter logs
func ParseOPCUAExporterLogs(logs string) map[string]string {
	logInfo := make(map[string]string)

	lines := strings.Split(logs, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Connected to OPC UA server") {
			logInfo["connection_status"] = "connected"
		}
		if strings.Contains(line, "Subscription created") {
			logInfo["subscription_status"] = "created"
		}
		if strings.Contains(line, "Error") || strings.Contains(line, "error") {
			logInfo["error"] = line
		}
		if strings.Contains(line, "Starting HTTP server") {
			logInfo["http_server"] = "started"
		}
	}

	return logInfo
}
