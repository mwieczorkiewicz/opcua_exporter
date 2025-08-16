package main

import (
	"testing"

	"github.com/gopcua/opcua/monitor"
	"github.com/gopcua/opcua/ua"
	"github.com/mwieczorkiewicz/opcua_exporter/internal/config"
	"github.com/mwieczorkiewicz/opcua_exporter/internal/metrics"
	"github.com/stretchr/testify/assert"
)

/**
* mockHandler implements the MsgHandler interface
 */
type mockHandler struct {
	called bool // set if the Handle() func has been called
}

func (th *mockHandler) Handle(val ua.Variant) error {
	th.called = true
	return nil
}

func (th *mockHandler) FloatValue(val ua.Variant) (float64, error) {
	return 0.0, nil
}

func makeTestMessage(nodeID *ua.NodeID) monitor.DataChangeMessage {
	return monitor.DataChangeMessage{
		NodeID: nodeID,
		DataValue: &ua.DataValue{
			Value: ua.MustVariant(0.0),
		},
	}
}

// Exercise the handleMessage() function
// Ensure that it dispatches messages to handlers as expected
func TestHandleMessage(t *testing.T) {
	nodeID1 := ua.NewStringNodeID(1, "foo")
	nodeID2 := ua.NewStringNodeID(1, "bar")
	nodeName1 := nodeID1.String()
	nodeName2 := nodeID2.String()
	handlerMap := make(map[string][]metrics.HandlerRecord)
	for i := 0; i < 3; i++ {
		mapRecord := metrics.HandlerRecord{
			Config:  config.NodeMapping{NodeName: nodeName1, MetricName: "whatever"},
			Handler: &mockHandler{},
		}
		handlerMap[nodeName1] = append(handlerMap[nodeName1], mapRecord)
	}

	mapRecord2 := metrics.HandlerRecord{
		Config:  config.NodeMapping{NodeName: nodeName2, MetricName: "whatever"},
		Handler: &mockHandler{},
	}
	handlerMap[nodeName2] = append(handlerMap[nodeName2], mapRecord2)

	assert.Equal(t, len(handlerMap[nodeName1]), 3)
	assert.Equal(t, len(handlerMap[nodeName2]), 1)

	// Handle a fake message addressed to nodeID1
	msg := makeTestMessage(nodeID1)
	handleMessage(&msg, handlerMap, false)

	// All three nodeName1 handlers should have been called
	for _, record := range handlerMap[nodeName1] {
		handler := record.Handler.(*mockHandler)
		assert.True(t, handler.called)
	}

	// But not the nodeName2 handler
	handler := handlerMap[nodeName2][0].Handler.(*mockHandler)
	assert.False(t, handler.called)
}

// Exercise the metrics registry functionality
// Ensure that it creates the right sort of HandlerMap
func TestMetricsRegistry(t *testing.T) {
	nodeconfigs := []config.NodeMapping{
		{
			NodeName:   "foo",
			MetricName: "foo_level_blorbs",
		},
		{
			NodeName:   "bar",
			MetricName: "bar_level_blorbs",
		},
		{
			NodeName:   "foo",
			MetricName: "foo_rate_blarbs",
		},
	}

	registry := metrics.NewRegistry()
	err := registry.CreateFromNodeMappings(nodeconfigs, "", false)
	assert.NoError(t, err)
	
	handlerMap := registry.GetHandlerMap()
	assert.Equal(t, len(handlerMap), 2)
	assert.Equal(t, len(handlerMap["foo"]), 2)
	assert.Equal(t, len(handlerMap["bar"]), 1)
}
