package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights/autocollection"
	aigin "github.com/Microsoft/ApplicationInsights-Go/appinsights/autocollection/gin"
	"github.com/gin-gonic/gin"
)

func main() {
	// TelemetryClient setup
	var telemetryClient appinsights.TelemetryClient
	if ikey, ok := os.LookupEnv("IKEY"); ok {
		telemetryClient = appinsights.NewTelemetryClient(ikey)
	} else {
		log.Fatal("Supply an instrumentation key in the IKEY environment variable")
	}

	telemetryClient.Context().CommonProperties["http_framework"] = "gin"
	autocollection.InstrumentDefaultHTTPClient(telemetryClient)
	appinsights.NewDiagnosticsMessageListener(func(msg string) error {
		log.Println(msg)
		return nil
	})

	// Gin setup
	router := gin.Default()
	router.Use(aigin.Middleware(telemetryClient))

	router.GET("/", IndexHandler)
	router.GET("/panic", PanicHandler)
	router.GET("/remote", RemoteHandler)
	router.GET("/payme", PaymeHandler)

	// TODO: Implement graceful shutdown as described in the gin docs.
	router.Run()
}

func IndexHandler(c *gin.Context) {
	op := appinsights.OperationFromContext(c.Request.Context())
	if op == nil {
		c.String(200, "Couldn't get operation")
	} else {
		op.TrackTrace("Hello world!", appinsights.Information)
		c.String(200, "Hello world!")
	}
}

func PanicHandler(c *gin.Context) {
	panic("Ouch")
}

func RemoteHandler(c *gin.Context) {
	req, err := http.NewRequest("GET", "https://httpbin.org/headers", nil)
	if err != nil {
		panic(err)
	}

	resp, err := http.DefaultClient.Do(req.WithContext(c.Request.Context()))
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()
	d, _ := ioutil.ReadAll(resp.Body)
	c.String(200, string(d))
}

func PaymeHandler(c *gin.Context) {
	c.String(402, "Payment required")
}
