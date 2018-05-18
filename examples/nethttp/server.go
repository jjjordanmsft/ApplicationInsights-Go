package main

import (
	"io"
	"log"
	"net/http"
	"os"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights/autocollection"
)

func main() {
	// TelemetryClient setup
	var telemetryClient appinsights.TelemetryClient
	if ikey, ok := os.LookupEnv("IKEY"); ok {
		telemetryClient = appinsights.NewTelemetryClient(ikey)
	} else {
		log.Fatal("Supply an instrumentation key in the IKEY environment variable")
	}

	telemetryClient.Context().CommonProperties["http_framework"] = "net/http"
	autocollection.InstrumentDefaultHTTPClient(telemetryClient, nil)
	appinsights.NewDiagnosticsMessageListener(func(msg string) error {
		log.Println(msg)
		return nil
	})

	// http server setup
	mux := http.NewServeMux()
	middleware := autocollection.NewHTTPMiddleware(telemetryClient, nil)

	mux.HandleFunc("/", IndexHandler)
	mux.HandleFunc("/panic", PanicHandler)
	mux.HandleFunc("/remote", RemoteHandler)
	mux.HandleFunc("/payme", PaymeHandler)

	http.ListenAndServe("127.0.0.1:3000", middleware.Handler(mux))
	<-telemetryClient.Channel().Close()
}

func IndexHandler(rw http.ResponseWriter, r *http.Request) {
	op := appinsights.OperationFromContext(r.Context())
	if op == nil {
		rw.Write([]byte("Couldn't get operation :-("))
	} else {
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
