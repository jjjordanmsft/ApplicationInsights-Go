package main

import (
	"flag"
	"fmt"
	"os"
	"time"
	
	"github.com/jjjordanmsft/ApplicationInsights-Go/appinsights"
	"github.com/satori/go.uuid"
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		fmt.Println("Please specify an ikey on the commandline")
		os.Exit(2)
	}
	
	client := appinsights.NewTelemetryClient(args[0])
	appinsights.NewDiagnosticsMessageListener(func(msg string) error {
		fmt.Printf("[%s] %s\n", time.Now().Format(time.UnixDate), msg)
		return nil
	})
	
	defer func() {
		<-client.Channel().Close()
	}()
	
	// Trace
	client.TrackTrace("Client.TrackTrace", appinsights.Warning)
	tr := appinsights.NewTraceTelemetry("Track_Trace", appinsights.Error)
	tr.Properties["prop"] = "value"
	client.Track(tr)
	
	// Event
	client.TrackEvent("Client.TrackEvent")
	ev := appinsights.NewEventTelemetry("Track_Event")
	ev.Properties["prop"] = "value"
	ev.Measurements["measure"] = 909.1
	client.Track(ev)
	
	// Metric
	client.TrackMetric("Client.TrackMetric", 900.1)
	mt := appinsights.NewMetricTelemetry("Track_Metric", 909.1)
	mt.Properties["prop"] = "value"
	client.Track(mt)
	
	// Aggregate
	agg := appinsights.NewAggregateMetricTelemetry("Track_AggregateMetric")
	agg.AddSampledData([]float64{909.1, 100.2, 53.4, 932.7, 292.8})
	agg.Properties["prop"] = "value"
	client.Track(agg)
	
	// Request
	client.TrackRequest("GET", "https://testuri.org/client/track", 4*time.Second, "200")
	req := appinsights.NewRequestTelemetry("GET", "https://testuri.org/track", 5*time.Second, "200")
	req.Properties["prop"] = "value"
	req.Measurements["measure"] = 909.1
	req.Source = "127.0.0.1"
	client.Track(req)
	
	// Remote dependency
	rem1 := appinsights.NewRemoteDependencyTelemetry("Get-bing", "HTTP", "http://bing.com", true)
	client.Track(rem1)
	
	rem2 := appinsights.NewRemoteDependencyTelemetry("Get-google", "HTTP", "http://google.com", true)
	rem2.ResultCode = "200"
	rem2.Duration = time.Minute
	rem2.Data = "searched"
	rem2.Id = uuid.Must(uuid.NewV4()).String()
	client.Track(rem2)
	
	// Availability
	client.TrackAvailability("web-test-1", time.Second, true)
	
	av := appinsights.NewAvailabilityTelemetry("web-test-2", time.Second, false)
	av.RunLocation = "The moon"
	av.Message = "Message"
	client.Track(av)
	
	// Page view
	pv := appinsights.NewPageViewTelemetry("Home", "https://mysite.appinsights.io/")
	client.Track(pv)
	
	pv2 := appinsights.NewPageViewTelemetry("the-page", "https://mysite.appinsights.io/")
	pv2.Duration = time.Minute
	client.Track(pv2)
	
	// Exception
	exception(client)
}

func exception(client appinsights.TelemetryClient) {
	defer func() {
		if r := recover(); r != nil {
			client.TrackException(r)
		}
	}()
	
	exc1()
}

func exc1() {
	exc2()
}

func exc2() {
	exc3()
}

func exc3() {
	exc4()
}

func exc4() {
	exc5()
}

func exc5() {
	panic(fmt.Errorf("Panic!"))
}
