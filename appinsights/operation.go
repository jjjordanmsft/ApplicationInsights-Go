package appinsights

import (
	"bytes"
	"math"
	
	"github.com/jjjordanmsft/ApplicationInsights-Go/appinsights/contracts"
)

type Operation struct {
	client             TelemetryClient
	context            *TelemetryContext
	isEnabled          bool
	CorrelationContext *CorrelationContext
}

// Custom properties for use with correlation headers.  Keys and values must
// not contain ',' or '='.
type CustomProperties map[string]string

type CorrelationContext struct {
	Name     string
	Id       string
	ParentId string

	// Needed?
	Properties CustomProperties
}

func NewCorrelationContext(operationId, parentId, name, header string) *CorrelationContext {
	return &CorrelationContext{
		Name:       name,
		Id:         operationId,
		ParentId:   parentId,
		Properties: ParseCustomProperties(header),
	}
}

// Parses correlation properties header and returns a CustomProperties object
func ParseCustomProperties(header string) CustomProperties {
	result := make(CustomProperties)

	entries := strings.Split(header, ", ")
	for _, entry := range entries {
		kv := strings.SplitN(entry, "=", 2)
		result[kv[0]] = result[kv[1]]
	}

	return result
}

func (props CustomProperties) Serialize() string {
	var result bytes.Buffer
	for k, v := range props {
		if strings.Contains(k, ',') || strings.Contains(k, '=') || strings.Contains(v, ',') || strings.Contains(v, '=') {
			diagnosticsWriter.Printf("Custom properties must not contain '=' or ','. Dropping key \"%s\"", k)
		} else {
			if result.Len() > 0 {
				result.Write(", ")
			}
			result.Write(k)
			result.Write("=")
			result.Write(v)
		}
	}

	return result.String()
}

// Standard sampling hash method for AI.  Takes an operation ID as input and
// returns a "hash code" between 0.0 and 100.0.  Compared against sampling
// percentage to determine inclusion.
func samplingHash(id string) float64 {
	if id == "" {
		return 0
	}

	for len(id) < 8 {
		id = id + id
	}

	// djb2, with + not ^
	var hash int32 = 5381
	for _, c := range id {
		hash = (hash << 5) + hash + int32(c)
	}

	if hash == math.MinInt32 {
		// handle int asymmetry
		hash = math.MaxInt32
	}

	return (math.Abs(float64(hash)) / float64(math.MaxInt32)) * 100.0
}

func NewOperation(client TelemetryClient, correlationContext *CorrelationContext) (*Operation, error) {
	if client == nil {
		return nil, fmt.Errorf("Client is nil")
	}
	if correlationContext == nil {
		return nil, fmt.Errorf("Correlation context is nil")
	}

	origContext := client.Context()
	context := NewTelemetryContext()
	context.iKey = origContext.iKey
	for k, v := range origContext.Tags {
		context.Tags[k] = v
	}

	context.Operation().SetName(correlationContext.Name)
	context.Operation().SetId(correlationContext.Id)
	context.Operation().SetParentId(correlationContext.ParentId)

	return &Operation{
		client:             client,
		CorrelationContext: correlationContext,
		context:            context,
		isEnabled:          true,
	}, nil
}

func (op *Operation) Track(item Telemetry) {
	if item != nil && op.isEnabled && op.client.IsEnabled() {
		op.client.Channel().Send(op.context.envelop(item))
	}
}

func (op *Operation) IsEnabled() bool {
	return op.isEnabled && op.client.IsEnabled()
}

func (op *Operation) SetIsEnabled(value bool) {
	op.isEnabled = value
}

func (op *Operation) Context() *TelemetryContext {
	return op.context
}

func (op *Operation) Channel() TelemetryChannel {
	return op.client.Channel()
}

func (op *Operation) InstrumentationKey() string {
	return op.client.InstrumentationKey()
}

func (op *Operation) TrackEvent(name string) {
	op.Track(NewEventTelemetry(name))
}

func (op *Operation) TrackMetric(name string, value float64) {
	op.Track(NewMetricTelemetry(name, value))
}

func (op *Operation) TrackTrace(message string, severity contracts.SeverityLevel) {
	op.Track(NewTraceTelemetry(message, severity))
}

func (op *Operation) TrackRequest(method, url string, duration time.Duration, responseCode string) {
	op.Track(NewRequestTelemetry(method, url, duration, responseCode))
}

func (op *Operation) TrackRemoteDependency(name, dependencyType, target string, success bool) {
	op.Track(NewRemoteDependencyTelemetry(name, dependencyType, target, success))
}

func (op *Operation) TrackAvailability(name string, duration time.Duration, success bool) {
	op.Track(NewAvailabilityTelemetry(name, duration, success))
}

func (op *Operation) TrackException(err interface{}) {
	op.Track(NewExceptionTelemetry(err))
}
