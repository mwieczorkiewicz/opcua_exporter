package config

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigErrorHandling(t *testing.T) {
	t.Run("invalid base64", func(t *testing.T) {
		invalidB64 := "invalid-base64!!!"
		nodes, err := ReadBase64(&invalidB64)
		
		assert.Error(t, err)
		assert.Nil(t, nodes)
		assert.Contains(t, err.Error(), "failed to decode base64 config")
	})
	
	t.Run("malformed yaml", func(t *testing.T) {
		malformedYaml := `
invalid:
  yaml:
    - missing
      closing:
`
		nodes, err := parse(strings.NewReader(malformedYaml))
		
		assert.Error(t, err)
		assert.Nil(t, nodes)
	})
	
	t.Run("empty config", func(t *testing.T) {
		nodes, err := parse(strings.NewReader(""))
		
		assert.NoError(t, err)
		assert.Empty(t, nodes)
	})
	
	t.Run("null config", func(t *testing.T) {
		nodes, err := parse(strings.NewReader("null"))
		
		assert.NoError(t, err)
		assert.Nil(t, nodes)
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