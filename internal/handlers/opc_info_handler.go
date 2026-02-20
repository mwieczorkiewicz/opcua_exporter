package handlers

import (
	"github.com/gopcua/opcua/ua"
	"github.com/prometheus/client_golang/prometheus"
)

// OpcInfoHandler handles OPC message string values and returns them as an info metric
type OpcInfoHandler struct {
	gauge prometheus.GaugeVec
}

// NewOpcInfoHandler creates a new OpcInfoHandler with the specified gauge.
func NewOpcInfoHandler(g prometheus.GaugeVec) OpcInfoHandler {
	return OpcInfoHandler{
		gauge: g,
	}
}

// Handle the message by setting value to a label value
// and emitting it as a gauge metric with label
func (h OpcInfoHandler) Handle(v ua.Variant) error {
	h.gauge.WithLabelValues(v.String()).Set(1.0)
	return nil
}

// FloatValue is not used for OpcInfoHandler
func (h OpcInfoHandler) FloatValue(v ua.Variant) (float64, error) {
	return 1.0, nil
}
