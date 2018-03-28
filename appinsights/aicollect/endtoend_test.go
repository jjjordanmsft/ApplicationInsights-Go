package aicollect

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights/contracts"
)

type testServer struct {
	*httptest.Server
	sync.Mutex
	requests        []*http.Request
	responseHeaders []http.Header
}

func newTestServer(telemetryClient appinsights.TelemetryClient) *testServer {
	httpClient := NewHTTPClient(nil, telemetryClient)
	result := &testServer{}

	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/e1", func(rw http.ResponseWriter, r *http.Request) {
		result.Lock()
		defer result.Unlock()
		result.requests = append(result.requests, r)
		fmt.Fprintf(rw, "Hello world!")
	})
	serveMux.HandleFunc("/e2", func(rw http.ResponseWriter, r *http.Request) {
		req, _ := http.NewRequest("GET", result.URL+"/e1", nil)
		resp, err := httpClient.Do(req.WithContext(r.Context()))
		defer resp.Body.Close()
		if err != nil {
			rw.WriteHeader(500)
			fmt.Fprintf(rw, "Server error: %s", err.Error())
		} else {
			io.Copy(rw, resp.Body)
		}

		result.Lock()
		defer result.Unlock()
		result.requests = append(result.requests, r)
		result.responseHeaders = append(result.responseHeaders, resp.Header)
	})

	middleware := NewHTTPMiddleware(telemetryClient)
	result.Server = httptest.NewServer(middleware.Handler(serveMux))
	return result
}

type mockChannel struct {
	sync.Mutex
	items []*contracts.Envelope
}

func (ch *mockChannel) EndpointAddress() string {
	return ""
}

func (ch *mockChannel) Send(item *contracts.Envelope) {
	ch.Lock()
	defer ch.Unlock()
	ch.items = append(ch.items, item)
}

func (ch *mockChannel) Stop() {
}

func (ch *mockChannel) Flush() {
}

func (ch *mockChannel) IsThrottled() bool {
	return false
}

func (ch *mockChannel) Close(retryTimeout ...time.Duration) <-chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}

func newMockTelemetryClient(config *appinsights.TelemetryConfiguration, cid string) (appinsights.TelemetryClient, *mockChannel) {
	config.ProfileQueryEndpoint = "<<Invalid URL>>"
	client := appinsights.NewTelemetryClientFromConfig(config)
	client.Channel().Close()
	channel := &mockChannel{}
	cv := reflect.Indirect(reflect.ValueOf(client))
	cf := cv.FieldByName("channel")
	cf = reflect.NewAt(cf.Type(), unsafe.Pointer(cf.UnsafeAddr())).Elem()
	cf.Set(reflect.ValueOf(channel))
	cidf := cv.FieldByName("cid")
	cidf = reflect.NewAt(cidf.Type(), unsafe.Pointer(cidf.UnsafeAddr())).Elem()
	cidf.Set(reflect.ValueOf(cid))
	return client, channel
}

func TestEndToEnd(t *testing.T) {
	tc, ch := newMockTelemetryClient(appinsights.NewTelemetryConfiguration("my_ikey"), "cid-v1:foobar")
	s := newTestServer(tc)
	defer s.Close()

	req, _ := http.NewRequest("GET", s.URL+"/e2", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	defer resp.Body.Close()

	for i, e := range ch.items {
		fmt.Printf("\nItem #%d\nTags:\n", i)
		for k, v := range e.Tags {
			fmt.Printf("\t%s: %s\n", k, v)
		}
		if rt, ok := e.Data.(*contracts.Data).BaseData.(*contracts.RequestData); ok {
			fmt.Printf("Request:\n\tId: %s\n\tSource: %s\n", rt.Id, rt.Source)
		}
		if rd, ok := e.Data.(*contracts.Data).BaseData.(*contracts.RemoteDependencyData); ok {
			fmt.Printf("Dependency:\n\tId: %s\n\tTarget: %s\n", rd.Id, rd.Target)
		}
	}
}
