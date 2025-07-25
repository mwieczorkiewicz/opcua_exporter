package handlers

import "github.com/gopcua/opcua/ua"

type MsgHandler interface {
	FloatValue(v ua.Variant) (float64, error) // metric value to be emitted
	Handle(v ua.Variant) error                // compute the metric value and publish it
}
