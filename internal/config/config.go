package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/spf13/viper"
)

// NodeMapping : Structure for representing mapping between OPCUA nodes and Prometheus metrics.
type NodeMapping struct {
	NodeName   string            `yaml:"nodeName"`             // OPC UA node identifier
	MetricName string            `yaml:"metricName"`           // Prometheus metric name to emit
	ExtractBit any               `yaml:"extractBit,omitempty"` // Optional numeric value. If present and positive, extract just this bit and emit it as a boolean metric
	MetricHelp string            `yaml:"metricHelp,omitempty"` // Optional HELP string for metric
	Labels     map[string]string `yaml:"labels,omitempty"`     // optional lables to add to metric
	InfoLabel  string            `yaml:"infoLabel,omitempty"`  // If set metric is considered info metric, value is wtitten into given label name, value is 1
}

// Validate checks if the NodeMapping is valid
func (n *NodeMapping) Validate() error {
	if n.NodeName == "" {
		return fmt.Errorf("nodeName cannot be empty")
	}
	if n.MetricName == "" {
		return fmt.Errorf("metricName cannot be empty")
	}
	if n.ExtractBit != nil {
		switch bit := n.ExtractBit.(type) {
		case int:
			if bit < 0 {
				return fmt.Errorf("extractBit must be non-negative, got %d", bit)
			}
		case float64:
			if bit < 0 || bit != float64(int(bit)) {
				return fmt.Errorf("extractBit must be a non-negative integer, got %f", bit)
			}
		default:
			return fmt.Errorf("extractBit must be a number, got %T", n.ExtractBit)
		}
	}
	return nil
}

// SecurityConfig holds security-related configuration for OPC UA connections
type SecurityConfig struct {
	// SecurityMode defines the security mode: None, Sign, SignAndEncrypt
	SecurityMode string `yaml:"securityMode" mapstructure:"securityMode"`

	// SecurityPolicy defines the security policy: None, Basic128Rsa15, Basic256, Basic256Sha256
	SecurityPolicy string `yaml:"securityPolicy" mapstructure:"securityPolicy"`

	// AuthMode defines authentication mode: Anonymous, Username, Certificate
	AuthMode string `yaml:"authMode" mapstructure:"authMode"`

	// Username for username/password authentication
	Username string `yaml:"username,omitempty" mapstructure:"username"`

	// Password for username/password authentication
	Password string `yaml:"password,omitempty" mapstructure:"password"`

	// CertificateFile path to the client certificate file (PEM format)
	CertificateFile string `yaml:"certificateFile,omitempty" mapstructure:"certificateFile"`

	// PrivateKeyFile path to the private key file (PEM format)
	PrivateKeyFile string `yaml:"privateKeyFile,omitempty" mapstructure:"privateKeyFile"`

	// AutoTrust automatically trusts server certificates (insecure - for testing only)
	AutoTrust bool `yaml:"autoTrust,omitempty" mapstructure:"autoTrust"`

	// AlwaysDiscoverEndpoints forces the endpoint discovery flow (GetEndpoints) even
	// when security mode is None and auth mode is Anonymous. Some servers require
	// discovery regardless of the configured security settings.
	AlwaysDiscoverEndpoints bool `yaml:"alwaysDiscoverEndpoints,omitempty" mapstructure:"alwaysDiscoverEndpoints"`
}

// ConnectionTimeouts holds timeout configuration for OPC UA connections
type ConnectionTimeouts struct {
	// DialTimeout is the timeout for establishing the initial connection
	DialTimeout time.Duration `yaml:"dialTimeout" mapstructure:"dialTimeout"`

	// RequestTimeout is the timeout for individual OPC UA service requests
	RequestTimeout time.Duration `yaml:"requestTimeout" mapstructure:"requestTimeout"`

	// SessionTimeout is the requested session timeout (how long the session stays alive)
	SessionTimeout time.Duration `yaml:"sessionTimeout" mapstructure:"sessionTimeout"`

	// ConnectionRetryTimeout is the maximum total time to spend retrying connections
	ConnectionRetryTimeout time.Duration `yaml:"connectionRetryTimeout" mapstructure:"connectionRetryTimeout"`
}

// Config holds all configuration values for the OPC UA exporter
type Config struct {
	Port                int                `yaml:"port" mapstructure:"port"`
	Endpoint            string             `yaml:"endpoint" mapstructure:"endpoint"`
	PromPrefix          string             `yaml:"promPrefix" mapstructure:"promPrefix"`
	ConfigFile          string             `yaml:"configFile" mapstructure:"config"`
	Debug               bool               `yaml:"debug" mapstructure:"debug"`
	ReadTimeout         time.Duration      `yaml:"readTimeout" mapstructure:"readTimeout"`
	MaxTimeouts         int                `yaml:"maxTimeouts" mapstructure:"maxTimeouts"`
	BufferSize          int                `yaml:"bufferSize" mapstructure:"bufferSize"`
	SummaryInterval     time.Duration      `yaml:"summaryInterval" mapstructure:"summaryInterval"`
	SubscribeToTimeNode bool               `yaml:"subscribeToTimeNode" mapstructure:"subscribeToTimeNode"`
	NodeMappings        []NodeMapping      `yaml:"nodes" mapstructure:"nodes"`
	Security            SecurityConfig     `yaml:"security" mapstructure:"security"`
	Timeouts            ConnectionTimeouts `yaml:"timeouts" mapstructure:"timeouts"`
}

// Load loads configuration from multiple sources in priority order:
// 1. Command-line flags (highest priority)
// 2. Environment variables
// 3. YAML config file (if specified)
// 4. Defaults (lowest priority)
func Load(configFile string) (*Config, error) {
	v := viper.New()

	// Allow empty environment variables to override defaults
	v.AllowEmptyEnv(true)

	// Set default values
	v.SetDefault("port", 9686)
	v.SetDefault("endpoint", "opc.tcp://localhost:4096")
	v.SetDefault("promPrefix", "")
	v.SetDefault("debug", false)
	v.SetDefault("readTimeout", 5*time.Second)
	v.SetDefault("maxTimeouts", 0)
	v.SetDefault("bufferSize", 64)
	v.SetDefault("summaryInterval", 5*time.Minute)
	v.SetDefault("subscribeToTimeNode", false)
	v.SetDefault("nodes", []NodeMapping{})

	// Set security defaults (Anonymous access, no encryption)
	v.SetDefault("security.securityMode", "None")
	v.SetDefault("security.securityPolicy", "None")
	v.SetDefault("security.authMode", "Anonymous")
	v.SetDefault("security.autoTrust", false)
	v.SetDefault("security.alwaysDiscoverEndpoints", false)

	// Set timeout defaults (matching current hardcoded values)
	v.SetDefault("timeouts.dialTimeout", 10*time.Second)
	v.SetDefault("timeouts.requestTimeout", 5*time.Second)
	v.SetDefault("timeouts.sessionTimeout", 20*time.Minute)
	v.SetDefault("timeouts.connectionRetryTimeout", 5*time.Minute)

	// Configure environment variable support
	v.SetEnvPrefix("OPCUA_EXPORTER")
	v.AutomaticEnv()

	// Bind environment variables explicitly since viper's key replacer is tricky
	v.BindEnv("port", "OPCUA_EXPORTER_PORT")
	v.BindEnv("endpoint", "OPCUA_EXPORTER_ENDPOINT")
	v.BindEnv("promPrefix", "OPCUA_EXPORTER_PROM_PREFIX")
	v.BindEnv("debug", "OPCUA_EXPORTER_DEBUG")
	v.BindEnv("readTimeout", "OPCUA_EXPORTER_READ_TIMEOUT")
	v.BindEnv("maxTimeouts", "OPCUA_EXPORTER_MAX_TIMEOUTS")
	v.BindEnv("bufferSize", "OPCUA_EXPORTER_BUFFER_SIZE")
	v.BindEnv("summaryInterval", "OPCUA_EXPORTER_SUMMARY_INTERVAL")
	v.BindEnv("subscribeToTimeNode", "OPCUA_EXPORTER_SUBSCRIBE_TO_TIME_NODE")

	// Security settings
	v.BindEnv("security.securityMode", "OPCUA_EXPORTER_SECURITY_MODE")
	v.BindEnv("security.securityPolicy", "OPCUA_EXPORTER_SECURITY_POLICY")
	v.BindEnv("security.authMode", "OPCUA_EXPORTER_AUTH_MODE")
	v.BindEnv("security.username", "OPCUA_EXPORTER_USERNAME")
	v.BindEnv("security.password", "OPCUA_EXPORTER_PASSWORD")
	v.BindEnv("security.certificateFile", "OPCUA_EXPORTER_CERTIFICATE_FILE")
	v.BindEnv("security.privateKeyFile", "OPCUA_EXPORTER_PRIVATE_KEY_FILE")
	v.BindEnv("security.autoTrust", "OPCUA_EXPORTER_AUTO_TRUST")
	v.BindEnv("security.alwaysDiscoverEndpoints", "OPCUA_EXPORTER_ALWAYS_DISCOVER_ENDPOINTS")

	// Timeout settings
	v.BindEnv("timeouts.dialTimeout", "OPCUA_EXPORTER_DIAL_TIMEOUT")
	v.BindEnv("timeouts.requestTimeout", "OPCUA_EXPORTER_REQUEST_TIMEOUT")
	v.BindEnv("timeouts.sessionTimeout", "OPCUA_EXPORTER_SESSION_TIMEOUT")
	v.BindEnv("timeouts.connectionRetryTimeout", "OPCUA_EXPORTER_CONNECTION_RETRY_TIMEOUT")

	// Load configuration file if specified
	configFileLoaded := false
	if configFile != "" {
		v.SetConfigFile(configFile)
		if err := v.ReadInConfig(); err != nil {
			// Check if it's a file not found error
			if _, ok := err.(viper.ConfigFileNotFoundError); ok {
				log.Printf("Warning: config file %s not found, using defaults and environment variables", configFile)
			} else if os.IsNotExist(err) {
				// Handle regular file not found errors
				log.Printf("Warning: config file %s not found, using defaults and environment variables", configFile)
			} else {
				return nil, fmt.Errorf("error parsing YAML config file %s: %w", configFile, err)
			}
		} else {
			log.Printf("Loaded configuration from %s", configFile)
			configFileLoaded = true
		}
	}

	// Parse environment variables for node mappings (viper doesn't handle indexed arrays well)
	envNodeMappings := parseEnvNodeMappings()

	var config Config

	// Unmarshal the configuration
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling configuration: %w", err)
	}

	// Apply defaults for empty string values in security config (but not for optional fields)
	applySecurityDefaults(&config)

	// Apply defaults for zero values that should use defaults (only when loading from YAML)
	if configFileLoaded {
		applyZeroValueDefaults(&config)
	}

	// Handle node mappings with proper precedence: env vars completely replace YAML
	if len(envNodeMappings) > 0 {
		config.NodeMappings = envNodeMappings
		log.Printf("Loaded %d node mappings from environment variables", len(envNodeMappings))
	}

	// Validate all node mappings first (this will catch invalid ones before filtering)
	for i, mapping := range config.NodeMappings {
		if err := mapping.Validate(); err != nil {
			return nil, fmt.Errorf("invalid node mapping at index %d: %w", i, err)
		}
	}

	// Filter out empty node mappings (only removes truly empty ones, not invalid ones)
	config.NodeMappings = filterValidNodeMappings(config.NodeMappings)

	return &config, nil
}

// parseEnvNodeMappings extracts node mappings from environment variables
// Stops parsing when no more sequential mappings are found
func parseEnvNodeMappings() []NodeMapping {
	var envNodeMappings []NodeMapping
	for i := 0; ; i++ {
		nodeNameEnv := fmt.Sprintf("OPCUA_EXPORTER_NODES_%d_NODENAME", i)
		metricNameEnv := fmt.Sprintf("OPCUA_EXPORTER_NODES_%d_METRICNAME", i)
		extractBitEnv := fmt.Sprintf("OPCUA_EXPORTER_NODES_%d_EXTRACTBIT", i)

		nodeName := os.Getenv(nodeNameEnv)
		metricName := os.Getenv(metricNameEnv)

		// Stop parsing when we encounter the first missing sequential mapping
		if nodeName == "" || metricName == "" {
			break
		}

		nodeMapping := NodeMapping{
			NodeName:   nodeName,
			MetricName: metricName,
		}

		if extractBitStr := os.Getenv(extractBitEnv); extractBitStr != "" {
			if extractBit, err := strconv.Atoi(extractBitStr); err == nil {
				nodeMapping.ExtractBit = extractBit
			}
		}

		envNodeMappings = append(envNodeMappings, nodeMapping)
	}
	return envNodeMappings
}

// filterValidNodeMappings removes empty node mappings
func filterValidNodeMappings(mappings []NodeMapping) []NodeMapping {
	var validMappings []NodeMapping
	for _, mapping := range mappings {
		if mapping.NodeName != "" && mapping.MetricName != "" {
			validMappings = append(validMappings, mapping)
		}
	}
	return validMappings
}

// applySecurityDefaults applies default values for empty security configuration strings
// Only applies defaults to required fields, leaves optional fields empty
func applySecurityDefaults(config *Config) {
	if config.Security.SecurityMode == "" {
		config.Security.SecurityMode = "None"
	}
	if config.Security.SecurityPolicy == "" {
		config.Security.SecurityPolicy = "None"
	}
	if config.Security.AuthMode == "" {
		config.Security.AuthMode = "Anonymous"
	}
	// Optional fields (username, password, certificateFile, privateKeyFile) are left as-is
}

// applyZeroValueDefaults applies default values for zero values that should use defaults
// This handles cases where YAML explicitly sets values to 0 but we want to use defaults
func applyZeroValueDefaults(config *Config) {
	if config.Port == 0 {
		config.Port = 9686
	}
	if config.BufferSize == 0 {
		config.BufferSize = 64
	}
	// MaxTimeouts = 0 is valid and means no timeout limit, so we don't change it
}

// AddNodeMapping adds a node mapping to the configuration with highest priority
// Command-line flags override both YAML and environment variables by metric name
func (c *Config) AddNodeMapping(nodeMapping NodeMapping) {
	// Remove any existing mapping with the same metric name
	var result []NodeMapping
	for _, mapping := range c.NodeMappings {
		if mapping.MetricName != nodeMapping.MetricName {
			result = append(result, mapping)
		} else {
			log.Printf("Metric mapping '%s' overridden by command-line flag", mapping.MetricName)
		}
	}

	// Add the new mapping
	result = append(result, nodeMapping)
	c.NodeMappings = result
}
