package gin

import (
	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights/autocollection"
	"github.com/gin-gonic/gin"
)

func Middleware(telemetryClient appinsights.TelemetryClient) gin.HandlerFunc {
	middleware := autocollection.NewHTTPMiddleware(telemetryClient)
	return func(c *gin.Context) {
		request, telem, operation := middleware.BeginRequest(c.Request)
		for k, v := range middleware.GetCorrelationHeaders(c.Request, operation) {
			c.Writer.Header().Set(k, v)
		}

		defer middleware.CompleteRequest(telem, operation)
		c.Request = request
		c.Next()
		telem.SetResponseCode(c.Writer.Status())
	}
}
