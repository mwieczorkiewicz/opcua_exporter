package main

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

// Handle the message by deturmingint the float value
// and emiting it as a gauge metric
func (h OpcValueHandler) Handle(v ua.Variant) error {
	floatVal, err := h.FloatValue(v)
	if err != nil {
		return err
	}
	h.gauge.Set(floatVal)
	return nil
}

// FloatValue converts a ua.Variant to float64
// All prometheus metics are float64.
// Since OPCUA message values have variable types, sort out how to convert them to float.
func (h OpcValueHandler) FloatValue(v ua.Variant) (float64, error) {
	switch v.Type() {
	case ua.TypeIDNull:
		return 0.0, errors.New("Can not convert null value to float64")
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
		return 0.0, fmt.Errorf("Expected a time.Time value, but got a %T", v)
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
		return 0.0, fmt.Errorf("Expected a bool value, but got a %s", reflectedVal.Type())
	}
	b := reflectedVal.Bool()
	if b {
		return 1.0, nil
	}
	return 0.0, nil
}

func coerceToFloat64(unknown interface{}) (float64, error) {
	v := reflect.ValueOf(unknown)
	v = reflect.Indirect(v)

	floatType := reflect.TypeOf(0.0)
	if v.Type().ConvertibleTo(floatType) {
		return v.Convert(floatType).Float(), nil
	}

	return 0.0, fmt.Errorf("Unfloatable type: %v", v.Type())

}
