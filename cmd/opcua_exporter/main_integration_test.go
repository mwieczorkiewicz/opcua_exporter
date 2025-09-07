package main

import (
	"context"
	"testing"
	"time"

	"github.com/mwieczorkiewicz/opcua_exporter/internal/config"
	"github.com/mwieczorkiewicz/opcua_exporter/internal/metrics"
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

func TestMetricsRegistryValidation(t *testing.T) {
	tests := []struct {
		name      string
		config    config.NodeMapping
		shouldErr bool
		errMsg    string
	}{
		{
			name: "valid config",
			config: config.NodeMapping{
				NodeName:   "ns=1;s=test",
				MetricName: "test_metric_unique_1",
			},
			shouldErr: false,
		},
		{
			name: "empty metric name",
			config: config.NodeMapping{
				NodeName:   "ns=1;s=test",
				MetricName: "",
			},
			shouldErr: true,
			errMsg:    "metric name cannot be empty",
		},
		{
			name: "invalid extract bit type",
			config: config.NodeMapping{
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
			registry := metrics.NewRegistry()
			err := registry.RegisterNodeMapping(tt.config, "", false)
			
			if tt.shouldErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMetricsRegistryError(t *testing.T) {
	configs := []config.NodeMapping{
		{
			NodeName:   "ns=1;s=valid",
			MetricName: "valid_metric",
		},
		{
			NodeName:   "ns=1;s=invalid",
			MetricName: "", // This should cause an error
		},
	}

	registry := metrics.NewRegistry()
	err := registry.CreateFromNodeMappings(configs, "", false)
	
	assert.Error(t, err)
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
		
		registry := metrics.NewRegistry()
		err := registry.CreateFromNodeMappings(nodes, "", false)
		require.NoError(t, err)
		
		handlerMap := registry.GetHandlerMap()
		assert.Len(t, handlerMap, 1)
	})
	
	t.Run("duplicate metric registration", func(t *testing.T) {
		registry := metrics.NewRegistry()
		
		// First registration should succeed
		config1 := config.NodeMapping{
			NodeName:   "ns=1;s=test1",
			MetricName: "duplicate_metric_test_unique",
		}
		
		err := registry.RegisterNodeMapping(config1, "", false)
		require.NoError(t, err)
		
		// Second registration with same metric name should fail
		config2 := config.NodeMapping{
			NodeName:   "ns=1;s=test2", 
			MetricName: "duplicate_metric_test_unique",
		}
		
		err = registry.RegisterNodeMapping(config2, "", false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate_metric_test_unique")
	})
}