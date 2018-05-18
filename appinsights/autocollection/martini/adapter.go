package martini

import (
	"net/http"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights/autocollection"
	"github.com/go-martini/martini"
)

// MartiniAdapter is an adapter to make Application Insights' autocollection.HTTPMiddleware
// work natively within Martini.
func MartiniAdapter(middleware *autocollection.HTTPMiddleware) martini.Handler {
	return func(rw http.ResponseWriter, r *http.Request, c martini.Context) {
		middleware.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			c.MapTo(rw, (*http.ResponseWriter)(nil))
			c.Map(r)
			c.Next()
		})(rw, r)
	}
}
