package appinsights

import (
	"bytes"
	"strings"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights/contracts"
)

// Operation represents a logical operation (such as a request), and implements
// a TelemetryClient that attaches information about the operation to outgoing
// telemetry.
type Operation interface {
	TelemetryClient

	// Correlation returns the CorrelationContext for this operation.
	Correlation() *CorrelationContext
}

type operation struct {
	telemetryClient
	correlationContext *CorrelationContext
	originalClient     TelemetryClient
}

// CorrelationContext contains the IDs and other related information for a
// logical operation to be correlated across services.
type CorrelationContext struct {
	// Name is the operation name
	Name string

	// Id is the operation ID
	Id OperationId

	// ParentId is the operation's parent ID (such as request ID).
	ParentId OperationId

	// Properties is a map of user-defined custom properties
	Properties CorrelationProperties
}

// CorrelationProperties is a serializable map of custom properties used when
// correlating operations across services.
type CorrelationProperties map[string]string

// NewOperation creates an Operation instance with the specified correlation
// information.
func NewOperation(client TelemetryClient, correlation *CorrelationContext) Operation {
	context := NewTelemetryContext(client.InstrumentationKey())
	context.CommonProperties = client.Context().CommonProperties

	for k, v := range client.Context().Tags {
		context.Tags[k] = v
	}

	context.Tags[contracts.OperationParentId] = correlation.ParentId.String()
	context.Tags[contracts.OperationId] = correlation.Id.String()
	context.Tags[contracts.OperationName] = correlation.Name

	return &operation{
		correlationContext: correlation,
		originalClient:     client,
		telemetryClient: telemetryClient{
			channel:   client.Channel(),
			context:   context,
			config:    client.Config(),
			isEnabled: client.IsEnabled(),
			sampling:  client.GetSamplingPercentage(),
		},
	}
}

// Correlation returns the correlation context for this operation.
func (op *operation) Correlation() *CorrelationContext {
	return op.correlationContext
}

// CorrelationId returns the unique ID used to represent this application
// when correlating operations across services.  This is fetched from
// Application Insights servers using the instrumentation key.
func (op *operation) CorrelationId() string {
	return op.originalClient.CorrelationId()
}

// NewCorrelationContext creates a CorrelationContext with the specified IDs
// and properties.
func NewCorrelationContext(operationId, parentId OperationId, name string, properties CorrelationProperties) *CorrelationContext {
	if string(operationId) == "" {
		operationId = NewOperationId()
	}

	if string(parentId) == "" {
		parentId = operationId
	}

	if properties == nil {
		properties = make(CorrelationProperties)
	}

	return &CorrelationContext{
		Name:       name,
		Id:         operationId,
		ParentId:   parentId,
		Properties: properties,
	}
}

// ParseCorrelationProperties deserializes the custom property bag from the
// format used in HTTP headers.
func ParseCorrelationProperties(header string) CorrelationProperties {
	result := make(CorrelationProperties)

	entries := strings.Split(header, ",")
	for _, entry := range entries {
		kv := strings.SplitN(entry, "=", 2)
		if len(kv) == 2 {
			result[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}

	return result
}

// Serialize serializes this property bag to a format that can be transmitted
// in HTTP headers.
func (props CorrelationProperties) Serialize() string {
	var result bytes.Buffer
	for k, v := range props {
		if strings.ContainsRune(k, ',') || strings.ContainsRune(k, '=') || strings.ContainsRune(v, ',') || strings.ContainsRune(v, '=') {
			diagnosticsWriter.Printf("Custom properties must not contains '=' or ','. Dropping key \"%s\"", k)
		} else {
			if result.Len() > 0 {
				result.WriteRune(',')
			}
			result.WriteString(k)
			result.WriteRune('=')
			result.WriteString(v)
		}
	}

	return result.String()
}
