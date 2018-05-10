package appinsights

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestContextOperation(t *testing.T) {
	mockCidLookup(nil)
	defer resetCidLookup()

	_, client := newMockChannelClient(nil)
	operation := NewOperation(client, NewCorrelationContext(NewOperationId(), NewOperationId(), "", nil))

	ctx := context.Background()
	wrappedCtx := WrapContextOperation(ctx, operation)

	if OperationFromContext(wrappedCtx) != operation {
		t.Error("Unable to recover Operation")
	}

	if OperationFromContext(ctx) != nil {
		t.Error("non-nil result from empty context")
	}
}

func TestContextIgnore(t *testing.T) {
	ctx := context.Background()
	wrappedCtx := MarkContextIgnore(ctx)

	if CheckContextIgnore(ctx) {
		t.Error("Default context should not have ignore bit set")
	}

	if !CheckContextIgnore(wrappedCtx) {
		t.Error("Wrapped context should have ignore bit set")
	}
}

func TestRequestIgnore(t *testing.T) {
	req, err := http.NewRequest("GET", "https://portal.azure.com/", nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	if CheckContextIgnore(req.Context()) {
		t.Error("Default request should not have ignore bit set")
	}

	wrappedRequest := MarkRequestIgnore(req)
	if !CheckContextIgnore(wrappedRequest.Context()) {
		t.Error("Wrapped request should have ignore bit set")
	}

	if wrappedRequest.Method != "GET" {
		t.Error("Request method was not preserved")
	}

	if wrappedRequest.URL.String() != "https://portal.azure.com/" {
		t.Error("URL wasn't preserved")
	}
}

func TestWrapRequestTelemetry(t *testing.T) {
	req := NewRequestTelemetry("GET", "https://portal.azure.com/", time.Second, "200")

	ctx := context.Background()
	wrappedCtx := WrapContextRequestTelemetry(ctx, req)

	if RequestTelemetryFromContext(ctx) != nil {
		t.Error("Default context should not have request telemetry")
	}

	if RequestTelemetryFromContext(wrappedCtx) != req {
		t.Error("Wrapped context should have request telemetry")
	}
}
