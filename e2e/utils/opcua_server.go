package utils

import (
	"context"
	"fmt"
	"time"

	"github.com/efficientgo/e2e"
	"github.com/stretchr/testify/assert"
)

const (
	// OPCUAServerImage is the Docker image for the test OPC UA server
	OPCUAServerImage = "mcr.microsoft.com/iot/opc-ua-test-server:2.8"
	// OPCUAServerDefaultPort is the default port for the OPC UA server
	OPCUAServerDefaultPort = 4840
)

// OPCUAServer represents a test OPC UA server instance
type OPCUAServer struct {
	runnable e2e.Runnable
}

// NewOPCUAServer creates a new OPC UA test server instance with proper readiness probe
func NewOPCUAServer(env e2e.Environment, name string) *OPCUAServer {
	runnable := env.Runnable(name).
		WithPorts(map[string]int{
			"opcua": OPCUAServerDefaultPort,
		}).
		Init(e2e.StartOptions{
			Image:     OPCUAServerImage,
			Command:   e2e.NewCommand("--sample", "--port", fmt.Sprintf("%d", OPCUAServerDefaultPort)),
			Readiness: e2e.NewTCPReadinessProbe("opcua"),
		})

	return &OPCUAServer{
		runnable: runnable,
	}
}

// Start starts the OPC UA server and waits for it to be ready
func (s *OPCUAServer) Start(ctx context.Context) error {
	if err := e2e.StartAndWaitReady(s.runnable); err != nil {
		return fmt.Errorf("failed to start OPC UA server: %w", err)
	}

	// Additional wait for OPC UA server to fully initialize its endpoints
	// The readiness probe ensures TCP connectivity, but OPC UA needs more time
	return waitForTCPPortWithBackoff(s.runnable.Endpoint("opcua"), 10*time.Second)
}

// Stop stops the OPC UA server
func (s *OPCUAServer) Stop() error {
	return s.runnable.Stop()
}

// Endpoint returns the OPC UA endpoint URL
func (s *OPCUAServer) Endpoint() string {
	return fmt.Sprintf("opc.tcp://%s", s.runnable.InternalEndpoint("opcua"))
}

// WaitForReady waits for the OPC UA server to be ready to accept connections
func (s *OPCUAServer) WaitForReady(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for OPC UA server to be ready")
		case <-ticker.C:
			// Check if container is running and healthy
			if s.runnable.IsRunning() {
				// Additional check: verify the port is accessible via external endpoint
				if err := waitForTCPPort(s.runnable.Endpoint("opcua"), 5*time.Second); err == nil {
					return nil
				}
			}
		}
	}
}

// AssertRunning asserts that the OPC UA server is running
func (s *OPCUAServer) AssertRunning(t assert.TestingT) {
	assert.True(t, s.runnable.IsRunning(), "OPC UA server should be running")
}

// GetSimulationTestNodes returns a list of test node configurations for simulation testing
func (s *OPCUAServer) GetSimulationTestNodes() []TestNode {
	return []TestNode{
		{
			NodeName:   "ns=21;i=1244",
			MetricName: "flow_indicator_output",
		},
		{
			NodeName:   "ns=21;i=1259",
			MetricName: "level_indicator_output",
		},
		{
			NodeName:   "ns=21;i=1267",
			MetricName: "flow_indicator_2_output",
		},
	}
}

// TestNode represents a test node configuration
type TestNode struct {
	NodeName   string `yaml:"nodeName"`
	MetricName string `yaml:"metricName"`
	ExtractBit *int   `yaml:"extractBit,omitempty"`
}

// ToConfigString converts TestNode to config string format
func (n TestNode) ToConfigString() string {
	if n.ExtractBit != nil {
		return fmt.Sprintf("nodeName: %s\nmetricName: %s\nextractBit: %d", n.NodeName, n.MetricName, *n.ExtractBit)
	}
	return fmt.Sprintf("nodeName: %s\nmetricName: %s", n.NodeName, n.MetricName)
}
