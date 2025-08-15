package main

import (
	"testing"

	"github.com/mwieczorkiewicz/opcua_exporter/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestParseNodeFlag(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		expected config.NodeMapping
		hasError bool
	}{
		{
			name: "valid node without extractBit",
			flag: "ns=1;s=Test,test_metric",
			expected: config.NodeMapping{
				NodeName:   "ns=1;s=Test",
				MetricName: "test_metric",
			},
			hasError: false,
		},
		{
			name: "valid node with extractBit",
			flag: "ns=1;s=AlarmBits,alarm_bit_5,5",
			expected: config.NodeMapping{
				NodeName:   "ns=1;s=AlarmBits",
				MetricName: "alarm_bit_5",
				ExtractBit: 5,
			},
			hasError: false,
		},
		{
			name: "valid node with spaces",
			flag: " ns=1;s=Test , test_metric , 3 ",
			expected: config.NodeMapping{
				NodeName:   "ns=1;s=Test",
				MetricName: "test_metric",
				ExtractBit: 3,
			},
			hasError: false,
		},
		{
			name: "valid node with empty extractBit",
			flag: "ns=1;s=Test,test_metric,",
			expected: config.NodeMapping{
				NodeName:   "ns=1;s=Test",
				MetricName: "test_metric",
			},
			hasError: false,
		},
		{
			name:     "invalid - missing metric name",
			flag:     "ns=1;s=Test",
			hasError: true,
		},
		{
			name:     "invalid - empty flag",
			flag:     "",
			hasError: true,
		},
		{
			name:     "invalid - invalid extractBit",
			flag:     "ns=1;s=Test,test_metric,invalid",
			hasError: true,
		},
		{
			name:     "invalid - non-numeric extractBit",
			flag:     "ns=1;s=Test,test_metric,abc",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseNodeFlag(tt.flag)
			
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected.NodeName, result.NodeName)
				assert.Equal(t, tt.expected.MetricName, result.MetricName)
				assert.Equal(t, tt.expected.ExtractBit, result.ExtractBit)
			}
		})
	}
}