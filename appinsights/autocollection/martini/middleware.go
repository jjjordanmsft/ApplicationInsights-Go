package martini

import (
	"net/http"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights/autocollection"
	"github.com/go-martini/martini"
)

func Middleware(telemetryClient appinsights.TelemetryClient) martini.Handler {
	middleware := autocollection.NewHTTPMiddleware(telemetryClient)
	return func(rw http.ResponseWriter, r *http.Request, c martini.Context) {
		middleware.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			c.MapTo(rw, (*http.ResponseWriter)(nil))
			c.Map(r)
			c.Next()
		})(rw, r)
	}
}
