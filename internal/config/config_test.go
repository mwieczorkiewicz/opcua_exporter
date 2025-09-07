package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v2"
)

type ConfigTestSuite struct {
	suite.Suite
	tempDir    string
	configFile string
}

func (s *ConfigTestSuite) SetupTest() {
	s.tempDir = s.T().TempDir()
	s.configFile = filepath.Join(s.tempDir, "test_config.yaml")
	s.clearEnvVars()
}

func (s *ConfigTestSuite) TearDownTest() {
	s.clearEnvVars()
	if s.configFile != "" {
		os.Remove(s.configFile)
	}
}

func (s *ConfigTestSuite) clearEnvVars() {
	envVars := []string{
		"OPCUA_EXPORTER_PORT",
		"OPCUA_EXPORTER_ENDPOINT",
		"OPCUA_EXPORTER_PROM_PREFIX",
		"OPCUA_EXPORTER_DEBUG",
		"OPCUA_EXPORTER_READ_TIMEOUT",
		"OPCUA_EXPORTER_MAX_TIMEOUTS",
		"OPCUA_EXPORTER_BUFFER_SIZE",
		"OPCUA_EXPORTER_SUMMARY_INTERVAL",
		"OPCUA_EXPORTER_SUBSCRIBE_TO_TIME_NODE",
	}
	
	for i := 0; i < 10; i++ {
		envVars = append(envVars,
			"OPCUA_EXPORTER_NODES_"+string(rune('0'+i))+"_NODENAME",
			"OPCUA_EXPORTER_NODES_"+string(rune('0'+i))+"_METRICNAME",
			"OPCUA_EXPORTER_NODES_"+string(rune('0'+i))+"_EXTRACTBIT",
		)
	}
	
	for _, envVar := range envVars {
		os.Unsetenv(envVar)
	}
}

func (s *ConfigTestSuite) writeYAMLConfig(config interface{}) {
	data, err := yaml.Marshal(config)
	s.Require().NoError(err)
	err = os.WriteFile(s.configFile, data, 0644)
	s.Require().NoError(err)
}

func (s *ConfigTestSuite) TestLoadConfigFile() {
	testConfig := map[string]interface{}{
		"port":     9999,
		"endpoint": "opc.tcp://test:4840",
		"debug":    true,
		"nodes": []map[string]interface{}{
			{"nodeName": "foo", "metricName": "bar"},
			{"nodeName": "baz", "metricName": "bak", "extractBit": 4},
		},
	}
	
	s.writeYAMLConfig(testConfig)
	
	cfg, err := Load(s.configFile)
	s.Assert().NoError(err)
	s.Assert().Equal(9999, cfg.Port)
	s.Assert().Equal("opc.tcp://test:4840", cfg.Endpoint)
	s.Assert().True(cfg.Debug)
	s.Assert().Len(cfg.NodeMappings, 2)
	s.Assert().Equal("foo", cfg.NodeMappings[0].NodeName)
	s.Assert().Equal("bar", cfg.NodeMappings[0].MetricName)
	s.Assert().Nil(cfg.NodeMappings[0].ExtractBit)
	s.Assert().Equal(4, cfg.NodeMappings[1].ExtractBit)
}

func (s *ConfigTestSuite) TestLoadConfigFileNotFound() {
	cfg, err := Load("/path/that/does/not/exist.yaml")
	s.Assert().NoError(err)
	s.Assert().NotNil(cfg)
	s.Assert().Equal(9686, cfg.Port)
	s.Assert().Equal("opc.tcp://localhost:4096", cfg.Endpoint)
}

func (s *ConfigTestSuite) TestLoadConfigDefaults() {
	cfg, err := Load("")
	s.Assert().NoError(err)
	s.Assert().Equal(9686, cfg.Port)
	s.Assert().Equal("opc.tcp://localhost:4096", cfg.Endpoint)
	s.Assert().False(cfg.Debug)
	s.Assert().Equal(5*time.Second, cfg.ReadTimeout)
	s.Assert().Equal(0, cfg.MaxTimeouts)
	s.Assert().Equal(64, cfg.BufferSize)
	s.Assert().Equal(5*time.Minute, cfg.SummaryInterval)
	s.Assert().False(cfg.SubscribeToTimeNode)
	s.Assert().Empty(cfg.NodeMappings)
}

func (s *ConfigTestSuite) TestLoadEnvironmentVariables() {
	envVars := map[string]string{
		"OPCUA_EXPORTER_PORT":                   "8888",
		"OPCUA_EXPORTER_ENDPOINT":               "opc.tcp://env:4840",
		"OPCUA_EXPORTER_DEBUG":                  "true",
		"OPCUA_EXPORTER_NODES_0_NODENAME":       "ns=1;s=Test",
		"OPCUA_EXPORTER_NODES_0_METRICNAME":     "test_metric",
		"OPCUA_EXPORTER_NODES_1_NODENAME":       "ns=1;s=AlarmBits",
		"OPCUA_EXPORTER_NODES_1_METRICNAME":     "alarm_bit_5",
		"OPCUA_EXPORTER_NODES_1_EXTRACTBIT":     "5",
	}
	
	for key, value := range envVars {
		s.Require().NoError(os.Setenv(key, value))
	}
	
	cfg, err := Load("")
	s.Assert().NoError(err)
	s.Assert().Equal(8888, cfg.Port)
	s.Assert().Equal("opc.tcp://env:4840", cfg.Endpoint)
	s.Assert().True(cfg.Debug)
	s.Assert().Len(cfg.NodeMappings, 2)
	s.Assert().Equal("ns=1;s=Test", cfg.NodeMappings[0].NodeName)
	s.Assert().Equal("test_metric", cfg.NodeMappings[0].MetricName)
	s.Assert().Nil(cfg.NodeMappings[0].ExtractBit)
	s.Assert().Equal("ns=1;s=AlarmBits", cfg.NodeMappings[1].NodeName)
	s.Assert().Equal("alarm_bit_5", cfg.NodeMappings[1].MetricName)
	s.Assert().Equal(5, cfg.NodeMappings[1].ExtractBit)
}

func (s *ConfigTestSuite) TestAddNodeMapping() {
	cfg, err := Load("")
	s.Assert().NoError(err)
	s.Assert().Empty(cfg.NodeMappings)
	
	nodeMapping := NodeMapping{
		NodeName:   "ns=1;s=Test",
		MetricName: "test_metric",
		ExtractBit: 3,
	}
	
	cfg.AddNodeMapping(nodeMapping)
	s.Assert().Len(cfg.NodeMappings, 1)
	s.Assert().Equal(nodeMapping, cfg.NodeMappings[0])
}

func (s *ConfigTestSuite) TestFilterValidNodeMappings() {
	mappings := []NodeMapping{
		{NodeName: "valid1", MetricName: "metric1"},
		{NodeName: "", MetricName: "metric2"},
		{NodeName: "valid3", MetricName: ""},
		{NodeName: "valid4", MetricName: "metric4"},
	}
	
	valid := filterValidNodeMappings(mappings)
	s.Assert().Len(valid, 2)
	s.Assert().Equal("valid1", valid[0].NodeName)
	s.Assert().Equal("valid4", valid[1].NodeName)
}

func (s *ConfigTestSuite) TestInvalidYAMLConfig() {
	invalidYAML := "port: 9999\nendpoint: \"test\"\nnodes:\n  - nodeName: \"test\n    metricName: unterminated"
	err := os.WriteFile(s.configFile, []byte(invalidYAML), 0644)
	s.Require().NoError(err)
	
	_, err = Load(s.configFile)
	s.Assert().Error(err)
}

func (s *ConfigTestSuite) TestConfigFilePriority() {
	configData := map[string]interface{}{
		"port":     7777,
		"endpoint": "opc.tcp://configfile:4840",
		"debug":    false,
	}
	
	s.writeYAMLConfig(configData)
	
	s.Require().NoError(os.Setenv("OPCUA_EXPORTER_PORT", "6666"))
	
	cfg, err := Load(s.configFile)
	s.Assert().NoError(err)
	s.Assert().Equal(6666, cfg.Port)
	s.Assert().Equal("opc.tcp://configfile:4840", cfg.Endpoint)
	s.Assert().False(cfg.Debug)
}

func (s *ConfigTestSuite) TestYAMLConfigLoading() {
	testConfig := map[string]interface{}{
		"port":                 8080,
		"endpoint":            "opc.tcp://full-server:4840",
		"promPrefix":          "factory",
		"debug":               true,
		"readTimeout":         "10s",
		"maxTimeouts":         5,
		"bufferSize":          128,
		"summaryInterval":     "10m",
		"subscribeToTimeNode": true,
		"nodes": []map[string]interface{}{
			{
				"nodeName":   "ns=1;s=Temperature",
				"metricName": "temperature_celsius",
			},
			{
				"nodeName":   "ns=1;s=AlarmBits",
				"metricName": "alarm_bit_5",
				"extractBit": 5,
			},
		},
	}

	s.writeYAMLConfig(testConfig)

	cfg, err := Load(s.configFile)
	s.Assert().NoError(err)
	s.Assert().Equal(8080, cfg.Port)
	s.Assert().Equal("opc.tcp://full-server:4840", cfg.Endpoint)
	s.Assert().Equal("factory", cfg.PromPrefix)
	s.Assert().True(cfg.Debug)
	s.Assert().Equal(10*time.Second, cfg.ReadTimeout)
	s.Assert().Equal(5, cfg.MaxTimeouts)
	s.Assert().Equal(128, cfg.BufferSize)
	s.Assert().Equal(10*time.Minute, cfg.SummaryInterval)
	s.Assert().True(cfg.SubscribeToTimeNode)

	s.Require().Len(cfg.NodeMappings, 2)
	s.Assert().Equal("ns=1;s=Temperature", cfg.NodeMappings[0].NodeName)
	s.Assert().Equal("temperature_celsius", cfg.NodeMappings[0].MetricName)
	s.Assert().Nil(cfg.NodeMappings[0].ExtractBit)
	s.Assert().Equal("ns=1;s=AlarmBits", cfg.NodeMappings[1].NodeName)
	s.Assert().Equal("alarm_bit_5", cfg.NodeMappings[1].MetricName)
	s.Assert().Equal(5, cfg.NodeMappings[1].ExtractBit)
}

func (s *ConfigTestSuite) TestErrorHandling() {
	s.Run("malformed yaml", func() {
		malformedYAML := `
invalid:
  yaml:
    - missing
      closing:`

		err := os.WriteFile(s.configFile, []byte(malformedYAML), 0644)
		s.Require().NoError(err)

		_, err = Load(s.configFile)
		s.Assert().Error(err)
	})

	s.Run("empty config file", func() {
		err := os.WriteFile(s.configFile, []byte(""), 0644)
		s.Require().NoError(err)

		cfg, err := Load(s.configFile)
		s.Assert().NoError(err)
		s.Assert().NotNil(cfg)
		s.Assert().Empty(cfg.NodeMappings)
	})

	s.Run("null config file", func() {
		err := os.WriteFile(s.configFile, []byte("null"), 0644)
		s.Require().NoError(err)

		cfg, err := Load(s.configFile)
		s.Assert().NoError(err)
		s.Assert().NotNil(cfg)
	})
}

func (s *ConfigTestSuite) TestCompleteEnvironmentConfig() {
	envVars := map[string]string{
		"OPCUA_EXPORTER_PORT":                    "7777",
		"OPCUA_EXPORTER_ENDPOINT":                "opc.tcp://all-env:4840",
		"OPCUA_EXPORTER_PROM_PREFIX":             "plant",
		"OPCUA_EXPORTER_DEBUG":                   "true",
		"OPCUA_EXPORTER_READ_TIMEOUT":            "15s",
		"OPCUA_EXPORTER_MAX_TIMEOUTS":            "3",
		"OPCUA_EXPORTER_BUFFER_SIZE":             "256",
		"OPCUA_EXPORTER_SUMMARY_INTERVAL":        "15m",
		"OPCUA_EXPORTER_SUBSCRIBE_TO_TIME_NODE":  "true",
	}

	for key, value := range envVars {
		s.Require().NoError(os.Setenv(key, value))
	}

	cfg, err := Load("")
	s.Assert().NoError(err)
	s.Assert().Equal(7777, cfg.Port)
	s.Assert().Equal("opc.tcp://all-env:4840", cfg.Endpoint)
	s.Assert().Equal("plant", cfg.PromPrefix)
	s.Assert().True(cfg.Debug)
	s.Assert().Equal(15*time.Second, cfg.ReadTimeout)
	s.Assert().Equal(3, cfg.MaxTimeouts)
	s.Assert().Equal(256, cfg.BufferSize)
	s.Assert().Equal(15*time.Minute, cfg.SummaryInterval)
	s.Assert().True(cfg.SubscribeToTimeNode)
}

func (s *ConfigTestSuite) TestBooleanParsing() {
	tests := []struct {
		name        string
		envValue    string
		expected    bool
	}{
		{"true string", "true", true},
		{"false string", "false", false},
		{"1 string", "1", true},
		{"0 string", "0", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.SetupTest()
			if tt.envValue != "" {
				s.Require().NoError(os.Setenv("OPCUA_EXPORTER_DEBUG", tt.envValue))
			}

			cfg, err := Load("")
			s.Require().NoError(err)
			s.Assert().Equal(tt.expected, cfg.Debug)
		})
	}
}

func (s *ConfigTestSuite) TestInvalidNodeMappingValidation() {
	yamlContent := `
port: 9090
nodes:
  - nodeName: ""
    metricName: "test_metric"
`

	require.NoError(s.T(), os.WriteFile(s.configFile, []byte(yamlContent), 0644))

	_, err := Load(s.configFile)
	s.Assert().Error(err)
	s.Assert().Contains(err.Error(), "invalid node mapping")
}

func (s *ConfigTestSuite) TestNodeMappingOverride() {
	cfg, err := Load("")
	s.Require().NoError(err)
	s.Assert().Empty(cfg.NodeMappings)

	nodeMapping1 := NodeMapping{
		NodeName:   "ns=1;s=Test",
		MetricName: "test_metric",
	}
	cfg.AddNodeMapping(nodeMapping1)
	s.Assert().Len(cfg.NodeMappings, 1)

	nodeMapping2 := NodeMapping{
		NodeName:   "ns=1;s=Override",
		MetricName: "test_metric",
	}
	cfg.AddNodeMapping(nodeMapping2)
	s.Assert().Len(cfg.NodeMappings, 1)
	s.Assert().Equal("ns=1;s=Override", cfg.NodeMappings[0].NodeName)
}

func (s *ConfigTestSuite) TestConfigurationEdgeCases() {
	s.Run("zero and negative values", func() {
		yamlContent := `
port: 0
maxTimeouts: -1
bufferSize: 0
`

		require.NoError(s.T(), os.WriteFile(s.configFile, []byte(yamlContent), 0644))

		cfg, err := Load(s.configFile)
		s.Require().NoError(err)
		s.Assert().Equal(9686, cfg.Port)
		s.Assert().Equal(-1, cfg.MaxTimeouts)
		s.Assert().Equal(64, cfg.BufferSize)
	})

	s.Run("sparse node mappings from env", func() {
		s.Require().NoError(os.Setenv("OPCUA_EXPORTER_NODES_0_NODENAME", "ns=1;s=Node0"))
		s.Require().NoError(os.Setenv("OPCUA_EXPORTER_NODES_0_METRICNAME", "metric0"))
		s.Require().NoError(os.Setenv("OPCUA_EXPORTER_NODES_5_NODENAME", "ns=1;s=Node5"))
		s.Require().NoError(os.Setenv("OPCUA_EXPORTER_NODES_5_METRICNAME", "metric5"))

		cfg, err := Load("")
		s.Require().NoError(err)
		s.Assert().Len(cfg.NodeMappings, 1)
		s.Assert().Equal("ns=1;s=Node0", cfg.NodeMappings[0].NodeName)
	})
}