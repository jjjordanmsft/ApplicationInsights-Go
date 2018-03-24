package main

import (
	"net/http"
	"strconv"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights/aicollect"
	"github.com/gin-gonic/gin"
)

func Middleware(telemetryClient appinsights.TelemetryClient) gin.HandlerFunc {
	middleware := aicollect.NewHTTPMiddleware(telemetryClient)
	return func(c *gin.Context) {
		middleware.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			c.Request = r
			c.Next()

			// Replace code - since Context.Status() goes around it
			code := c.Writer.Status()
			telem := appinsights.RequestTelemetryFromContext(c.Request.Context())
			telem.ResponseCode = strconv.Itoa(code)
			telem.Success = code < 400 || code == 401
		})(c.Writer, c.Request)
	}
}
