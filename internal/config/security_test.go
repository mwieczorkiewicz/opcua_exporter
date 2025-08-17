package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecurityConfigDefaults(t *testing.T) {
	cfg, err := Load("")
	require.NoError(t, err)

	// Test that security defaults are properly set
	assert.Equal(t, "None", cfg.Security.SecurityMode)
	assert.Equal(t, "None", cfg.Security.SecurityPolicy)
	assert.Equal(t, "Anonymous", cfg.Security.AuthMode)
	assert.Empty(t, cfg.Security.Username)
	assert.Empty(t, cfg.Security.Password)
	assert.Empty(t, cfg.Security.CertificateFile)
	assert.Empty(t, cfg.Security.PrivateKeyFile)
	assert.False(t, cfg.Security.AutoTrust)
}

func TestSecurityConfigFromYAML(t *testing.T) {
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

	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(yamlContent), 0644))

	cfg, err := Load(configFile)
	require.NoError(t, err)

	// Test basic config
	assert.Equal(t, 9090, cfg.Port)
	assert.Equal(t, "opc.tcp://test-server:4840", cfg.Endpoint)

	// Test security config
	assert.Equal(t, "SignAndEncrypt", cfg.Security.SecurityMode)
	assert.Equal(t, "Basic256Sha256", cfg.Security.SecurityPolicy)
	assert.Equal(t, "Username", cfg.Security.AuthMode)
	assert.Equal(t, "testuser", cfg.Security.Username)
	assert.Equal(t, "testpass", cfg.Security.Password)
	assert.Equal(t, "/path/to/cert.pem", cfg.Security.CertificateFile)
	assert.Equal(t, "/path/to/key.pem", cfg.Security.PrivateKeyFile)
	assert.True(t, cfg.Security.AutoTrust)

	// Test node config
	require.Len(t, cfg.NodeMappings, 1)
	assert.Equal(t, "ns=1;s=Test", cfg.NodeMappings[0].NodeName)
	assert.Equal(t, "test_metric", cfg.NodeMappings[0].MetricName)
}

func TestSecurityConfigFromEnvironmentVariables(t *testing.T) {
	// Set up environment variables
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

	// Set environment variables
	for key, value := range envVars {
		require.NoError(t, os.Setenv(key, value))
	}

	// Clean up environment variables after test
	defer func() {
		for key := range envVars {
			os.Unsetenv(key)
		}
	}()

	cfg, err := Load("")
	require.NoError(t, err)

	// Test security config from environment
	assert.Equal(t, "Sign", cfg.Security.SecurityMode)
	assert.Equal(t, "Basic256", cfg.Security.SecurityPolicy)
	assert.Equal(t, "Certificate", cfg.Security.AuthMode)
	assert.Equal(t, "envuser", cfg.Security.Username)
	assert.Equal(t, "envpass", cfg.Security.Password)
	assert.Equal(t, "/env/path/cert.pem", cfg.Security.CertificateFile)
	assert.Equal(t, "/env/path/key.pem", cfg.Security.PrivateKeyFile)
	assert.True(t, cfg.Security.AutoTrust)
}

func TestSecurityConfigPrecedence(t *testing.T) {
	// Create YAML config with some security settings
	yamlContent := `
security:
  securityMode: None
  securityPolicy: None
  authMode: Anonymous
  username: yamluser
  password: yamlpass
`

	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(yamlContent), 0644))

	// Set environment variables that should override YAML
	envVars := map[string]string{
		"OPCUA_EXPORTER_SECURITY_MODE":   "SignAndEncrypt",
		"OPCUA_EXPORTER_SECURITY_POLICY": "Basic256Sha256",
		"OPCUA_EXPORTER_AUTH_MODE":       "Username",
		"OPCUA_EXPORTER_USERNAME":        "envuser",
	}

	for key, value := range envVars {
		require.NoError(t, os.Setenv(key, value))
	}

	defer func() {
		for key := range envVars {
			os.Unsetenv(key)
		}
	}()

	cfg, err := Load(configFile)
	require.NoError(t, err)

	// Environment variables should override YAML
	assert.Equal(t, "SignAndEncrypt", cfg.Security.SecurityMode)
	assert.Equal(t, "Basic256Sha256", cfg.Security.SecurityPolicy)
	assert.Equal(t, "Username", cfg.Security.AuthMode)
	assert.Equal(t, "envuser", cfg.Security.Username)
	
	// Values not set in env should come from YAML
	assert.Equal(t, "yamlpass", cfg.Security.Password)
}

func TestSecurityConfigMixedSources(t *testing.T) {
	// Test that security config works alongside other configuration sources
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

	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(yamlContent), 0644))

	// Set some environment variables
	require.NoError(t, os.Setenv("OPCUA_EXPORTER_SECURITY_POLICY", "Basic256Sha256"))
	require.NoError(t, os.Setenv("OPCUA_EXPORTER_PASSWORD", "envpass"))
	require.NoError(t, os.Setenv("OPCUA_EXPORTER_NODES_0_NODENAME", "ns=1;s=EnvNode"))
	require.NoError(t, os.Setenv("OPCUA_EXPORTER_NODES_0_METRICNAME", "env_metric"))

	defer func() {
		os.Unsetenv("OPCUA_EXPORTER_SECURITY_POLICY")
		os.Unsetenv("OPCUA_EXPORTER_PASSWORD")
		os.Unsetenv("OPCUA_EXPORTER_NODES_0_NODENAME")
		os.Unsetenv("OPCUA_EXPORTER_NODES_0_METRICNAME")
	}()

	cfg, err := Load(configFile)
	require.NoError(t, err)

	// Port from YAML
	assert.Equal(t, 8080, cfg.Port)

	// Security config from mixed sources
	assert.Equal(t, "Sign", cfg.Security.SecurityMode)           // From YAML
	assert.Equal(t, "Basic256Sha256", cfg.Security.SecurityPolicy) // From env
	assert.Equal(t, "Username", cfg.Security.AuthMode)           // From YAML
	assert.Equal(t, "yamluser", cfg.Security.Username)           // From YAML
	assert.Equal(t, "envpass", cfg.Security.Password)            // From env

	// Node mappings should be merged (env overrides YAML by metric name)
	require.Len(t, cfg.NodeMappings, 1)
	assert.Equal(t, "ns=1;s=EnvNode", cfg.NodeMappings[0].NodeName)   // Env overrides
	assert.Equal(t, "env_metric", cfg.NodeMappings[0].MetricName)     // Env overrides
}

func TestSecurityConfigEmptyValues(t *testing.T) {
	// Test behavior with empty security values
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

	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(yamlContent), 0644))

	cfg, err := Load(configFile)
	require.NoError(t, err)

	// Empty values should use defaults
	assert.Equal(t, "None", cfg.Security.SecurityMode)
	assert.Equal(t, "None", cfg.Security.SecurityPolicy)
	assert.Equal(t, "Anonymous", cfg.Security.AuthMode)
	assert.Empty(t, cfg.Security.Username)
	assert.Empty(t, cfg.Security.Password)
	assert.Empty(t, cfg.Security.CertificateFile)
	assert.Empty(t, cfg.Security.PrivateKeyFile)
}

func TestSecurityConfigBooleanValues(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				require.NoError(t, os.Setenv("OPCUA_EXPORTER_AUTO_TRUST", tt.envValue))
				defer os.Unsetenv("OPCUA_EXPORTER_AUTO_TRUST")
			}

			cfg, err := Load("")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, cfg.Security.AutoTrust)
		})
	}
}