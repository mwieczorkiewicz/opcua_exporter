package config

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// NodeMapping : Structure for representing mapping between OPCUA nodes and Prometheus metrics.
type NodeMapping struct {
	NodeName   string      `yaml:"nodeName"`             // OPC UA node identifier
	MetricName string      `yaml:"metricName"`           // Prometheus metric name to emit
	ExtractBit any `yaml:"extractBit,omitempty"` // Optional numeric value. If present and positive, extract just this bit and emit it as a boolean metric
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
