package collection

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

const OperationKey = "appinsights.operation"

func NewHTTPMiddleware(client TelemetryClient, handler http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		
		telem := appinsights.NewRequestTelemetry(r.Method, r.URL.String(), 0, "200")
		
		// TODO: Pull correlation headers, if applicable.
		
		operation := appinsights.NewOperation(client, "???")
		
		// Pop in the operation to the context
		parentContext := r.Context()
		context := context.WithValue(parentContext, OperationKey, operation)
		newRequest := r.WithContext(context)
		
		proxy := &proxyWriter{request: telem, writer: rw}
		
		// Track any panics caused by the handler
		defer func trackPanic() {
			if r := recover(); r != nil {
				// Wrap up the current request -- mark as a 500.
				telem.Success = false
				telem.StatusCode = "500"
				telem.MarkTime(startTime, time.Now())
				operation.Track(telem)
				
				// Track the panic.
				operation.TrackException(r)
				
				// Throw, let an upstream middleware handle this.
				panic(r)
			}
		}()
		
		handler(proxy, newRequest)
		
		telem.MarkTime(startTime, time.Now())
		operation.Track(telem)
	}
}

func NewHTTPMiddlewareMaker(client TelemetryClient) func(http.HandlerFunc) http.HandlerFunc {
	return func(handler http.HandlerFunc) http.HandlerFunc {
		return NewHTTPMiddleware(client, handler)
	}
}

func ExtractOperationFromRequest(request *http.Request) *appinsights.Operation {
	return ExtractOperation(request.Context())
}

func ExtractOperation(ctx *context.Context) *appinsights.Operation {
	value := ctx.Value(OperationKey)
	if value != nil {
		if op, ok := value.(*appinsights.Operation); ok {
			return op
		}
	}
	
	return nil
}

func WrapOperationContext(ctx *context.Context, *appinsights.Operation) *context.Context {
	if ctx == nil {
		return context.
	}
}

type proxyWriter struct {
	request       *appinsights.RequestTelemetry
	writer        http.ResponseWriter
	statusWritten bool
}

func (proxy *proxyWriter) Header() http.Header {
	return proxy.writer.Header()
}

func (proxy *proxyWriter) Write(data []byte) (int, error) {
	if !proxy.statusWritten {
		proxy.statusWritten = true
		proxy.request.StatusCode = "200"
		proxy.request.Success = true
	}
	
	return proxy.writer.Write(data)
}

func (proxy *proxyWriter) WriteHeader(statusCode int) {
	proxy.writer.WriteHeader(statusCode)
	proxy.request.StatusCode = strconv.Itoa(statusCode)
	proxy.request.Success = statusCode < 400 || statusCode == 401
	proxy.statusWritten = true
}
