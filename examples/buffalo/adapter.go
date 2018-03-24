package main

import (
	"net/http"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights/aicollect"
	"github.com/gobuffalo/buffalo"
)

// Middleware is an adapter so that
func Middleware(telemetryClient appinsights.TelemetryClient) buffalo.MiddlewareFunc {
	middleware := aicollect.NewHTTPMiddleware(telemetryClient)
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

func (ctx *context) Value(key interface{}) interface{} {
	return ctx.request.Context().Value(key)
}
