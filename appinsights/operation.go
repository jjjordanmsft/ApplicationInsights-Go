package appinsights

import (
	"github.com/Microsoft/ApplicationInsights-Go/appinsights/contracts"
)

type Operation interface {
	TelemetryClient
	Correlation() *CorrelationContext
}

type operation struct {
	telemetryClient
	correlationContext *CorrelationContext
	originalClient TelemetryClient
}

func NewOperation(client TelemetryClient, correlation *CorrelationContext) Operation {
	context := NewTelemetryContext(client.InstrumentationKey())
	context.CommonProperties = client.Context().CommonProperties

	for k, v := range client.Context().Tags {
		context.Tags[k] = v
	}

	//context.Tags[contracts.OperationParentId] = string(correlation.Id)
	context.Tags[contracts.OperationParentId] = string(correlation.ParentId)
	context.Tags[contracts.OperationId] = string(correlation.Id)
	context.Tags[contracts.OperationName] = correlation.Name

	return &operation{
		correlationContext: correlation,
		originalClient: client,
		telemetryClient: telemetryClient{
			channel:   client.Channel(),
			context:   context,
			config:    client.Config(),
			isEnabled: true,
		},
	}
}

func (op *operation) Correlation() *CorrelationContext {
	return op.correlationContext
}

func (op *operation) CorrelationId() string {
	return op.originalClient.CorrelationId()
}
