package buffalo

import (
	"net/http"
	"strconv"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights/autocollection"
	"github.com/gobuffalo/buffalo"
	"github.com/gobuffalo/buffalo/render"
)

// Middleware is an adapter so that the AI middleware's HandlerFunc can be used in buffalo.
// This should be added via:
//   app.Use(Middleware(telemetryClient))
func Middleware(telemetryClient appinsights.TelemetryClient) buffalo.MiddlewareFunc {
	middleware := autocollection.NewHTTPMiddleware(telemetryClient)
	return func(next buffalo.Handler) buffalo.Handler {
		return func(c buffalo.Context) error {
			var err error
			middleware.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				err = next(&context{
					Context:  c,
					request:  r,
					response: rw,
				})
			})(c.Response(), c.Request().WithContext(c) /* needed? */)

			return err
		}
	}
}

type context struct {
	buffalo.Context
	request  *http.Request
	response http.ResponseWriter
}

func (ctx *context) Request() *http.Request {
	return ctx.request
}

func (ctx *context) Response() http.ResponseWriter {
	return ctx.response
}

func (ctx *context) Render(code int, rr render.Renderer) error {
	telem := appinsights.RequestTelemetryFromContext(ctx)
	if telem != nil {
		telem.ResponseCode = strconv.Itoa(code)
		telem.Success = code < 400 || code == 401
	}

	return ctx.Context.Render(code, rr)
}

func (ctx *context) Value(key interface{}) interface{} {
	return ctx.request.Context().Value(key)
}
