package handlers

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/gopcua/opcua/ua"
	"github.com/prometheus/client_golang/prometheus"
)

// OpcValueHandler handles generic OPC message values
// Unfortunately, we don't know the message type at construction time.
type OpcValueHandler struct {
	gauge prometheus.Gauge
}

// NewOpcValueHandler creates a new OpcValueHandler with the specified gauge.
func NewOpcValueHandler(g prometheus.Gauge) OpcValueHandler {
	return OpcValueHandler{
		gauge: g,
	}
}

// Handle the message by determining the float value
// and emitting it as a gauge metric
func (h OpcValueHandler) Handle(v ua.Variant) error {
	floatVal, err := h.FloatValue(v)
	if err != nil {
		return err
	}
	h.gauge.Set(floatVal)
	return nil
}

// FloatValue converts a ua.Variant to float64
// All prometheus metrics are float64.
// Since OPCUA message values have variable types, sort out how to convert them to float.
func (h OpcValueHandler) FloatValue(v ua.Variant) (float64, error) {
	switch v.Type() {
	case ua.TypeIDNull:
		return 0.0, errors.New("cannot convert null value to float64")
	case ua.TypeIDBoolean:
		return boolToFloat(v.Value())
	case ua.TypeIDDateTime:
		return timeToFloat(v.Value())
	default:
		return coerceToFloat64(v.Value())
	}
}

func timeToFloat(v any) (float64, error) {
	switch t := v.(type) {
	case time.Time:
		return float64(t.Unix()), nil
	default:
		return 0.0, fmt.Errorf("expected a time.Time value, but got a %T", v)
	}
}

func boolToFloat(v any) (float64, error) {
	if b, ok := v.(bool); ok {
		if b {
			return 1.0, nil
		}
		return 0.0, nil
	}
	return fallbackBooleans(v)
}

func fallbackBooleans(v any) (float64, error) {
	reflectedVal := reflect.ValueOf(v)
	reflectedVal = reflect.Indirect(reflectedVal)

	if reflectedVal.Type().Kind() != reflect.Bool {
		return 0.0, fmt.Errorf("expected a bool value, but got a %s", reflectedVal.Type())
	}
	b := reflectedVal.Bool()
	if b {
		return 1.0, nil
	}
	return 0.0, nil
}

func coerceToFloat64(unknown interface{}) (float64, error) {
	switch val := unknown.(type) {
	case float64:
		return val, nil
	case float32:
		return float64(val), nil
	case int:
		return float64(val), nil
	case int8:
		return float64(val), nil
	case int16:
		return float64(val), nil
	case int32:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case uint:
		return float64(val), nil
	case uint8:
		return float64(val), nil
	case uint16:
		return float64(val), nil
	case uint32:
		return float64(val), nil
	case uint64:
		return float64(val), nil
	default:
		return 0.0, fmt.Errorf("unsupported type for float conversion: %T", unknown)
	}
}
