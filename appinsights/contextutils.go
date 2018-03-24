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

func WrapContextOperation(ctx context.Context, op Operation) context.Context {
	return context.WithValue(ctx, operationContextKey, op)
}

func OperationFromContext(ctx context.Context) Operation {
	if obj := ctx.Value(operationContextKey); obj != nil {
		if op, ok := obj.(*operation); ok {
			return op
		}
	}

	return nil
}

func MarkContextIgnore(ctx context.Context) context.Context {
	return context.WithValue(ctx, ignoreContextKey, true)
}

func MarkRequestIgnore(req *http.Request) *http.Request {
	return req.WithContext(MarkContextIgnore(req.Context()))
}

func CheckContextIgnore(ctx context.Context) bool {
	return ctx.Value(ignoreContextKey) != nil
}

func WrapContextRequestTelemetry(ctx context.Context, t *RequestTelemetry) context.Context {
	return context.WithValue(ctx, requestTelemetryContextKey, t)
}

func RequestTelemetryFromContext(ctx context.Context) *RequestTelemetry {
	if obj := ctx.Value(requestTelemetryContextKey); obj != nil {
		if t, ok := obj.(*RequestTelemetry); ok {
			return t
		}
	}

	return nil
}
