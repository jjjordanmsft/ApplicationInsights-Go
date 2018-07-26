package autocollection

import (
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
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
}

func newTestServer(handler http.Handler, telemetryClient appinsights.TelemetryClient, config *HTTPMiddlewareConfiguration) *testServer {
	result := &testServer{}
	middleware := NewHTTPMiddleware(telemetryClient, config)
	result.Server = httptest.NewServer(middleware.Handler(handler))
	return result
}

func (ts *testServer) newRequest(method, path string, data io.Reader) (*http.Request, error) {
	uri, err := url.Parse(ts.URL)
	if err != nil {
		return nil, err
	}

	uri.Path = path
	return http.NewRequest(method, uri.String(), data)
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

func (ch *mockChannel) getRequest(item int) *contracts.RequestData {
	if rt, ok := ch.items[item].Data.(*contracts.Data).BaseData.(*contracts.RequestData); ok {
		return rt
	} else {
		return nil
	}
}

func (ch *mockChannel) getDependency(item int) *contracts.RemoteDependencyData {
	if rd, ok := ch.items[item].Data.(*contracts.Data).BaseData.(*contracts.RemoteDependencyData); ok {
		return rd
	} else {
		return nil
	}
}

func (ch *mockChannel) getData(item int) interface{} {
	return ch.items[item].Data.(*contracts.Data).BaseData
}

func newMockTelemetryClient(config *appinsights.TelemetryConfiguration, cid string) (appinsights.TelemetryClient, *mockChannel) {
	config.ProfileQueryEndpoint = "<<Invalid URL>>"
	client := appinsights.NewTelemetryClientFromConfig(config)
	client.Context().Tags.Cloud().SetRole("endtoend")
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

const float_precision = 1e-4

func checkDataContract(t *testing.T, property string, actual, expected interface{}) {
	if x, ok := actual.(float64); ok {
		if y, ok := expected.(float64); ok {
			if math.Abs(x-y) > float_precision {
				t.Errorf("Float property %s mismatched; got %f, want %f.\n", property, actual, expected)
			}

			return
		}
	}

	if x, ok := actual.(string); ok {
		if y, ok := expected.(string); ok {
			if x != y {
				t.Errorf("Property %s mismatched; got %s, want %s.\n", property, x, y)
			}

			return
		}
	}

	if actual != expected {
		t.Errorf("Property %s mismatched; got %v, want %v.\n", property, actual, expected)
	}
}

func checkPattern(t *testing.T, property, actual, expected string) {
	if m, err := regexp.MatchString(expected, actual); !m || err != nil {
		t.Errorf("Property %s mismatched; got %s, want %s.\n", property, actual, expected)
	}
}

func checkHeader(t *testing.T, header http.Header, key string, expected interface{}) {
	checkDataContract(t, "Header["+key+"]", header.Get(key), expected)
}

func checkHeaderPattern(t *testing.T, header http.Header, key, expected string) {
	checkPattern(t, "Header["+key+"]", header.Get(key), expected)
}
