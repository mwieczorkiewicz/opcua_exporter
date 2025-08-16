package handlers

import (
	"testing"
	"time"

	"github.com/gopcua/opcua/ua"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func getTestHandler() OpcValueHandler {
	var testGuage = prom.NewGauge(prom.GaugeOpts{Name: "foo"})
	return OpcValueHandler{testGuage}
}

func TestCoerceTimeValues(t *testing.T) {
	handler := getTestHandler()

	// Test with a valid time.Time value
	timeValue := time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)
	variant, err := ua.NewVariant(timeValue)
	assert.Nil(t, err)

	res, err := handler.FloatValue(*variant)
	assert.Nil(t, err)
	assert.Equal(t, float64(timeValue.Unix()), res)

	// Test with an invalid type
	invalidVariant, _ := ua.NewVariant("not a time")
	invalidVariant.Time()
	_, err = handler.FloatValue(*invalidVariant)
	assert.Error(t, err)
}

func TestTimeConversionToFloat(t *testing.T) {
	time := time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)
	ft, err := timeToFloat(time)
	assert.Nil(t, err)
	assert.Equal(t, float64(time.Unix()), ft)

	// Test with an invalid type
	invalidValue := "not a time"
	_, err = timeToFloat(invalidValue)
	assert.Error(t, err)
	assert.Equal(t, "expected a time.Time value, but got a string", err.Error())
}

func TestCoerceBooleanValues(t *testing.T) {
	handler := getTestHandler()

	variant, _ := ua.NewVariant(true)
	res, err := handler.FloatValue(*variant)
	assert.Nil(t, err)
	assert.Equal(t, 1.0, res)

	variant, _ = ua.NewVariant(false)
	res, err = handler.FloatValue(*variant)
	assert.Nil(t, err)
	assert.Equal(t, 0.0, res)
}

func TestCoerceNumericValues(t *testing.T) {
	handler := getTestHandler()

	type floatTest struct {
		input  interface{}
		output float64
	}

	testCases := []floatTest{
		{byte(0x03), 3.0},
		{int8(-4), -4.0},
		{int16(2), 2.0},
		{int32(33), 33.0},
		{int64(25), 25.0},
		{uint8(4), 4.0},
		{uint16(2), 2.0},
		{uint32(33), 33.0},
		{uint64(25), 25.0},
		{float32(8.8), float64(float32(8.8))}, // float32 --> float64 actually introduces rounding errors on the order of 1e-7
		{float64(238.4), 238.4},               // float32 --> float64 actually introduces rounding errors on the order of 1e-7
	}
	for _, testCase := range testCases {
		variant, e := ua.NewVariant(testCase.input)
		if e != nil {
			panic(e)
		}
		result, err := handler.FloatValue(*variant)
		assert.Nil(t, err)
		assert.Equal(t, testCase.output, result)
	}

}

func TestValueHandlerErrors(t *testing.T) {
	handler := getTestHandler()
	errorValues := []interface{}{
		"not a number",
	}

	for _, v := range errorValues {
		variant, vErr := ua.NewVariant(v)
		assert.Nil(t, vErr)
		_, err := handler.FloatValue(*variant)
		assert.Error(t, err)
	}
}
