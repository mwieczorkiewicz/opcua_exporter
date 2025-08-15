package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigErrorHandling(t *testing.T) {
	t.Run("malformed yaml config file", func(t *testing.T) {
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "malformed.yaml")
		
		malformedYaml := `
invalid:
  yaml:
    - missing
      closing:`
      
		err := os.WriteFile(configFile, []byte(malformedYaml), 0644)
		require.NoError(t, err)
		
		_, err = Load(configFile)
		assert.Error(t, err)
	})
	
	t.Run("empty config file", func(t *testing.T) {
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "empty.yaml")
		
		err := os.WriteFile(configFile, []byte(""), 0644)
		require.NoError(t, err)
		
		cfg, err := Load(configFile)
		assert.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Empty(t, cfg.NodeMappings)
	})
	
	t.Run("null config file", func(t *testing.T) {
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "null.yaml")
		
		err := os.WriteFile(configFile, []byte("null"), 0644)
		require.NoError(t, err)
		
		cfg, err := Load(configFile)
		assert.NoError(t, err)
		assert.NotNil(t, cfg)
	})
	
	t.Run("config file with only valid defaults", func(t *testing.T) {
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "defaults.yaml")
		
		yamlContent := `
port: 9999
endpoint: "opc.tcp://test:4840"
debug: true`
		
		err := os.WriteFile(configFile, []byte(yamlContent), 0644)
		require.NoError(t, err)
		
		cfg, err := Load(configFile)
		assert.NoError(t, err)
		assert.Equal(t, 9999, cfg.Port)
		assert.Equal(t, "opc.tcp://test:4840", cfg.Endpoint)
		assert.True(t, cfg.Debug)
		assert.Empty(t, cfg.NodeMappings)
	})
}

func TestNodeMappingValidation(t *testing.T) {
	tests := []struct {
		name     string
		mapping  NodeMapping
		isValid  bool
	}{
		{
			name: "valid mapping",
			mapping: NodeMapping{
				NodeName:   "ns=1;s=Temperature",
				MetricName: "temperature_celsius",
			},
			isValid: true,
		},
		{
			name: "valid mapping with extract bit",
			mapping: NodeMapping{
				NodeName:   "ns=1;s=AlarmBits",
				MetricName: "alarm_bit_0",
				ExtractBit: 0,
			},
			isValid: true,
		},
		{
			name: "empty node name",
			mapping: NodeMapping{
				NodeName:   "",
				MetricName: "test_metric",
			},
			isValid: false,
		},
		{
			name: "empty metric name", 
			mapping: NodeMapping{
				NodeName:   "ns=1;s=test",
				MetricName: "",
			},
			isValid: false,
		},
		{
			name: "negative extract bit",
			mapping: NodeMapping{
				NodeName:   "ns=1;s=test",
				MetricName: "test_metric",
				ExtractBit: -1,
			},
			isValid: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mapping.Validate()
			if tt.isValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestFilterValidNodeMappingsWithValidation(t *testing.T) {
	mappings := []NodeMapping{
		{NodeName: "valid1", MetricName: "metric1"},
		{NodeName: "", MetricName: "metric2"}, // Invalid: empty node name
		{NodeName: "valid3", MetricName: ""}, // Invalid: empty metric name
		{NodeName: "valid4", MetricName: "metric4"},
		{NodeName: "valid5", MetricName: "metric5", ExtractBit: -1}, // Filter doesn't check extractBit
	}
	
	// Test that our filter function removes mappings with empty names
	valid := filterValidNodeMappings(mappings)
	assert.Equal(t, 3, len(valid)) // valid1, valid4, and valid5 should remain
	
	// The filter only checks for empty strings, not extractBit validation
	assert.Equal(t, "valid1", valid[0].NodeName)
	assert.Equal(t, "valid4", valid[1].NodeName) 
	assert.Equal(t, "valid5", valid[2].NodeName)
	
	// Test validation separately - some filtered mappings may still fail validation
	validCount := 0
	for _, mapping := range valid {
		if mapping.Validate() == nil {
			validCount++
		}
	}
	assert.Equal(t, 2, validCount) // Only valid1 and valid4 pass validation
}

// Add validation method to NodeMapping
func (n *NodeMapping) Validate() error {
	if n.NodeName == "" {
		return fmt.Errorf("nodeName cannot be empty")
	}
	if n.MetricName == "" {
		return fmt.Errorf("metricName cannot be empty")
	}
	if n.ExtractBit != nil {
		if bit, ok := n.ExtractBit.(int); ok && bit < 0 {
			return fmt.Errorf("extractBit must be non-negative, got %d", bit)
		}
	}
	return nil
}