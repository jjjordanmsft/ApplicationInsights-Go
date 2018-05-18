package autocollection

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights/contracts"
)

// HTTPMiddleware is a generic middleware that logs incoming requests to
// Application Insights.
type HTTPMiddleware struct {
	client appinsights.TelemetryClient
	config *HTTPMiddlewareConfiguration
}

// HTTPMiddlewareConfiguration specifies flags that modify the behavior of an
// HTTPMiddleware. For better forward-compatibility, callers should modify the
// result from NewHTTPMiddlewareConfiguration() rather than instantiate this
// structure directly.
type HTTPMiddlewareConfiguration struct {
	// SendCorrelationHeaders specifies whether the middleware should emit
	// correlation headers to clients.
	SendCorrelationHeaders bool

	// LogUserAgent specifies whether to log the user agent. Defaults to
	// false for data usage savings.
	LogUserAgent bool
}

// NewHTTPMiddlewareConfiguration returns a new, default configuration for the
// HTTP middleware.
func NewHTTPMiddlewareConfiguration() *HTTPMiddlewareConfiguration {
	return &HTTPMiddlewareConfiguration{
		SendCorrelationHeaders: true,
		LogUserAgent:           false,
	}
}

// NewHTTPMiddleware creates a middleware that uses the specified TelemetryClient.
func NewHTTPMiddleware(client appinsights.TelemetryClient, config *HTTPMiddlewareConfiguration) *HTTPMiddleware {
	if config == nil {
		config = NewHTTPMiddlewareConfiguration()
	}

	return &HTTPMiddleware{
		client: client,
		config: config,
	}
}

// HandlerFunc wraps the specified http.HandlerFunc with this middleware.
func (middleware *HTTPMiddleware) HandlerFunc(handler http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		middleware.ServeHTTP(rw, r, handler)
	}
}

// Handler wraps the specified http.Handler with this middleware.
func (middleware *HTTPMiddleware) Handler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		middleware.ServeHTTP(rw, r, handler.ServeHTTP)
	})
}

// ServeHTTP invokes this middleware's handler, which then calls next.
// Note: This signature is needed so the middleware object works directly
// with negroni.
func (middleware *HTTPMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	newRequest, telem, operation := middleware.BeginRequest(r)
	for k, v := range middleware.GetCorrelationHeaders(r, operation) {
		rw.Header().Set(k, v)
	}

	defer middleware.CompleteRequest(telem, operation)
	next(newResponseWriter(rw, telem), newRequest)
}

// BeginRequest creates and returns a telemetry item, an operation, and a wrapped
// version of the request to pass to successive handlers.
func (middleware *HTTPMiddleware) BeginRequest(r *http.Request) (*http.Request, *appinsights.RequestTelemetry, appinsights.Operation) {
	startTime := time.Now()
	telem := appinsights.NewRequestTelemetry(r.Method, r.URL.String(), 0, "200")
	telem.Timestamp = startTime

	headers := parseCorrelationRequestHeaders(r)
	correlation := appinsights.NewCorrelationContext(headers.requestId.GetRoot(), headers.requestId, telem.Name, headers.properties)
	if headers.parentId.String() == "" {
		telem.Tags[contracts.OperationParentId] = headers.rootId
	} else {
		telem.Tags[contracts.OperationParentId] = headers.parentId.String()
	}

	operation := appinsights.NewOperation(middleware.client, correlation)
	telem.Id = string(headers.requestId)
	telem.Tags[contracts.LocationIp] = getIP(r)
	telem.Source = headers.getCorrelatedSource()
	// TODO: Referer uri

	if middleware.config.LogUserAgent {
		telem.Tags["ai.user.userAgent"] = r.UserAgent()
	}

	newRequest := r.WithContext(appinsights.WrapContextRequestTelemetry(appinsights.WrapContextOperation(r.Context(), operation), telem))
	return newRequest, telem, operation
}

// GetCorrelationHeaders returns the correlation headers to add to the response
// for the specified request and operation, if applicable.
func (middleware *HTTPMiddleware) GetCorrelationHeaders(r *http.Request, operation appinsights.Operation) map[string]string {
	if middleware.config.SendCorrelationHeaders {
		return getCorrelationResponseHeaders(operation)
	} else {
		return nil
	}
}

// CompleteRequest wraps up the request and submits the telemetry item.  This
// will track panics if called from a defer.
func (middleware *HTTPMiddleware) CompleteRequest(telem *appinsights.RequestTelemetry, operation appinsights.Operation) {
	r := recover()
	if r != nil {
		telem.SetResponseCode(500)
		operation.TrackException(r)
	}

	telem.Duration = time.Since(telem.Timestamp)
	operation.Track(telem)

	if r != nil {
		panic(r)
	}
}

func getIP(req *http.Request) string {
	if xff := req.Header.Get("x-forwarded-for"); xff != "" {
		if comma := strings.IndexByte(xff, ','); comma >= 0 {
			firstIP := strings.TrimSpace(xff[:comma])
			if net.ParseIP(firstIP) != nil {
				return firstIP
			}
		}

		if net.ParseIP(xff) != nil {
			return xff
		}
	}

	if raddr := req.RemoteAddr; raddr != "" {
		if raddr[0] == '[' {
			// IPv6
			rbracket := strings.IndexByte(raddr, ']')
			ip := raddr[1:rbracket]
			if net.ParseIP(ip) != nil {
				return ip
			}
		}

		if colon := strings.IndexByte(raddr, ':'); colon >= 0 {
			ip := raddr[:colon]
			if net.ParseIP(ip) != nil {
				return ip
			}
		}

		if net.ParseIP(raddr) != nil {
			return raddr
		}
	}

	return ""
}

// responseWriter wraps an http.ResponseWriter that captures the status code
// and writes it to a telemetry item.
type responseWriter struct {
	http.ResponseWriter
	telem         *appinsights.RequestTelemetry
	statusWritten bool
}

type responseWriterPusher struct {
	http.ResponseWriter
	http.Pusher
}

// newResponseWriter wraps the specified http.ResponseWriter and http.Pusher
// (if available).
func newResponseWriter(rw http.ResponseWriter, telem *appinsights.RequestTelemetry) http.ResponseWriter {
	newWriter := &responseWriter{
		ResponseWriter: rw,
		telem:          telem,
		statusWritten:  false,
	}

	// If the input ResponseWriter supports push, we must return one that does as well.
	if pusher, ok := rw.(http.Pusher); ok {
		return &responseWriterPusher{
			ResponseWriter: newWriter,
			Pusher:         pusher,
		}
	} else {
		return newWriter
	}
}

func (w *responseWriter) Write(data []byte) (int, error) {
	if !w.statusWritten {
		w.statusWritten = true
		w.telem.SetResponseCode(200)
	}

	return w.ResponseWriter.Write(data)
}

func (w *responseWriter) WriteHeader(statusCode int) {
	if !w.statusWritten {
		w.statusWritten = true
		w.telem.SetResponseCode(statusCode)
	}

	w.ResponseWriter.WriteHeader(statusCode)
}
