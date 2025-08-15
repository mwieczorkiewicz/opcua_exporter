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
	OPCUAExporterImage = "ghcr.io/mwieczorkiewicz/opcua_exporter:latest"
	OPCUAExporterPort  = 9686
)

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
		Port:                OPCUAExporterPort,
		Endpoint:            opcuaServerEndpoint,
		Debug:               true,
		SubscribeToTimeNode: true,
		Nodes:               testNodes,
	}

	// Get the future runnable to access its directory
	futureRunnable := env.Runnable(name).
		WithPorts(map[string]int{
			"http": OPCUAExporterPort,
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
		Image: OPCUAExporterImage,
		Command: e2e.NewCommand(
			"/opcua_exporter",
			"--config", filepath.Join(futureRunnable.Dir(), "config.yaml"),
		),
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
	return fmt.Sprintf("http://%s/metrics", e.Endpoint())
}

// GenerateConfigForNodes generates a YAML configuration string for the given nodes
func GenerateConfigForNodes(endpoint string, nodes []TestNode) string {
	config := OPCUAExporterConfig{
		Port:                OPCUAExporterPort,
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
		"OPCUA_EXPORTER_ENDPOINT":              opcuaServerEndpoint,
		"OPCUA_EXPORTER_PORT":                  fmt.Sprintf("%d", OPCUAExporterPort),
		"OPCUA_EXPORTER_DEBUG":                 "true",
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
			"http": OPCUAExporterPort,
		}).
		Init(e2e.StartOptions{
			Image:   OPCUAExporterImage,
			Command: e2e.NewCommand("/opcua_exporter"),
			EnvVars: envVars,
		})

	return &OPCUAExporter{
		runnable: runnable,
	}, nil
}

// NewOPCUAExporterWithYAMLConfig creates a new OPC UA exporter using YAML config file
func NewOPCUAExporterWithYAMLConfig(env e2e.Environment, name string, opcuaServerEndpoint string, testNodes []TestNode) (*OPCUAExporter, error) {
	// Create the configuration
	config := OPCUAExporterConfig{
		Port:                OPCUAExporterPort,
		Endpoint:            opcuaServerEndpoint,
		Debug:               true,
		SubscribeToTimeNode: true,
		Nodes:               testNodes,
	}

	// Get the future runnable to access its directory
	futureRunnable := env.Runnable(name).
		WithPorts(map[string]int{
			"http": OPCUAExporterPort,
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
		Image: OPCUAExporterImage,
		Command: e2e.NewCommand(
			"/opcua_exporter",
			"--config", "/shared/simulation_config.yaml",
		),
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
		"--port", fmt.Sprintf("%d", OPCUAExporterPort),
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
			"http": OPCUAExporterPort,
		}).
		Init(e2e.StartOptions{
			Image:   OPCUAExporterImage,
			Command: e2e.NewCommand("/opcua_exporter", args...),
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
