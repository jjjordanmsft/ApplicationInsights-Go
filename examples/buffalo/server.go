package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights/autocollection"
	aibuffalo "github.com/Microsoft/ApplicationInsights-Go/appinsights/autocollection/buffalo"
	"github.com/gobuffalo/buffalo"
	"github.com/gobuffalo/buffalo/render"
)

var telemetryClient appinsights.TelemetryClient

func init() {
	// TelemetryClient setup
	if ikey, ok := os.LookupEnv("IKEY"); ok {
		telemetryClient = appinsights.NewTelemetryClient(ikey)
	} else {
		log.Fatal("Supply an instrumentation key in the IKEY environment variable")
	}

	telemetryClient.Context().CommonProperties["http_framework"] = "buffalo"
	autocollection.InstrumentDefaultHTTPClient(telemetryClient, nil)
	appinsights.NewDiagnosticsMessageListener(func(msg string) error {
		log.Println(msg)
		return nil
	})
}

func main() {
	app := App()
	err := app.Serve()
	<-telemetryClient.Channel().Close()

	if err != nil {
		log.Fatal(err)
	}
}

func App() *buffalo.App {
	app := buffalo.New(buffalo.Options{})
	app.Use(aibuffalo.BuffaloAdapter(autocollection.NewHTTPMiddleware(telemetryClient, nil)))

	app.GET("/", IndexHandler)
	app.GET("/panic", PanicHandler)
	app.GET("/remote", RemoteHandler)
	app.GET("/payme", PaymeHandler)

	// FIXME: unhandled 404's are not detected.

	return app
}

func IndexHandler(c buffalo.Context) error {
	op := appinsights.OperationFromContext(c)
	if op == nil {
		return c.Render(200, render.String("Couldn't get operation :-("))
	}
	op.TrackTrace("Hello world!", appinsights.Information)
	return c.Render(200, render.String("Hello world!"))
}

func PanicHandler(c buffalo.Context) error {
	panic("Ouch")
}

func RemoteHandler(c buffalo.Context) error {
	req, err := http.NewRequest("GET", "https://httpbin.org/headers", nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req.WithContext(c))
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return c.Render(200, render.String(string(body)))
}

func PaymeHandler(c buffalo.Context) error {
	return c.Render(402, render.String("Payment required"))
}
