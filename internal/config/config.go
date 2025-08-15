package config

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
)

// NodeMapping : Structure for representing mapping between OPCUA nodes and Prometheus metrics.
type NodeMapping struct {
	NodeName   string `yaml:"nodeName"`             // OPC UA node identifier
	MetricName string `yaml:"metricName"`           // Prometheus metric name to emit
	ExtractBit any    `yaml:"extractBit,omitempty"` // Optional numeric value. If present and positive, extract just this bit and emit it as a boolean metric
}

// ReadFile reads the configuration from a YAML file at the specified path.
// It returns a slice of NodeConfig or an error if the file cannot be read or parsed
func ReadFile(path string) ([]NodeMapping, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}

	return parse(f)
}

// ReadBase64 decodes a base64-encoded configuration string and parses it into NodeConfig.
// It logs an error if the decoding fails.
func ReadBase64(encodedConfig *string) ([]NodeMapping, error) {
	config, decodeErr := base64.StdEncoding.DecodeString(*encodedConfig)
	if decodeErr != nil {
		return nil, fmt.Errorf("failed to decode base64 config: %w", decodeErr)
	}
	return parse(bytes.NewReader(config))
}

// parse reads the YAML configuration from the provided reader and returns a slice of NodeConfig.
// It logs the number of nodes found in the configuration.
func parse(r io.Reader) ([]NodeMapping, error) {
	content, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var nodes []NodeMapping
	err = yaml.Unmarshal(content, &nodes)
	log.Printf("Found %d nodes in config file.", len(nodes))
	return nodes, err
}

// Config holds all configuration values for the OPC UA exporter
type Config struct {
	Port                int           `mapstructure:"port"`
	Endpoint            string        `mapstructure:"endpoint"`
	PromPrefix          string        `mapstructure:"prom-prefix"`
	ConfigFile          string        `mapstructure:"config"`
	ConfigB64           string        `mapstructure:"config-b64"`
	Debug               bool          `mapstructure:"debug"`
	ReadTimeout         time.Duration `mapstructure:"read-timeout"`
	MaxTimeouts         int           `mapstructure:"max-timeouts"`
	BufferSize          int           `mapstructure:"buffer-size"`
	SummaryInterval     time.Duration `mapstructure:"summary-interval"`
	SubscribeToTimeNode bool          `mapstructure:"subscribe-to-time-node"`
	NodeMappings        []NodeMapping `mapstructure:"nodes"`
}

// InitViper initializes viper with default values, environment variable bindings, and flag parsing
func InitViper() *Config {
	v := viper.New()

	// Set default values
	v.SetDefault("port", 9686)
	v.SetDefault("endpoint", "opc.tcp://localhost:4096")
	v.SetDefault("prom-prefix", "")
	v.SetDefault("config", "")
	v.SetDefault("config-b64", "")
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
	v.BindEnv("config", "OPCUA_EXPORTER_CONFIG")
	v.BindEnv("config-b64", "OPCUA_EXPORTER_CONFIG_B64")
	v.BindEnv("debug", "OPCUA_EXPORTER_DEBUG")
	v.BindEnv("read-timeout", "OPCUA_EXPORTER_READ_TIMEOUT")
	v.BindEnv("max-timeouts", "OPCUA_EXPORTER_MAX_TIMEOUTS")
	v.BindEnv("buffer-size", "OPCUA_EXPORTER_BUFFER_SIZE")
	v.BindEnv("summary-interval", "OPCUA_EXPORTER_SUMMARY_INTERVAL")
	v.BindEnv("subscribe-to-time-node", "OPCUA_EXPORTER_SUBSCRIBE_TO_TIME_NODE")
	
	// Support indexed environment variables for node mappings
	// e.g., OPCUA_EXPORTER_NODES_0_NODENAME, OPCUA_EXPORTER_NODES_0_METRICNAME, etc.
	for i := 0; i < 100; i++ { // Support up to 100 nodes via environment variables
		v.BindEnv(fmt.Sprintf("nodes.%d.nodeName", i), fmt.Sprintf("OPCUA_EXPORTER_NODES_%d_NODENAME", i))
		v.BindEnv(fmt.Sprintf("nodes.%d.metricName", i), fmt.Sprintf("OPCUA_EXPORTER_NODES_%d_METRICNAME", i))
		v.BindEnv(fmt.Sprintf("nodes.%d.extractBit", i), fmt.Sprintf("OPCUA_EXPORTER_NODES_%d_EXTRACTBIT", i))
	}

	// Set config name and paths for YAML config file
	v.SetConfigName("opcua_exporter")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("$HOME/.opcua_exporter")
	v.AddConfigPath("/etc/opcua_exporter")

	// Try to read config file (ignore if not found)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			log.Printf("Error reading config file: %v", err)
		}
	}
	

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		log.Printf("Error unmarshalling config: %v", err)
	}

	// Manually parse environment variables for node mappings since viper's unmarshal doesn't handle them well
	var envNodeMappings []NodeMapping
	for i := 0; i < 100; i++ {
		nodeNameKey := fmt.Sprintf("nodes.%d.nodeName", i)
		metricNameKey := fmt.Sprintf("nodes.%d.metricName", i)
		extractBitKey := fmt.Sprintf("nodes.%d.extractBit", i)
		
		nodeName := v.GetString(nodeNameKey)
		metricName := v.GetString(metricNameKey)
		
		if nodeName != "" && metricName != "" {
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
	}
	
	// If we found environment variables, use them instead of config file mappings
	if len(envNodeMappings) > 0 {
		config.NodeMappings = envNodeMappings
	} else {
		// Filter out empty node mappings from config file
		var validNodeMappings []NodeMapping
		for _, node := range config.NodeMappings {
			if node.NodeName != "" && node.MetricName != "" {
				validNodeMappings = append(validNodeMappings, node)
			}
		}
		config.NodeMappings = validNodeMappings
	}

	return &config
}
