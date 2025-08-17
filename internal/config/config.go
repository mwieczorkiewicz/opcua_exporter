package config

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/viper"
)

// NodeMapping : Structure for representing mapping between OPCUA nodes and Prometheus metrics.
type NodeMapping struct {
	NodeName   string `yaml:"nodeName"`             // OPC UA node identifier
	MetricName string `yaml:"metricName"`           // Prometheus metric name to emit
	ExtractBit any    `yaml:"extractBit,omitempty"` // Optional numeric value. If present and positive, extract just this bit and emit it as a boolean metric
}

// SecurityConfig holds security-related configuration for OPC UA connections
type SecurityConfig struct {
	// SecurityMode defines the security mode: None, Sign, SignAndEncrypt
	SecurityMode string `yaml:"securityMode" mapstructure:"security-mode"`

	// SecurityPolicy defines the security policy: None, Basic128Rsa15, Basic256, Basic256Sha256
	SecurityPolicy string `yaml:"securityPolicy" mapstructure:"security-policy"`

	// AuthMode defines authentication mode: Anonymous, Username, Certificate
	AuthMode string `yaml:"authMode" mapstructure:"auth-mode"`

	// Username for username/password authentication
	Username string `yaml:"username,omitempty" mapstructure:"username"`

	// Password for username/password authentication
	Password string `yaml:"password,omitempty" mapstructure:"password"`

	// CertificateFile path to the client certificate file (PEM format)
	CertificateFile string `yaml:"certificateFile,omitempty" mapstructure:"certificate-file"`

	// PrivateKeyFile path to the private key file (PEM format)
	PrivateKeyFile string `yaml:"privateKeyFile,omitempty" mapstructure:"private-key-file"`

	// AutoTrust automatically trusts server certificates (insecure - for testing only)
	AutoTrust bool `yaml:"autoTrust,omitempty" mapstructure:"auto-trust"`
}

// Config holds all configuration values for the OPC UA exporter
type Config struct {
	Port                int            `mapstructure:"port"`
	Endpoint            string         `mapstructure:"endpoint"`
	PromPrefix          string         `mapstructure:"prom-prefix"`
	ConfigFile          string         `mapstructure:"config"`
	Debug               bool           `mapstructure:"debug"`
	ReadTimeout         time.Duration  `mapstructure:"read-timeout"`
	MaxTimeouts         int            `mapstructure:"max-timeouts"`
	BufferSize          int            `mapstructure:"buffer-size"`
	SummaryInterval     time.Duration  `mapstructure:"summary-interval"`
	SubscribeToTimeNode bool           `mapstructure:"subscribe-to-time-node"`
	NodeMappings        []NodeMapping  `mapstructure:"nodes"`
	Security            SecurityConfig `mapstructure:"security"`
}

// Load loads configuration from multiple sources in priority order:
// 1. Command-line flags (highest priority)
// 2. Environment variables
// 3. YAML config file (if specified)
// 4. Defaults (lowest priority)
func Load(configFile string) (*Config, error) {
	v := viper.New()

	// Set default values
	v.SetDefault("port", 9686)
	v.SetDefault("endpoint", "opc.tcp://localhost:4096")
	v.SetDefault("prom-prefix", "")
	v.SetDefault("debug", false)
	v.SetDefault("read-timeout", 5*time.Second)
	v.SetDefault("max-timeouts", 0)
	v.SetDefault("buffer-size", 64)
	v.SetDefault("summary-interval", 5*time.Minute)
	v.SetDefault("subscribe-to-time-node", false)
	v.SetDefault("nodes", []NodeMapping{})

	// Set security defaults (Anonymous access, no encryption)
	v.SetDefault("security.security-mode", "None")
	v.SetDefault("security.security-policy", "None")
	v.SetDefault("security.auth-mode", "Anonymous")
	v.SetDefault("security.auto-trust", false)

	// Enable environment variable support
	v.SetEnvPrefix("OPCUA_EXPORTER")
	v.AutomaticEnv()

	// Bind environment variables to config keys (much cleaner with SetEnvPrefix)
	v.BindEnv("port")
	v.BindEnv("endpoint")
	v.BindEnv("prom-prefix")
	v.BindEnv("debug")
	v.BindEnv("read-timeout")
	v.BindEnv("max-timeouts")
	v.BindEnv("buffer-size")
	v.BindEnv("summary-interval")
	v.BindEnv("subscribe-to-time-node")

	// Bind security environment variables (explicit binding for non-nested names)
	v.BindEnv("security.security-mode", "OPCUA_EXPORTER_SECURITY_MODE")
	v.BindEnv("security.security-policy", "OPCUA_EXPORTER_SECURITY_POLICY")
	v.BindEnv("security.auth-mode", "OPCUA_EXPORTER_AUTH_MODE")
	v.BindEnv("security.username", "OPCUA_EXPORTER_USERNAME")
	v.BindEnv("security.password", "OPCUA_EXPORTER_PASSWORD")
	v.BindEnv("security.certificate-file", "OPCUA_EXPORTER_CERTIFICATE_FILE")
	v.BindEnv("security.private-key-file", "OPCUA_EXPORTER_PRIVATE_KEY_FILE")
	v.BindEnv("security.auto-trust", "OPCUA_EXPORTER_AUTO_TRUST")

	// Support indexed environment variables for node mappings
	// Bind up to a reasonable limit, but parseEnvNodeMappings will stop early
	for i := range 100 {
		v.BindEnv(fmt.Sprintf("nodes.%d.nodeName", i), fmt.Sprintf("OPCUA_EXPORTER_NODES_%d_NODENAME", i))
		v.BindEnv(fmt.Sprintf("nodes.%d.metricName", i), fmt.Sprintf("OPCUA_EXPORTER_NODES_%d_METRICNAME", i))
		v.BindEnv(fmt.Sprintf("nodes.%d.extractBit", i), fmt.Sprintf("OPCUA_EXPORTER_NODES_%d_EXTRACTBIT", i))
	}

	// Load config file if specified
	if configFile != "" {
		v.SetConfigFile(configFile)
		if err := v.ReadInConfig(); err != nil {
			// Check if it's a file not found error - if so, warn but continue
			if os.IsNotExist(err) {
				log.Printf("Warning: config file %s not found, using defaults and environment variables", configFile)
			} else {
				return nil, fmt.Errorf("error reading config file %s: %w", configFile, err)
			}
		} else {
			log.Printf("Loaded configuration from %s", configFile)
		}
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshalling config: %w", err)
	}
	
	// Handle YAML camelCase to kebab-case mapping for security fields
	// Viper converts all keys to lowercase, so check lowercase variants
	// Only override if the kebab-case key has default value (meaning env var wasn't set)
	if v.IsSet("security.securitymode") && v.GetString("security.security-mode") == "None" {
		config.Security.SecurityMode = v.GetString("security.securitymode")
	}
	if v.IsSet("security.securitypolicy") && v.GetString("security.security-policy") == "None" {
		config.Security.SecurityPolicy = v.GetString("security.securitypolicy")
	}
	if v.IsSet("security.authmode") && v.GetString("security.auth-mode") == "Anonymous" {
		config.Security.AuthMode = v.GetString("security.authmode")
	}
	if v.IsSet("security.username") {
		config.Security.Username = v.GetString("security.username")
	}
	if v.IsSet("security.password") {
		config.Security.Password = v.GetString("security.password")
	}
	if v.IsSet("security.certificatefile") && v.GetString("security.certificate-file") == "" {
		config.Security.CertificateFile = v.GetString("security.certificatefile")
	}
	if v.IsSet("security.privatekeyfile") && v.GetString("security.private-key-file") == "" {
		config.Security.PrivateKeyFile = v.GetString("security.privatekeyfile")
	}
	if v.IsSet("security.autotrust") && v.GetBool("security.auto-trust") == false {
		config.Security.AutoTrust = v.GetBool("security.autotrust")
	}

	// Parse environment variables for node mappings
	envNodeMappings := parseEnvNodeMappings(v)

	// Combine mappings with proper precedence: env vars override YAML
	config.NodeMappings = mergeNodeMappings(config.NodeMappings, envNodeMappings)

	if len(envNodeMappings) > 0 {
		log.Printf("Loaded %d node mappings from environment variables", len(envNodeMappings))
	}

	// Filter out empty node mappings
	config.NodeMappings = filterValidNodeMappings(config.NodeMappings)

	// Ensure security defaults are set if the section is missing or has empty values
	if config.Security.SecurityMode == "" {
		config.Security.SecurityMode = "None"
	}
	if config.Security.SecurityPolicy == "" {
		config.Security.SecurityPolicy = "None"
	}
	if config.Security.AuthMode == "" {
		config.Security.AuthMode = "Anonymous"
	}

	return &config, nil
}

// parseEnvNodeMappings extracts node mappings from environment variables
// Stops parsing when no more sequential mappings are found
func parseEnvNodeMappings(v *viper.Viper) []NodeMapping {
	var envNodeMappings []NodeMapping
	for i := 0; ; i++ {
		nodeNameKey := fmt.Sprintf("nodes.%d.nodeName", i)
		metricNameKey := fmt.Sprintf("nodes.%d.metricName", i)
		extractBitKey := fmt.Sprintf("nodes.%d.extractBit", i)

		nodeName := v.GetString(nodeNameKey)
		metricName := v.GetString(metricNameKey)

		// Stop parsing when we encounter the first missing sequential mapping
		if nodeName == "" || metricName == "" {
			break
		}

		nodeMapping := NodeMapping{
			NodeName:   nodeName,
			MetricName: metricName,
		}

		if v.IsSet(extractBitKey) {
			extractBit := v.GetInt(extractBitKey)
			nodeMapping.ExtractBit = extractBit
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

// mergeNodeMappings combines two slices of node mappings, with higher priority mappings overriding lower priority ones
// higherPriority mappings override lowerPriority mappings when metric names match
func mergeNodeMappings(lowerPriority, higherPriority []NodeMapping) []NodeMapping {
	// Create a map to track metric names from higher priority source
	higherPriorityMetrics := make(map[string]NodeMapping)
	for _, mapping := range higherPriority {
		if mapping.MetricName != "" {
			higherPriorityMetrics[mapping.MetricName] = mapping
		}
	}

	var result []NodeMapping

	// Add lower priority mappings, skipping those overridden by higher priority
	for _, mapping := range lowerPriority {
		if _, overridden := higherPriorityMetrics[mapping.MetricName]; !overridden {
			result = append(result, mapping)
		} else {
			log.Printf("Metric mapping '%s' from config file overridden by environment variable", mapping.MetricName)
		}
	}

	// Add all higher priority mappings
	for _, mapping := range higherPriority {
		if mapping.MetricName != "" {
			result = append(result, mapping)
		}
	}

	return result
}

// AddNodeMapping adds a node mapping to the configuration with highest priority
// Command-line flags override both YAML and environment variables
func (c *Config) AddNodeMapping(nodeMapping NodeMapping) {
	// Command-line flags have highest priority, so they override existing mappings
	c.NodeMappings = mergeNodeMappings(c.NodeMappings, []NodeMapping{nodeMapping})
}
