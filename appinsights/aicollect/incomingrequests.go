package aicollect

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
)

func NewHTTPMiddlewareFactory(client appinsights.TelemetryClient) func(http.HandlerFunc) http.HandlerFunc {
	return func(handler http.HandlerFunc) http.HandlerFunc {
		return NewHTTPMiddleware(client, handler)
	}
}

func NewHTTPMiddleware(client appinsights.TelemetryClient, handler http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		telem := appinsights.NewRequestTelemetry(r.Method, r.URL.String(), 0, "200")
		correlation := parseCorrelationHeaders(r)
		operation := appinsights.NewOperation(client, correlation)

		newRequest := r.WithContext(appinsights.WrapContextOperation(r.Context(), operation))
		newWriter := &responseWriter{
			telem:         telem,
			writer:        rw,
			statusWritten: false,
		}

		defer func() {
			r := recover()
			if r != nil {
				telem.Success = false
				telem.ResponseCode = "500"
				operation.TrackException(r)
			}

			telem.MarkTime(startTime, time.Now())
			operation.Track(telem)

			if r != nil {
				panic(r)
			}
		}()

		handler(newWriter, newRequest)
	}
}

type responseWriter struct {
	telem         *appinsights.RequestTelemetry
	writer        http.ResponseWriter
	statusWritten bool
}

func (w *responseWriter) Header() http.Header {
	return w.writer.Header()
}

func (w *responseWriter) Write(data []byte) (int, error) {
	if !w.statusWritten {
		w.statusWritten = true
		w.telem.ResponseCode = "200"
		w.telem.Success = true
	}

	return w.writer.Write(data)
}

func (w *responseWriter) WriteHeader(statusCode int) {
	if !w.statusWritten {
		w.statusWritten = true
		w.telem.ResponseCode = strconv.Itoa(statusCode)
		w.telem.Success = statusCode < 400 || statusCode == 401
	}

	w.writer.WriteHeader(statusCode)
}

func parseCorrelationHeaders(r *http.Request) *appinsights.CorrelationContext {
	// TODO: Parse it out, or create a new one.
	return nil
}
