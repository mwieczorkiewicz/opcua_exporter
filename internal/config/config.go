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


// Config holds all configuration values for the OPC UA exporter
type Config struct {
	Port                int           `mapstructure:"port"`
	Endpoint            string        `mapstructure:"endpoint"`
	PromPrefix          string        `mapstructure:"prom-prefix"`
	ConfigFile          string        `mapstructure:"config"`
	Debug               bool          `mapstructure:"debug"`
	ReadTimeout         time.Duration `mapstructure:"read-timeout"`
	MaxTimeouts         int           `mapstructure:"max-timeouts"`
	BufferSize          int           `mapstructure:"buffer-size"`
	SummaryInterval     time.Duration `mapstructure:"summary-interval"`
	SubscribeToTimeNode bool          `mapstructure:"subscribe-to-time-node"`
	NodeMappings        []NodeMapping `mapstructure:"nodes"`
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

	// Enable environment variable support
	v.SetEnvPrefix("OPCUA_EXPORTER")
	v.AutomaticEnv()

	// Bind environment variables to config keys  
	v.BindEnv("port", "OPCUA_EXPORTER_PORT")
	v.BindEnv("endpoint", "OPCUA_EXPORTER_ENDPOINT")
	v.BindEnv("prom-prefix", "OPCUA_EXPORTER_PROM_PREFIX")
	v.BindEnv("debug", "OPCUA_EXPORTER_DEBUG")
	v.BindEnv("read-timeout", "OPCUA_EXPORTER_READ_TIMEOUT")
	v.BindEnv("max-timeouts", "OPCUA_EXPORTER_MAX_TIMEOUTS")
	v.BindEnv("buffer-size", "OPCUA_EXPORTER_BUFFER_SIZE")
	v.BindEnv("summary-interval", "OPCUA_EXPORTER_SUMMARY_INTERVAL")
	v.BindEnv("subscribe-to-time-node", "OPCUA_EXPORTER_SUBSCRIBE_TO_TIME_NODE")

	// Support indexed environment variables for node mappings
	// Bind up to a reasonable limit, but parseEnvNodeMappings will stop early
	for i := 0; i < 100; i++ {
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

	// Parse environment variables for node mappings
	envNodeMappings := parseEnvNodeMappings(v)
	
	// Combine mappings with proper precedence: env vars override YAML
	config.NodeMappings = mergeNodeMappings(config.NodeMappings, envNodeMappings)
	
	if len(envNodeMappings) > 0 {
		log.Printf("Loaded %d node mappings from environment variables", len(envNodeMappings))
	}

	// Filter out empty node mappings
	config.NodeMappings = filterValidNodeMappings(config.NodeMappings)

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
