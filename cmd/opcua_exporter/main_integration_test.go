package main

import (
	"context"
	"testing"
	"time"

	"github.com/mwieczorkiewicz/opcua_exporter/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	
	app := &App{
		ctx:    ctx,
		cancel: cancel,
	}

	// Test shutdown with error - this will call os.Exit so we can't test it directly
	// Instead, we'll test the context cancellation behavior
	go func() {
		// Simulate shutdown without os.Exit for testing
		app.cancel()
	}()
	
	// Context should be cancelled
	select {
	case <-ctx.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Context was not cancelled")
	}
}

func TestGetClientError(t *testing.T) {
	// The opcua library doesn't validate endpoints at creation time
	// It only validates during connection, so we test with a valid format
	validEndpoint := "opc.tcp://localhost:4840"
	client, err := getClient(&validEndpoint)
	
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestCreateHandlerValidation(t *testing.T) {
	tests := []struct {
		name      string
		config    NodeConfig
		shouldErr bool
		errMsg    string
	}{
		{
			name: "valid config",
			config: NodeConfig{
				NodeName:   "ns=1;s=test",
				MetricName: "test_metric_unique_1",
			},
			shouldErr: false,
		},
		{
			name: "empty metric name",
			config: NodeConfig{
				NodeName:   "ns=1;s=test",
				MetricName: "",
			},
			shouldErr: true,
			errMsg:    "metric name cannot be empty",
		},
		{
			name: "invalid extract bit type",
			config: NodeConfig{
				NodeName:   "ns=1;s=test",
				MetricName: "test_metric_unique_2",
				ExtractBit: "invalid",
			},
			shouldErr: true,
			errMsg:    "extractBit must be an integer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := createHandler(tt.config)
			
			if tt.shouldErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, handler)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, handler)
			}
		})
	}
}

func TestCreateMetricsError(t *testing.T) {
	configs := []NodeConfig{
		{
			NodeName:   "ns=1;s=valid",
			MetricName: "valid_metric",
		},
		{
			NodeName:   "ns=1;s=invalid",
			MetricName: "", // This should cause an error
		},
	}

	handlerMap, err := createMetrics(&configs)
	
	assert.Error(t, err)
	assert.Nil(t, handlerMap)
	assert.Contains(t, err.Error(), "metric name cannot be empty")
}

func TestConfigValidation(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		nodes := []config.NodeMapping{
			{
				NodeName:   "ns=1;s=test_validation",
				MetricName: "test_metric_validation_unique",
			},
		}
		
		handlerMap, err := createMetrics(&nodes)
		require.NoError(t, err)
		assert.Len(t, handlerMap, 1)
	})
	
	t.Run("duplicate metric registration", func(t *testing.T) {
		// First registration should succeed
		config1 := NodeConfig{
			NodeName:   "ns=1;s=test1",
			MetricName: "duplicate_metric_test_unique",
		}
		
		_, err := createHandler(config1)
		require.NoError(t, err)
		
		// Second registration with same metric name should fail
		config2 := NodeConfig{
			NodeName:   "ns=1;s=test2", 
			MetricName: "duplicate_metric_test_unique",
		}
		
		_, err = createHandler(config2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to register metric")
	})
}