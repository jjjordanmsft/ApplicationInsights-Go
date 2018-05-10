package appinsights

import (
	"sync"
	"testing"
	"time"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights/contracts"
)

func BenchmarkClientBurstPerformance(b *testing.B) {
	client := NewTelemetryClient("")
	client.(*telemetryClient).channel.(*InMemoryChannel).transmitter = &nullTransmitter{}

	for i := 0; i < b.N; i++ {
		client.TrackTrace("A message", Information)
	}

	<-client.Channel().Close(time.Minute)
}

func TestClientProperties(t *testing.T) {
	mockCidLookup(map[string]string{test_ikey: "test_correlation_id"})
	defer resetCidLookup()

	client := NewTelemetryClient(test_ikey)
	defer client.Channel().Close()

	if _, ok := client.Channel().(*InMemoryChannel); !ok {
		t.Error("Client's Channel() is not InMemoryChannel")
	}

	if ikey := client.InstrumentationKey(); ikey != test_ikey {
		t.Error("Client's InstrumentationKey is not expected")
	}

	if ikey := client.Context().InstrumentationKey(); ikey != test_ikey {
		t.Error("Context's InstrumentationKey is not expected")
	}

	if client.Context() == nil {
		t.Error("Client.Context == nil")
	}

	if client.IsEnabled() == false {
		t.Error("Client.IsEnabled == false")
	}

	client.SetIsEnabled(false)
	if client.IsEnabled() == true {
		t.Error("Client.SetIsEnabled had no effect")
	}

	if client.Channel().EndpointAddress() != "https://dc.services.visualstudio.com/v2/track" {
		t.Error("Client.Channel.EndpointAddress was incorrect")
	}

	if client.CorrelationId() != "cid-v1:test_correlation_id" {
		t.Error("Client.CorrelationId was incorrect")
	}

	if client.GetSamplingPercentage() != 100.0 {
		t.Error("Default sampling percentage should be 100")
	}

	client.SetSamplingPercentage(34.0)
	if client.GetSamplingPercentage() != 34.0 {
		t.Error("Sampling percentage should be modified by SetSamplingPercentage")
	}
}

func TestEndToEnd(t *testing.T) {
	mockClock(time.Unix(1511001321, 0))
	defer resetClock()
	defer resetCidLookup()
	xmit, server := newTestClientServer()
	defer server.Close()

	config := NewTelemetryConfiguration(test_ikey)
	config.EndpointUrl = xmit.(*httpTransmitter).endpoint
	cidServer := useCidServer(config, map[string]string{test_ikey: "test_cid"})
	defer cidServer.Close()
	client := NewTelemetryClientFromConfig(config)
	defer client.Channel().Close()

	// Ensure cid lookup takes place
	cidServer.waitForRequest(t)

	// Track directly off the client
	client.TrackEvent("client-event")
	client.TrackMetric("client-metric", 44.0)
	client.TrackTrace("client-trace", Information)
	client.TrackRequest("GET", "www.testurl.org", time.Minute, "404")

	// NOTE: A lot of this is covered elsewhere, so we won't duplicate
	// *too* much.

	// Set up server response
	server.responseData = []byte(`{"itemsReceived":4, "itemsAccepted":4, "errors":[]}`)
	server.responseHeaders["Content-type"] = "application/json"

	// Wait for automatic transmit -- get the request
	slowTick(11)
	req := server.waitForRequest(t)

	body, err := req.getPayload()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check out payload
	j := parsePayload(t, body)
	if len(j) != 4 {
		t.Fatal("Unexpected event count")
	}

	j[0].assertPath(t, "iKey", test_ikey)
	j[0].assertPath(t, "name", "Microsoft.ApplicationInsights.01234567000089abcdef000000000000.Event")
	j[0].assertPath(t, "time", "2017-11-18T10:35:21Z")

	j[1].assertPath(t, "iKey", test_ikey)
	j[1].assertPath(t, "name", "Microsoft.ApplicationInsights.01234567000089abcdef000000000000.Metric")
	j[1].assertPath(t, "time", "2017-11-18T10:35:21Z")

	j[2].assertPath(t, "iKey", test_ikey)
	j[2].assertPath(t, "name", "Microsoft.ApplicationInsights.01234567000089abcdef000000000000.Message")
	j[2].assertPath(t, "time", "2017-11-18T10:35:21Z")

	j[3].assertPath(t, "iKey", test_ikey)
	j[3].assertPath(t, "name", "Microsoft.ApplicationInsights.01234567000089abcdef000000000000.Request")
	j[3].assertPath(t, "time", "2017-11-18T10:34:21Z")
}

func TestSampling(t *testing.T) {
	mockCidLookup(nil)
	defer resetCidLookup()
	channel, client := newMockChannelClient(nil)

	// Set sampling to 60%, 1000 events, 0.01 tolerance
	pct := 60.0
	count := 1000
	tolerance := 0.1
	expected := (pct / 100.0) * float64(count)

	// Send events
	client.SetSamplingPercentage(pct)
	for i := 0; i < count; i++ {
		client.TrackEvent("Sample test")
	}

	// Check count
	if float64(len(channel.items)) < (expected*(1-tolerance)) || float64(len(channel.items)) > (expected*(1+tolerance)) {
		t.Errorf("Sent %d messages, and received %d which is outside of the expected tolerance of %f", count, len(channel.items), tolerance)
	}

	// Make sure sampling percentage is set on each message.
	for _, item := range channel.items {
		if item.SampleRate != pct {
			t.Errorf("Unexpected sample rate value: %v", item.SampleRate)
		}
	}
}

// Test helpers -----

func newMockChannelClient(config *TelemetryConfiguration) (*mockChannel, TelemetryClient) {
	if config == nil {
		config = NewTelemetryConfiguration(test_ikey)
	}

	channel := &mockChannel{}
	tc := NewTelemetryClientFromConfig(config).(*telemetryClient)
	tc.channel.Close()
	tc.channel = channel
	return channel, tc
}

type mockChannel struct {
	sync.Mutex
	items telemetryBufferItems
}

func (ch *mockChannel) Send(item *contracts.Envelope) {
	ch.Lock()
	defer ch.Unlock()
	ch.items = append(ch.items, item)
}

func (ch *mockChannel) EndpointAddress() string {
	return ""
}

func (ch *mockChannel) Stop() {
}

func (ch *mockChannel) Flush() {
}

func (ch *mockChannel) IsThrottled() bool {
	return false
}

func (ch *mockChannel) Close(retryTimeout ...time.Duration) <-chan struct{} {
	result := make(chan struct{})
	close(result)
	return result
}
