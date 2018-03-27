package aicollect

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights/contracts"
)

type HTTPMiddleware struct {
	client appinsights.TelemetryClient
}

func NewHTTPMiddleware(client appinsights.TelemetryClient) *HTTPMiddleware {
	return &HTTPMiddleware{
		client: client,
	}
}

func (middleware *HTTPMiddleware) HandlerFunc(handler http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		middleware.ServeHTTP(rw, r, handler)
	}
}

func (middleware *HTTPMiddleware) Handler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		middleware.ServeHTTP(rw, r, handler.ServeHTTP)
	})
}

func (middleware *HTTPMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	startTime := time.Now()
	telem := appinsights.NewRequestTelemetry(r.Method, r.URL.String(), 0, "200")
	correlation, id := parseCorrelationHeaders(r)
	correlation.Name = telem.Name
	operation := appinsights.NewOperation(middleware.client, correlation)

	telem.Id = string(id)
	telem.Tags["ai.user.userAgent"] = r.UserAgent()
	telem.Tags[contracts.LocationIp] = getIp(r)
	telem.Source = getCorrelatedSource(correlation)

	newRequest := r.WithContext(appinsights.WrapContextRequestTelemetry(appinsights.WrapContextOperation(r.Context(), operation), telem))
	newWriter := &responseWriter{
		ResponseWriter: rw,
		telem:          telem,
		statusWritten:  false,
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

	next(newWriter, newRequest)
}

func getIp(req *http.Request) string {
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

type responseWriter struct {
	http.ResponseWriter
	telem         *appinsights.RequestTelemetry
	statusWritten bool
}

func (w *responseWriter) Write(data []byte) (int, error) {
	if !w.statusWritten {
		w.statusWritten = true
		w.telem.ResponseCode = "200"
		w.telem.Success = true
	}

	return w.ResponseWriter.Write(data)
}

func (w *responseWriter) WriteHeader(statusCode int) {
	if !w.statusWritten {
		w.statusWritten = true
		w.telem.ResponseCode = strconv.Itoa(statusCode)
		w.telem.Success = statusCode < 400 || statusCode == 401
	}

	w.ResponseWriter.WriteHeader(statusCode)
}
