package appinsights

import (
	"context"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights/contracts"
)

const operationKey = "Microsoft.ApplicationInsights.Operation"

type Operation interface {
	TelemetryClient
	Correlation() *CorrelationContext
}

type operation struct {
	telemetryClient
	correlationContext *CorrelationContext
}

func NewOperation(client TelemetryClient, correlation *CorrelationContext) Operation {
	context := NewTelemetryContext()
	context.iKey = client.InstrumentationKey()
	context.CommonProperties = client.Context().CommonProperties

	for k, v := range client.Context().Tags {
		context.Tags[k] = v
	}

	context.Tags[contracts.OperationId] = string(correlation.Id)
	context.Tags[contracts.OperationParentId] = string(correlation.ParentId)
	context.Tags[contracts.OperationName] = correlation.Name

	return &operation{
		correlationContext: correlation,
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

func GetContextOperation(ctx context.Context) Operation {
	if obj := ctx.Value(operationKey); obj != nil {
		if op, ok := obj.(Operation); ok {
			return op
		}
	}

	return nil
}

func WrapContextOperation(ctx context.Context, op Operation) context.Context {
	return context.WithValue(ctx, operationKey, op)
}
