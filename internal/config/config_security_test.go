package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type SecurityConfigTestSuite struct {
	suite.Suite
	tempDir    string
	configFile string
}

func (s *SecurityConfigTestSuite) SetupTest() {
	s.tempDir = s.T().TempDir()
	s.configFile = filepath.Join(s.tempDir, "test_config.yaml")
	s.clearSecurityEnvVars()
}

func (s *SecurityConfigTestSuite) TearDownTest() {
	s.clearSecurityEnvVars()
	if s.configFile != "" {
		os.Remove(s.configFile)
	}
}

func (s *SecurityConfigTestSuite) clearSecurityEnvVars() {
	envVars := []string{
		"OPCUA_EXPORTER_SECURITY_MODE",
		"OPCUA_EXPORTER_SECURITY_POLICY",
		"OPCUA_EXPORTER_AUTH_MODE",
		"OPCUA_EXPORTER_USERNAME",
		"OPCUA_EXPORTER_PASSWORD",
		"OPCUA_EXPORTER_CERTIFICATE_FILE",
		"OPCUA_EXPORTER_PRIVATE_KEY_FILE",
		"OPCUA_EXPORTER_AUTO_TRUST",
	}

	for _, envVar := range envVars {
		os.Unsetenv(envVar)
	}
}

func (s *SecurityConfigTestSuite) TestSecurityConfigDefaults() {
	cfg, err := Load("")
	s.Require().NoError(err)

	s.Assert().Equal("None", cfg.Security.SecurityMode)
	s.Assert().Equal("None", cfg.Security.SecurityPolicy)
	s.Assert().Equal("Anonymous", cfg.Security.AuthMode)
	s.Assert().Empty(cfg.Security.Username)
	s.Assert().Empty(cfg.Security.Password)
	s.Assert().Empty(cfg.Security.CertificateFile)
	s.Assert().Empty(cfg.Security.PrivateKeyFile)
	s.Assert().False(cfg.Security.AutoTrust)
}

func (s *SecurityConfigTestSuite) TestSecurityConfigFromYAML() {
	yamlContent := `
port: 9090
endpoint: "opc.tcp://test-server:4840"
security:
  securityMode: SignAndEncrypt
  securityPolicy: Basic256Sha256
  authMode: Username
  username: testuser
  password: testpass
  certificateFile: /path/to/cert.pem
  privateKeyFile: /path/to/key.pem
  autoTrust: true
nodes:
  - nodeName: "ns=1;s=Test"
    metricName: "test_metric"
`

	require.NoError(s.T(), os.WriteFile(s.configFile, []byte(yamlContent), 0644))

	cfg, err := Load(s.configFile)
	s.Require().NoError(err)

	s.Assert().Equal(9090, cfg.Port)
	s.Assert().Equal("opc.tcp://test-server:4840", cfg.Endpoint)

	s.Assert().Equal("SignAndEncrypt", cfg.Security.SecurityMode)
	s.Assert().Equal("Basic256Sha256", cfg.Security.SecurityPolicy)
	s.Assert().Equal("Username", cfg.Security.AuthMode)
	s.Assert().Equal("testuser", cfg.Security.Username)
	s.Assert().Equal("testpass", cfg.Security.Password)
	s.Assert().Equal("/path/to/cert.pem", cfg.Security.CertificateFile)
	s.Assert().Equal("/path/to/key.pem", cfg.Security.PrivateKeyFile)
	s.Assert().True(cfg.Security.AutoTrust)

	s.Require().Len(cfg.NodeMappings, 1)
	s.Assert().Equal("ns=1;s=Test", cfg.NodeMappings[0].NodeName)
	s.Assert().Equal("test_metric", cfg.NodeMappings[0].MetricName)
}

func (s *SecurityConfigTestSuite) TestSecurityConfigFromEnvironmentVariables() {
	envVars := map[string]string{
		"OPCUA_EXPORTER_SECURITY_MODE":      "Sign",
		"OPCUA_EXPORTER_SECURITY_POLICY":   "Basic256",
		"OPCUA_EXPORTER_AUTH_MODE":         "Certificate",
		"OPCUA_EXPORTER_USERNAME":          "envuser",
		"OPCUA_EXPORTER_PASSWORD":          "envpass",
		"OPCUA_EXPORTER_CERTIFICATE_FILE":  "/env/path/cert.pem",
		"OPCUA_EXPORTER_PRIVATE_KEY_FILE":  "/env/path/key.pem",
		"OPCUA_EXPORTER_AUTO_TRUST":        "true",
	}

	for key, value := range envVars {
		s.Require().NoError(os.Setenv(key, value))
	}

	cfg, err := Load("")
	s.Require().NoError(err)

	s.Assert().Equal("Sign", cfg.Security.SecurityMode)
	s.Assert().Equal("Basic256", cfg.Security.SecurityPolicy)
	s.Assert().Equal("Certificate", cfg.Security.AuthMode)
	s.Assert().Equal("envuser", cfg.Security.Username)
	s.Assert().Equal("envpass", cfg.Security.Password)
	s.Assert().Equal("/env/path/cert.pem", cfg.Security.CertificateFile)
	s.Assert().Equal("/env/path/key.pem", cfg.Security.PrivateKeyFile)
	s.Assert().True(cfg.Security.AutoTrust)
}

func (s *SecurityConfigTestSuite) TestSecurityConfigPrecedence() {
	yamlContent := `
security:
  securityMode: None
  securityPolicy: None
  authMode: Anonymous
  username: yamluser
  password: yamlpass
`

	require.NoError(s.T(), os.WriteFile(s.configFile, []byte(yamlContent), 0644))

	envVars := map[string]string{
		"OPCUA_EXPORTER_SECURITY_MODE":   "SignAndEncrypt",
		"OPCUA_EXPORTER_SECURITY_POLICY": "Basic256Sha256",
		"OPCUA_EXPORTER_AUTH_MODE":       "Username",
		"OPCUA_EXPORTER_USERNAME":        "envuser",
	}

	for key, value := range envVars {
		s.Require().NoError(os.Setenv(key, value))
	}

	cfg, err := Load(s.configFile)
	s.Require().NoError(err)

	s.Assert().Equal("SignAndEncrypt", cfg.Security.SecurityMode)
	s.Assert().Equal("Basic256Sha256", cfg.Security.SecurityPolicy)
	s.Assert().Equal("Username", cfg.Security.AuthMode)
	s.Assert().Equal("envuser", cfg.Security.Username)
	s.Assert().Equal("yamlpass", cfg.Security.Password)
}

func (s *SecurityConfigTestSuite) TestSecurityConfigMixedSources() {
	yamlContent := `
port: 8080
security:
  securityMode: Sign
  authMode: Username
  username: yamluser
nodes:
  - nodeName: "ns=1;s=YamlNode"
    metricName: "yaml_metric"
`

	require.NoError(s.T(), os.WriteFile(s.configFile, []byte(yamlContent), 0644))

	s.Require().NoError(os.Setenv("OPCUA_EXPORTER_SECURITY_POLICY", "Basic256Sha256"))
	s.Require().NoError(os.Setenv("OPCUA_EXPORTER_PASSWORD", "envpass"))
	s.Require().NoError(os.Setenv("OPCUA_EXPORTER_NODES_0_NODENAME", "ns=1;s=EnvNode"))
	s.Require().NoError(os.Setenv("OPCUA_EXPORTER_NODES_0_METRICNAME", "env_metric"))

	cfg, err := Load(s.configFile)
	s.Require().NoError(err)

	s.Assert().Equal(8080, cfg.Port)
	s.Assert().Equal("Sign", cfg.Security.SecurityMode)
	s.Assert().Equal("Basic256Sha256", cfg.Security.SecurityPolicy)
	s.Assert().Equal("Username", cfg.Security.AuthMode)
	s.Assert().Equal("yamluser", cfg.Security.Username)
	s.Assert().Equal("envpass", cfg.Security.Password)

	s.Require().Len(cfg.NodeMappings, 1)
	s.Assert().Equal("ns=1;s=EnvNode", cfg.NodeMappings[0].NodeName)
	s.Assert().Equal("env_metric", cfg.NodeMappings[0].MetricName)
}

func (s *SecurityConfigTestSuite) TestSecurityConfigEmptyValues() {
	yamlContent := `
security:
  securityMode: ""
  securityPolicy: ""
  authMode: ""
  username: ""
  password: ""
  certificateFile: ""
  privateKeyFile: ""
`

	require.NoError(s.T(), os.WriteFile(s.configFile, []byte(yamlContent), 0644))

	cfg, err := Load(s.configFile)
	s.Require().NoError(err)

	s.Assert().Equal("None", cfg.Security.SecurityMode)
	s.Assert().Equal("None", cfg.Security.SecurityPolicy)
	s.Assert().Equal("Anonymous", cfg.Security.AuthMode)
	s.Assert().Empty(cfg.Security.Username)
	s.Assert().Empty(cfg.Security.Password)
	s.Assert().Empty(cfg.Security.CertificateFile)
	s.Assert().Empty(cfg.Security.PrivateKeyFile)
}

func (s *SecurityConfigTestSuite) TestSecurityConfigBooleanValues() {
	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"true", "true", true},
		{"false", "false", false},
		{"1", "1", true},
		{"0", "0", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.SetupTest()
			if tt.envValue != "" {
				s.Require().NoError(os.Setenv("OPCUA_EXPORTER_AUTO_TRUST", tt.envValue))
			}

			cfg, err := Load("")
			s.Require().NoError(err)
			s.Assert().Equal(tt.expected, cfg.Security.AutoTrust)
		})
	}
}

func (s *SecurityConfigTestSuite) TestNodeMappingValidation() {
	tests := []struct {
		name    string
		mapping NodeMapping
		isValid bool
		errMsg  string
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
			name: "valid mapping with int extract bit",
			mapping: NodeMapping{
				NodeName:   "ns=1;s=AlarmBits",
				MetricName: "alarm_bit_0",
				ExtractBit: 0,
			},
			isValid: true,
		},
		{
			name: "valid mapping with float64 extract bit",
			mapping: NodeMapping{
				NodeName:   "ns=1;s=AlarmBits",
				MetricName: "alarm_bit_3",
				ExtractBit: 3.0,
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
			errMsg:  "nodeName cannot be empty",
		},
		{
			name: "empty metric name",
			mapping: NodeMapping{
				NodeName:   "ns=1;s=test",
				MetricName: "",
			},
			isValid: false,
			errMsg:  "metricName cannot be empty",
		},
		{
			name: "negative int extract bit",
			mapping: NodeMapping{
				NodeName:   "ns=1;s=test",
				MetricName: "test_metric",
				ExtractBit: -1,
			},
			isValid: false,
			errMsg:  "extractBit must be non-negative",
		},
		{
			name: "negative float64 extract bit",
			mapping: NodeMapping{
				NodeName:   "ns=1;s=test",
				MetricName: "test_metric",
				ExtractBit: -5.0,
			},
			isValid: false,
			errMsg:  "extractBit must be a non-negative integer",
		},
		{
			name: "non-integer float64 extract bit",
			mapping: NodeMapping{
				NodeName:   "ns=1;s=test",
				MetricName: "test_metric",
				ExtractBit: 5.5,
			},
			isValid: false,
			errMsg:  "extractBit must be a non-negative integer",
		},
		{
			name: "invalid extract bit type",
			mapping: NodeMapping{
				NodeName:   "ns=1;s=test",
				MetricName: "test_metric",
				ExtractBit: "invalid",
			},
			isValid: false,
			errMsg:  "extractBit must be a number",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := tt.mapping.Validate()
			if tt.isValid {
				s.Assert().NoError(err)
			} else {
				s.Assert().Error(err)
				if tt.errMsg != "" {
					s.Assert().Contains(err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func (s *SecurityConfigTestSuite) TestFilterValidNodeMappingsWithValidation() {
	mappings := []NodeMapping{
		{NodeName: "valid1", MetricName: "metric1"},
		{NodeName: "", MetricName: "metric2"},
		{NodeName: "valid3", MetricName: ""},
		{NodeName: "valid4", MetricName: "metric4"},
		{NodeName: "valid5", MetricName: "metric5", ExtractBit: -1},
	}

	valid := filterValidNodeMappings(mappings)
	s.Assert().Len(valid, 3)

	s.Assert().Equal("valid1", valid[0].NodeName)
	s.Assert().Equal("valid4", valid[1].NodeName)
	s.Assert().Equal("valid5", valid[2].NodeName)

	validCount := 0
	for _, mapping := range valid {
		if mapping.Validate() == nil {
			validCount++
		}
	}
	s.Assert().Equal(2, validCount)
}

func TestConfigTestSuite(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}

func TestSecurityConfigTestSuite(t *testing.T) {
	suite.Run(t, new(SecurityConfigTestSuite))
}