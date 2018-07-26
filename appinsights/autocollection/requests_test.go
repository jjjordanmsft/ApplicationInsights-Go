package autocollection

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights/contracts"
)

func TestXForwardedFor(t *testing.T) {
	tc, ch := newMockTelemetryClient(appinsights.NewTelemetryConfiguration("ikey"), "cid-v1:hello")
	s := newTestServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Write([]byte("OK"))
	}), tc, nil)
	defer s.Close()

	sendWithXForwardedFor := func(xff string) (*contracts.RemoteDependencyData, map[string]string) {
		req, err := s.newRequest("GET", "/", nil)
		if err != nil {
			t.Fatal(err.Error())
		}

		req.Header.Add("x-forwarded-for", xff)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err.Error())
		}

		defer resp.Body.Close()
		ioutil.ReadAll(resp.Body)
		result := ch.getDependency(0)
		tags := ch.items[0].Tags
		ch.items = ch.items[:0]
		return result, tags
	}

	// simple IPv4
	_, tags1 := sendWithXForwardedFor("123.45.67.89")
	checkDataContract(t, "ai.location.ip", tags1["ai.location.ip"], "123.45.67.89")
	_, tags2 := sendWithXForwardedFor("98.76.54.32, 123.45.67.89")
	checkDataContract(t, "ai.location.ip", tags2["ai.location.ip"], "98.76.54.32")
}

func TestRequestContext(t *testing.T) {
	tc, ch := newMockTelemetryClient(appinsights.NewTelemetryConfiguration("ikey"), "cid-v1:hello")
	s := newTestServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		op := appinsights.OperationFromContext(r.Context())
		if op == nil {
			t.Fatal("Can't get operation")
		}

		req := appinsights.RequestTelemetryFromContext(r.Context())
		if req == nil {
			t.Fatal("Can't get request telemetry")
		}

		req.Properties["Added"] = "Yep"
		op.TrackTrace("Test trace", appinsights.Warning)

		rw.Write([]byte("OK"))
	}), tc, nil)
	defer s.Close()

	req, err := s.newRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err.Error())
	}

	defer resp.Body.Close()
	ioutil.ReadAll(resp.Body)

	// Check the trace
	traceEnvelope := ch.items[0]
	traceItem := traceEnvelope.Data.(*contracts.Data).BaseData.(*contracts.MessageData)

	checkDataContract(t, "trace.envelope.name", traceEnvelope.Name, "Microsoft.ApplicationInsights.ikey.Message")
	checkDataContract(t, "trace.Message", traceItem.Message, "Test trace")
	checkDataContract(t, "trace.SeverityLevel", int(traceItem.SeverityLevel), 2)
	checkPattern(t, "trace.envelope.tags.OperationId", traceEnvelope.Tags[contracts.OperationId], uuidPattern)
	checkPattern(t, "trace.envelope.tags.OperationParentId", traceEnvelope.Tags[contracts.OperationParentId], `\|`+uuidPattern+`\.`)

	// Check the request
	reqItem := ch.getRequest(1)
	reqEnvelope := ch.items[1]

	checkDataContract(t, "request.name", reqItem.Name, "GET /")
	checkDataContract(t, "request.responseCode", reqItem.ResponseCode, "200")
	checkDataContract(t, "request.success", reqItem.Success, true)
	checkDataContract(t, "request.url", reqItem.Url, "/")
	checkDataContract(t, "request.properties[Added]", reqItem.Properties["Added"], "Yep")
	checkDataContract(t, "request.envelope.tags.OperationId", reqEnvelope.Tags[contracts.OperationId], traceEnvelope.Tags[contracts.OperationId])
	checkDataContract(t, "request.envelope.tags.OperationParentId", reqEnvelope.Tags[contracts.OperationParentId], "")
	checkDataContract(t, "request.id", reqItem.Id, traceEnvelope.Tags[contracts.OperationParentId])
	checkPattern(t, "request.id", reqItem.Id, `\|`+uuidPattern+`\.`)
}

func TestRequestStatusCode(t *testing.T) {
	tc, ch := newMockTelemetryClient(appinsights.NewTelemetryConfiguration("ikey"), "cid-v1:hello")
	s := newTestServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(419)
		rw.Write([]byte("Throttled"))
	}), tc, nil)
	defer s.Close()

	req, err := s.newRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err.Error())
	}

	defer resp.Body.Close()
	ioutil.ReadAll(resp.Body)

	// Check the request
	reqItem := ch.getRequest(0)

	checkDataContract(t, "request.responseCode", reqItem.ResponseCode, "419")
	checkDataContract(t, "request.success", reqItem.Success, false)
	checkDataContract(t, "request.url", reqItem.Url, "/")
}

func TestRequestPanic(t *testing.T) {
	print(" >>>>> A panic here is OK <<<<<\n")
	tc, ch := newMockTelemetryClient(appinsights.NewTelemetryConfiguration("ikey"), "cid-v1:hello")
	s := newTestServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		panic("Ouch")
	}), tc, nil)
	defer s.Close()

	req, err := s.newRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// This probably causes an error.
	} else {
		defer resp.Body.Close()
		ioutil.ReadAll(resp.Body)
	}

	// Check the exception
	excEnvelope := ch.items[0]
	excData := excEnvelope.Data.(*contracts.Data).BaseData.(*contracts.ExceptionData)
	excDetails := excData.Exceptions[0]

	checkDataContract(t, "exception.envelope.name", excEnvelope.Name, "Microsoft.ApplicationInsights.ikey.Exception")
	checkDataContract(t, "exception.Message", excDetails.Message, "Ouch")
	checkDataContract(t, "exception.SeverityLevel", int(excData.SeverityLevel), 3)
	checkPattern(t, "exception.envelope.tags.OperationId", excEnvelope.Tags[contracts.OperationId], uuidPattern)
	checkPattern(t, "exception.envelope.tags.OperationParentId", excEnvelope.Tags[contracts.OperationParentId], `\|`+uuidPattern+`\.`)

	// Check the request
	reqItem := ch.getRequest(1)
	reqEnvelope := ch.items[1]

	checkDataContract(t, "request.name", reqItem.Name, "GET /")
	checkDataContract(t, "request.responseCode", reqItem.ResponseCode, "500")
	checkDataContract(t, "request.success", reqItem.Success, false)
	checkDataContract(t, "request.url", reqItem.Url, "/")
	checkDataContract(t, "request.envelope.tags.OperationId", reqEnvelope.Tags[contracts.OperationId], excEnvelope.Tags[contracts.OperationId])
	checkDataContract(t, "request.envelope.tags.OperationParentId", reqEnvelope.Tags[contracts.OperationParentId], "")
	checkDataContract(t, "request.id", reqItem.Id, excEnvelope.Tags[contracts.OperationParentId])
	checkPattern(t, "request.id", reqItem.Id, `\|`+uuidPattern+`\.`)
}

type testPusher struct {
	called bool
}

func (p *testPusher) Push(target string, opts *http.PushOptions) error {
	p.called = true
	return nil
}

func TestRequestPusher(t *testing.T) {
	tc, _ := newMockTelemetryClient(appinsights.NewTelemetryConfiguration("ikey"), "cid-v1:hello")
	middleware := NewHTTPMiddleware(tc, NewHTTPMiddlewareConfiguration())

	req := httptest.NewRequest("GET", "/", nil)
	recorder := httptest.NewRecorder()
	testPusher := &testPusher{}
	rw := &responseWriterPusher{
		ResponseWriter: recorder,
		Pusher:         testPusher,
	}

	middleware.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if pusher, ok := rw.(http.Pusher); ok {
			pusher.Push("", nil)
		}
	}).ServeHTTP(rw, req)

	if !testPusher.called {
		t.Error("Middleware did not propagate http.Pusher")
	}
}

func TestGetIP(t *testing.T) {
	type testCase struct {
		remoteAddr string
		xff        string
		result     string
	}

	cases := []testCase{
		testCase{"", "", ""},
		testCase{"127.0.0.1", "10.0.0.1", "10.0.0.1"},
		testCase{"10.0.0.1", "", "10.0.0.1"},
		testCase{"10.0.0.1", "NOT-AN-IP", "10.0.0.1"},
		testCase{"NOT_AN_IP", "10.0.0.1", "10.0.0.1"},
		testCase{"NOT_AN_IP", "NOT_AN_IP", ""},
		testCase{"[2001:db8::1]", "", "2001:db8::1"},
		testCase{"[2001:db8::1]:32707", "", "2001:db8::1"},
		testCase{"2001:db8::1", "", "2001:db8::1"},
		testCase{"FE80:0000:0000:0000:0202:B3FF:FE1E:8329", "", "FE80:0000:0000:0000:0202:B3FF:FE1E:8329"},
		testCase{"FE80::0202:B3FF:FE1E:8329", "", "FE80::0202:B3FF:FE1E:8329"},
		testCase{"::1", "", "::1"},
		testCase{"[::1]", "", "::1"},
		testCase{"2001:db8:0:1", "", ""},
	}

	for i, c := range cases {
		req := http.Request{
			RemoteAddr: c.remoteAddr,
			Header:     make(http.Header),
		}

		if c.xff != "" {
			req.Header.Set("x-forwarded-for", c.xff)
		}

		result := getIP(&req)
		if result != c.result {
			t.Errorf("Case %d failed. Expected '%s' got '%s'.", i, c.result, result)
		}
	}
}
