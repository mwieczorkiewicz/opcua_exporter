package errors

import "fmt"

// ConfigError represents configuration-related errors with context
type ConfigError struct {
	Field string
	Value interface{}
	Err   error
}

func (e ConfigError) Error() string {
	return fmt.Sprintf("config field %s: %v: %v", e.Field, e.Value, e.Err)
}

func (e ConfigError) Unwrap() error {
	return e.Err
}

// NewConfigError creates a new configuration error
func NewConfigError(field string, value interface{}, err error) ConfigError {
	return ConfigError{
		Field: field,
		Value: value,
		Err:   err,
	}
}

// ConnectionError represents OPC UA connection-related errors
type ConnectionError struct {
	Endpoint string
	Err      error
}

func (e ConnectionError) Error() string {
	return fmt.Sprintf("connection to %s failed: %v", e.Endpoint, e.Err)
}

func (e ConnectionError) Unwrap() error {
	return e.Err
}

// NewConnectionError creates a new connection error
func NewConnectionError(endpoint string, err error) ConnectionError {
	return ConnectionError{
		Endpoint: endpoint,
		Err:      err,
	}
}

// MetricError represents Prometheus metric registration errors
type MetricError struct {
	MetricName string
	NodeName   string
	Err        error
}

func (e MetricError) Error() string {
	return fmt.Sprintf("metric %s for node %s: %v", e.MetricName, e.NodeName, e.Err)
}

func (e MetricError) Unwrap() error {
	return e.Err
}

// NewMetricError creates a new metric error
func NewMetricError(metricName, nodeName string, err error) MetricError {
	return MetricError{
		MetricName: metricName,
		NodeName:   nodeName,
		Err:        err,
	}
}