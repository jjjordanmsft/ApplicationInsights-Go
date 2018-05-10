package appinsights

import (
	"context"
	"net/http"
)

type contextKey string

const (
	operationContextKey        contextKey = "Microsoft.ApplicationInsights.Operation"
	requestTelemetryContextKey contextKey = "Microsoft.ApplicationInsights.RequestTelemetry"
	ignoreContextKey           contextKey = "Microsoft.ApplicationInsights.Ignore"
)

// WrapContextOperation embeds an Operation in the specified context.Context
func WrapContextOperation(ctx context.Context, op Operation) context.Context {
	return context.WithValue(ctx, operationContextKey, op)
}

// OperationFromContext retrieves an embedded Operation from the specified context.Context
func OperationFromContext(ctx context.Context) Operation {
	if obj := ctx.Value(operationContextKey); obj != nil {
		if op, ok := obj.(*operation); ok {
			return op
		}
	}

	return nil
}

// MarkContextIgnore specifies that any outgoing dependency tracking should be disabled
// for all requests originating from this context.Context
func MarkContextIgnore(ctx context.Context) context.Context {
	return context.WithValue(ctx, ignoreContextKey, true)
}

// MarkRequestIgnore specifies that this Request should not be tracked as a dependency
func MarkRequestIgnore(req *http.Request) *http.Request {
	return req.WithContext(MarkContextIgnore(req.Context()))
}

// CheckContextIgnore tests whether the specified context.Context was marked with the ignore bit
func CheckContextIgnore(ctx context.Context) bool {
	return ctx.Value(ignoreContextKey) != nil
}

// WrapContextRequestTelemetry embeds a RequestTelemetry item in the specified context.Context
func WrapContextRequestTelemetry(ctx context.Context, t *RequestTelemetry) context.Context {
	return context.WithValue(ctx, requestTelemetryContextKey, t)
}

// RequestTelemetryFromContext retrieves an embedded RequestTelemetry from the specified context.Context
func RequestTelemetryFromContext(ctx context.Context) *RequestTelemetry {
	if obj := ctx.Value(requestTelemetryContextKey); obj != nil {
		if t, ok := obj.(*RequestTelemetry); ok {
			return t
		}
	}

	return nil
}
