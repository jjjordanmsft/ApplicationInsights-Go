package buffalo

import (
	"net/http"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights/autocollection"
	"github.com/gobuffalo/buffalo"
	"github.com/gobuffalo/buffalo/render"
)

// BuffaloAdapter is an adapter to make Application Insights' autocollection.HTTPMiddleware
// work natively within buffalo.
// This should be added via:
//   app.Use(BuffaloAdapter(autocollection.NewHTTPMiddleware(telemetryClient, config)))
func BuffaloAdapter(middleware *autocollection.HTTPMiddleware) buffalo.MiddlewareFunc {
	return func(next buffalo.Handler) buffalo.Handler {
		return func(c buffalo.Context) error {
			var err error
			middleware.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				myctx := &context{
					Context:  c,
					request:  r,
					response: rw,
				}

				telem := appinsights.RequestTelemetryFromContext(r.Context())

				err = next(myctx)
				if err != nil && err == myctx.err && myctx.errTelem != nil {
					if operation := appinsights.OperationFromContext(r.Context()); operation != nil {
						operation.Track(myctx.errTelem)
						telem.SetResponseCode(myctx.errStatus)
					}
				} else if br, ok := c.Response().(*buffalo.Response); ok {
					telem.SetResponseCode(br.Status)
				}
			})(c.Response(), c.Request().WithContext(c) /* needed? */)

			return err
		}
	}
}

type context struct {
	buffalo.Context
	request   *http.Request
	response  http.ResponseWriter
	errTelem  *appinsights.ExceptionTelemetry
	errStatus int
	err       error
}

func (ctx *context) Request() *http.Request {
	return ctx.request
}

func (ctx *context) Response() http.ResponseWriter {
	return ctx.response
}

func (ctx *context) Render(status int, rr render.Renderer) error {
	telem := appinsights.RequestTelemetryFromContext(ctx)
	if telem != nil {
		telem.SetResponseCode(status)
	}

	return ctx.Context.Render(status, rr)
}

func (ctx *context) Error(status int, err error) error {
	if err != nil {
		ctx.errTelem = appinsights.NewExceptionTelemetry(err)
		ctx.errStatus = status
	}

	result := ctx.Context.Error(status, err)
	ctx.err = result
	return result
}

func (ctx *context) Value(key interface{}) interface{} {
	return ctx.request.Context().Value(key)
}
