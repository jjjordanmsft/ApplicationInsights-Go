package main

import (
	"io"
	"log"
	"net/http"
	"os"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights/aicollect"
	"github.com/go-martini/martini"
)

func main() {
	// TelemetryClient setup
	var telemetryClient appinsights.TelemetryClient
	if ikey, ok := os.LookupEnv("IKEY"); ok {
		telemetryClient = appinsights.NewTelemetryClient(ikey)
	} else {
		log.Fatal("Supply an instrumentation key in the IKEY environment variable")
	}

	telemetryClient.Context().CommonProperties["http_framework"] = "martini"
	aicollect.InstrumentDefaultHTTPClient(telemetryClient)
	appinsights.NewDiagnosticsMessageListener(func(msg string) error {
		log.Println(msg)
		return nil
	})

	// http server setup
	m := martini.Classic()
	m.Use(Middleware(telemetryClient))
	m.Get("/", IndexHandler)
	m.Get("/panic", PanicHandler)
	m.Get("/remote", RemoteHandler)
	m.Get("/payme", PaymeHandler)

	// TODO: Graceful shutdown doesn't work.
	defer func() { <-telemetryClient.Channel().Close() }()
	m.Run()
}

func IndexHandler(rw http.ResponseWriter, r *http.Request) {
	op := appinsights.OperationFromContext(r.Context())
	if op == nil {
		rw.Write([]byte("Couldn't get operation :-("))
	} else {
		op.TrackTrace("Hello world!", appinsights.Information)
		rw.Write([]byte("Hello world!"))
	}
}

func PanicHandler(rw http.ResponseWriter, r *http.Request) {
	panic("Ouch")
}

func RemoteHandler(rw http.ResponseWriter, r *http.Request) {
	req, err := http.NewRequest("GET", "https://httpbin.org/headers", nil)
	if err != nil {
		panic(err)
	}

	resp, err := http.DefaultClient.Do(req.WithContext(r.Context()))
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()
	io.Copy(rw, resp.Body)
}

func PaymeHandler(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(402)
	rw.Write([]byte("Payment required"))
}
