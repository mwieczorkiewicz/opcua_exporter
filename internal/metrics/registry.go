package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/mwieczorkiewicz/opcua_exporter/internal/config"
	"github.com/mwieczorkiewicz/opcua_exporter/internal/errors"
	"github.com/mwieczorkiewicz/opcua_exporter/internal/handlers"
)

// Registry manages Prometheus metrics and their associated handlers
type Registry struct {
	handlerMap map[string][]HandlerRecord
}

// HandlerRecord pairs a node configuration with its message handler
type HandlerRecord struct {
	Config  config.NodeMapping
	Handler handlers.MsgHandler
}

// NewRegistry creates a new metrics registry
func NewRegistry() *Registry {
	return &Registry{
		handlerMap: make(map[string][]HandlerRecord),
	}
}

// RegisterNodeMapping creates a Prometheus gauge and handler for the given node mapping
func (r *Registry) RegisterNodeMapping(nodeMapping config.NodeMapping, promPrefix string, debug bool) error {
	metricName := nodeMapping.MetricName
	if metricName == "" {
		return errors.NewConfigError("metricName", metricName, fmt.Errorf("metric name cannot be empty"))
	}
	
	if promPrefix != "" {
		metricName = fmt.Sprintf("%s_%s", promPrefix, metricName)
	}
	
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: metricName,
		Help: "From OPC UA",
	})
	
	if err := prometheus.Register(gauge); err != nil {
		return errors.NewMetricError(metricName, nodeMapping.NodeName, err)
	}

	var handler handlers.MsgHandler
	if nodeMapping.ExtractBit != nil {
		extractBit, ok := nodeMapping.ExtractBit.(int)
		if !ok {
			return errors.NewConfigError("extractBit", nodeMapping.ExtractBit, fmt.Errorf("extractBit must be an integer, got %T", nodeMapping.ExtractBit))
		}
		handler = handlers.NewOpcuaBitVectorHandler(gauge, extractBit, debug)
	} else {
		handler = handlers.NewOpcValueHandler(gauge)
	}

	record := HandlerRecord{
		Config:  nodeMapping,
		Handler: handler,
	}
	
	r.handlerMap[nodeMapping.NodeName] = append(r.handlerMap[nodeMapping.NodeName], record)
	return nil
}

// GetHandlerMap returns the internal handler map for subscription management
func (r *Registry) GetHandlerMap() map[string][]HandlerRecord {
	return r.handlerMap
}

// CreateFromNodeMappings creates metrics for all provided node mappings
func (r *Registry) CreateFromNodeMappings(nodeMappings []config.NodeMapping, promPrefix string, debug bool) error {
	for _, nodeMapping := range nodeMappings {
		if err := r.RegisterNodeMapping(nodeMapping, promPrefix, debug); err != nil {
			return err // Error already wrapped with proper context
		}
	}
	return nil
}