package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

var testNodes = []NodeMapping{
	{
		NodeName:   "foo",
		MetricName: "bar",
	},
	{
		NodeName:   "baz",
		MetricName: "bak",
		ExtractBit: 4,
	},
}

var testConfig = map[string]interface{}{
	"port":     9999,
	"endpoint": "opc.tcp://test:4840",
	"debug":    true,
	"nodes":    testNodes,
}

func TestLoadConfigFile(t *testing.T) {
	// Create temporary config file
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test_config.yaml")
	
	data, err := yaml.Marshal(testConfig)
	require.NoError(t, err)
	
	err = os.WriteFile(configFile, data, 0644)
	require.NoError(t, err)
	
	// Test loading config file
	cfg, err := Load(configFile)
	assert.NoError(t, err)
	assert.Equal(t, 9999, cfg.Port)
	assert.Equal(t, "opc.tcp://test:4840", cfg.Endpoint)
	assert.True(t, cfg.Debug)
	assert.Equal(t, 2, len(cfg.NodeMappings))
	assert.Equal(t, "foo", cfg.NodeMappings[0].NodeName)
	assert.Equal(t, "bar", cfg.NodeMappings[0].MetricName)
	assert.Nil(t, cfg.NodeMappings[0].ExtractBit)
	assert.Equal(t, 4, cfg.NodeMappings[1].ExtractBit)
}

func TestLoadConfigFileNotFound(t *testing.T) {
	// Test non-existent config file - should now warn but not fail
	cfg, err := Load("/path/that/does/not/exist.yaml")
	assert.NoError(t, err, "Should not fail when config file doesn't exist")
	assert.NotNil(t, cfg, "Should return default config when file not found")
	assert.Equal(t, 9686, cfg.Port, "Should have default port")
	assert.Equal(t, "opc.tcp://localhost:4096", cfg.Endpoint, "Should have default endpoint")
}

func TestLoadConfigDefaults(t *testing.T) {
	// Test loading without config file (defaults only)
	cfg, err := Load("")
	assert.NoError(t, err)
	assert.Equal(t, 9686, cfg.Port)
	assert.Equal(t, "opc.tcp://localhost:4096", cfg.Endpoint)
	assert.False(t, cfg.Debug)
	assert.Equal(t, 5*time.Second, cfg.ReadTimeout)
	assert.Equal(t, 0, cfg.MaxTimeouts)
	assert.Equal(t, 64, cfg.BufferSize)
	assert.Equal(t, 5*time.Minute, cfg.SummaryInterval)
	assert.False(t, cfg.SubscribeToTimeNode)
	assert.Empty(t, cfg.NodeMappings)
}

func TestLoadEnvironmentVariables(t *testing.T) {
	// Set environment variables
	os.Setenv("OPCUA_EXPORTER_PORT", "8888")
	os.Setenv("OPCUA_EXPORTER_ENDPOINT", "opc.tcp://env:4840")
	os.Setenv("OPCUA_EXPORTER_DEBUG", "true")
	os.Setenv("OPCUA_EXPORTER_NODES_0_NODENAME", "ns=1;s=Test")
	os.Setenv("OPCUA_EXPORTER_NODES_0_METRICNAME", "test_metric")
	os.Setenv("OPCUA_EXPORTER_NODES_1_NODENAME", "ns=1;s=AlarmBits")
	os.Setenv("OPCUA_EXPORTER_NODES_1_METRICNAME", "alarm_bit_5")
	os.Setenv("OPCUA_EXPORTER_NODES_1_EXTRACTBIT", "5")
	
	defer func() {
		os.Unsetenv("OPCUA_EXPORTER_PORT")
		os.Unsetenv("OPCUA_EXPORTER_ENDPOINT")
		os.Unsetenv("OPCUA_EXPORTER_DEBUG")
		os.Unsetenv("OPCUA_EXPORTER_NODES_0_NODENAME")
		os.Unsetenv("OPCUA_EXPORTER_NODES_0_METRICNAME")
		os.Unsetenv("OPCUA_EXPORTER_NODES_1_NODENAME")
		os.Unsetenv("OPCUA_EXPORTER_NODES_1_METRICNAME")
		os.Unsetenv("OPCUA_EXPORTER_NODES_1_EXTRACTBIT")
	}()
	
	cfg, err := Load("")
	assert.NoError(t, err)
	assert.Equal(t, 8888, cfg.Port)
	assert.Equal(t, "opc.tcp://env:4840", cfg.Endpoint)
	assert.True(t, cfg.Debug)
	assert.Equal(t, 2, len(cfg.NodeMappings))
	assert.Equal(t, "ns=1;s=Test", cfg.NodeMappings[0].NodeName)
	assert.Equal(t, "test_metric", cfg.NodeMappings[0].MetricName)
	assert.Nil(t, cfg.NodeMappings[0].ExtractBit)
	assert.Equal(t, "ns=1;s=AlarmBits", cfg.NodeMappings[1].NodeName)
	assert.Equal(t, "alarm_bit_5", cfg.NodeMappings[1].MetricName)
	assert.Equal(t, 5, cfg.NodeMappings[1].ExtractBit)
}

func TestAddNodeMapping(t *testing.T) {
	cfg, err := Load("")
	assert.NoError(t, err)
	assert.Empty(t, cfg.NodeMappings)
	
	nodeMapping := NodeMapping{
		NodeName:   "ns=1;s=Test",
		MetricName: "test_metric",
		ExtractBit: 3,
	}
	
	cfg.AddNodeMapping(nodeMapping)
	assert.Equal(t, 1, len(cfg.NodeMappings))
	assert.Equal(t, nodeMapping, cfg.NodeMappings[0])
}

func TestFilterValidNodeMappings(t *testing.T) {
	mappings := []NodeMapping{
		{NodeName: "valid1", MetricName: "metric1"},
		{NodeName: "", MetricName: "metric2"}, // Invalid: empty node name
		{NodeName: "valid3", MetricName: ""}, // Invalid: empty metric name
		{NodeName: "valid4", MetricName: "metric4"},
	}
	
	valid := filterValidNodeMappings(mappings)
	assert.Equal(t, 2, len(valid))
	assert.Equal(t, "valid1", valid[0].NodeName)
	assert.Equal(t, "valid4", valid[1].NodeName)
}

func TestInvalidYAMLConfig(t *testing.T) {
	// Create temporary config file with invalid YAML
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "invalid_config.yaml")
	
	invalidYAML := "port: 9999\nendpoint: \"test\"\nnodes:\n  - nodeName: \"test\n    metricName: unterminated"
	err := os.WriteFile(configFile, []byte(invalidYAML), 0644)
	require.NoError(t, err)
	
	// Test loading invalid config file
	_, err = Load(configFile)
	assert.Error(t, err)
}

func TestConfigFilePriority(t *testing.T) {
	// Create config file with specific values
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "priority_test.yaml")
	
	configData := map[string]interface{}{
		"port":     7777,
		"endpoint": "opc.tcp://configfile:4840",
		"debug":    false,
	}
	
	data, err := yaml.Marshal(configData)
	require.NoError(t, err)
	
	err = os.WriteFile(configFile, data, 0644)
	require.NoError(t, err)
	
	// Set environment variable that should override config file
	os.Setenv("OPCUA_EXPORTER_PORT", "6666")
	defer os.Unsetenv("OPCUA_EXPORTER_PORT")
	
	cfg, err := Load(configFile)
	assert.NoError(t, err)
	
	// Environment variable should override config file
	assert.Equal(t, 6666, cfg.Port)
	// Config file value should be used where no env var exists
	assert.Equal(t, "opc.tcp://configfile:4840", cfg.Endpoint)
	assert.False(t, cfg.Debug)
}