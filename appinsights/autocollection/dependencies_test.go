package autocollection

import (
	"errors"
	"math"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
)

const (
	uuidPattern  = `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`
	reqidPattern = `\|` + uuidPattern + `\.[0-9a-f]+_`
	depidPattern = reqidPattern + `[0-9]+\.`

	fakeOperationUuid = "fedcba98-0123-4567-89ab-0123456789ab"
)

func TestDisableDependencyCorrelationHeaders(t *testing.T) {
	cfg := NewDependencyTrackingConfiguration()
	cfg.SendCorrelationHeaders = false
	ch, dummyrt, client, tclient := setupHttpClient(cfg)

	request, _ := http.NewRequest("GET", "http://test-server/foo", nil)
	op := newFakeOperation(tclient, fakeOperationUuid)
	request = request.WithContext(appinsights.WrapContextOperation(request.Context(), op))
	client.Do(request)

	if len(dummyrt.request.Header) > 0 {
		t.Error("Expected no headers added to request")
	}

	// Check telemetry
	dep := ch.getDependency(0)
	if dep == nil {
		t.Fatal("No dependency telemetry was sent")
	}

	checkDataContract(t, "ResultCode", dep.ResultCode, "200")
	checkDataContract(t, "Success", dep.Success, true)
	checkDataContract(t, "Target", dep.Target, "test-server")
	checkDataContract(t, "Type", dep.Type, "Http")
	checkDataContract(t, "Id", dep.Id, "")
}

func TestDependencyCorrelationBlacklist(t *testing.T) {
	cfg := NewDependencyTrackingConfiguration()
	cfg.ExcludeDomains = append(cfg.ExcludeDomains, "*-server")
	ch, dummyrt, client, tclient := setupHttpClient(cfg)

	request, _ := http.NewRequest("GET", "http://test-server/foo", nil)
	op := newFakeOperation(tclient, fakeOperationUuid)
	request = request.WithContext(appinsights.WrapContextOperation(request.Context(), op))
	client.Do(request)

	if len(dummyrt.request.Header) > 0 {
		t.Error("Expected no headers added to request")
	}

	// Check telemetry
	dep := ch.getDependency(0)
	if dep == nil {
		t.Fatal("No dependency telemetry was sent")
	}

	checkDataContract(t, "ResultCode", dep.ResultCode, "200")
	checkDataContract(t, "Success", dep.Success, true)
	checkDataContract(t, "Target", dep.Target, "test-server")
	checkDataContract(t, "Type", dep.Type, "Http")
	checkDataContract(t, "Id", dep.Id, "")
}

func TestDependencyBuiltinCorrelationBlacklist(t *testing.T) {
	ch, dummyrt, client, tclient := setupHttpClient(nil)

	request, _ := http.NewRequest("GET", "http://somewhere.core.windows.net/foo", nil)
	op := newFakeOperation(tclient, fakeOperationUuid)
	request = request.WithContext(appinsights.WrapContextOperation(request.Context(), op))
	client.Do(request)

	if len(dummyrt.request.Header) > 0 {
		t.Error("Expected no headers added to request")
	}

	// Check telemetry
	dep := ch.getDependency(0)
	if dep == nil {
		t.Fatal("No dependency telemetry was sent")
	}

	checkDataContract(t, "ResultCode", dep.ResultCode, "200")
	checkDataContract(t, "Success", dep.Success, true)
	checkDataContract(t, "Target", dep.Target, "somewhere.core.windows.net")
	checkDataContract(t, "Type", dep.Type, "Http")
	checkDataContract(t, "Id", dep.Id, "")
}

func TestDependencyToAI(t *testing.T) {
	channel, dummyrt, client, tc := setupHttpClient(nil)
	tc.Context().Tags.Cloud().SetRole("corr_headers_test")
	dummyrt.response.Header.Set("request-context", "appId=test_the_appId,roleName=test_the_role,otherstuff=foo")
	request, _ := http.NewRequest("GET", "http://test-server/foo", nil)
	op := newFakeOperation(tc, fakeOperationUuid)
	op.Correlation().Properties()["foo"] = "bar"
	request = request.WithContext(appinsights.WrapContextOperation(request.Context(), op))
	client.Do(request)

	headers := dummyrt.request.Header
	checkHeaderPattern(t, headers, "request-id", reqidPattern)
	checkHeaderPattern(t, headers, "x-ms-request-root-id", reqidPattern)
	checkHeader(t, headers, "x-ms-request-id", fakeOperationUuid)
	checkHeader(t, headers, "correlation-context", "foo=bar")
	if headers.Get("request-context") != "appId=test_cid,roleName=corr_headers_test" && headers.Get("request-context") != "roleName=corr_headers_test,appId=test_cid" {
		t.Errorf("Got unexpected value for request-context header: %s", headers.Get("request-context"))
	}

	tags := channel.items[0].Tags
	dep := channel.getDependency(0)
	if dep == nil {
		t.Fatal("Could not get dependency telemetry")
	}

	checkPattern(t, "Tags[ai.operation.parentId]", tags["ai.operation.parentId"], reqidPattern)
	checkPattern(t, "Tags[ai.operation.id]", tags["ai.operation.id"], uuidPattern)
	checkDataContract(t, "Tags[ai.cloud.role]", tags["ai.cloud.role"], "corr_headers_test")

	checkDataContract(t, "Dep.Name", dep.Name, "GET /foo")
	checkPattern(t, "Dep.Id", dep.Id, depidPattern)
	checkDataContract(t, "Dep.ResultCode", dep.ResultCode, "200")
	checkDataContract(t, "Dep.Type", dep.Type, "Http (tracked component)")
	checkDataContract(t, "Dep.Target", dep.Target, "test-server | test_the_appId | roleName:test_the_role")
	checkDataContract(t, "Dep.Success", dep.Success, true)
	checkDataContract(t, "Dep.Data", dep.Data, "http://test-server/foo")

	if !strings.HasPrefix(dep.Id, tags["ai.operation.parentId"]) {
		t.Error("Expected dependency ID to start with request parent id")
	}
}

func TestDependencyToStandard(t *testing.T) {
	channel, dummyrt, client, tc := setupHttpClient(nil)
	tc.Context().Tags.Cloud().SetRole("corr_headers_test")
	request, _ := http.NewRequest("GET", "http://test-server/foo", nil)
	op := newFakeOperation(tc, fakeOperationUuid)
	request = request.WithContext(appinsights.WrapContextOperation(request.Context(), op))
	client.Do(request)

	headers := dummyrt.request.Header
	checkHeaderPattern(t, headers, "request-id", reqidPattern)
	checkHeaderPattern(t, headers, "x-ms-request-root-id", reqidPattern)
	checkHeader(t, headers, "x-ms-request-id", fakeOperationUuid)
	checkHeader(t, headers, "correlation-context", "")
	if headers.Get("request-context") != "appId=test_cid,roleName=corr_headers_test" && headers.Get("request-context") != "roleName=corr_headers_test,appId=test_cid" {
		t.Errorf("Got unexpected value for request-context header: %s", headers.Get("request-context"))
	}

	tags := channel.items[0].Tags
	dep := channel.getDependency(0)
	if dep == nil {
		t.Fatal("Could not get dependency telemetry")
	}

	checkDataContract(t, "Dep.Name", dep.Name, "GET /foo")
	checkPattern(t, "Dep.Id", dep.Id, depidPattern)
	checkDataContract(t, "Dep.ResultCode", dep.ResultCode, "200")
	checkDataContract(t, "Dep.Type", dep.Type, "Http")
	checkDataContract(t, "Dep.Target", dep.Target, "test-server")
	checkDataContract(t, "Dep.Success", dep.Success, true)
	checkDataContract(t, "Dep.Data", dep.Data, "http://test-server/foo")

	if !strings.HasPrefix(dep.Id, tags["ai.operation.parentId"]) {
		t.Error("Expected dependency ID to start with request parent id")
	}

}

func TestDependencyFailedRequest(t *testing.T) {
	channel, dummyrt, client, tc := setupHttpClient(nil)
	request, _ := http.NewRequest("GET", "http://test-server/foo", nil)
	op := newFakeOperation(tc, fakeOperationUuid)
	request = request.WithContext(appinsights.WrapContextOperation(request.Context(), op))
	dummyrt.response.Status = "404 Not Found"
	dummyrt.response.StatusCode = 404
	client.Do(request)

	dep := channel.getDependency(0)
	if dep == nil {
		t.Fatal("Could not get dependency telemetry")
	}

	checkDataContract(t, "Dep.Name", dep.Name, "GET /foo")
	checkPattern(t, "Dep.Id", dep.Id, depidPattern)
	checkDataContract(t, "Dep.ResultCode", dep.ResultCode, "404")
	checkDataContract(t, "Dep.Type", dep.Type, "Http")
	checkDataContract(t, "Dep.Target", dep.Target, "test-server")
	checkDataContract(t, "Dep.Success", dep.Success, false)
	checkDataContract(t, "Dep.Data", dep.Data, "http://test-server/foo")
}

func TestDependencyNoOperation(t *testing.T) {
	channel, dummyrt, client, tc := setupHttpClient(nil)
	tc.Context().Tags.Cloud().SetRole("corr_headers_test")
	dummyrt.response.Header.Set("request-context", "appId=test_the_appId,roleName=test_the_role,otherstuff=foo")
	request, _ := http.NewRequest("GET", "http://test-server/foo", nil)
	client.Do(request)

	headers := dummyrt.request.Header
	checkHeader(t, headers, "request-id", "")
	checkHeader(t, headers, "x-ms-request-root-id", "")
	checkHeader(t, headers, "x-ms-request-id", "")
	checkHeader(t, headers, "correlation-context", "")
	if headers.Get("request-context") != "appId=test_cid,roleName=corr_headers_test" && headers.Get("request-context") != "roleName=corr_headers_test,appId=test_cid" {
		t.Errorf("Got unexpected value for request-context header: %s", headers.Get("request-context"))
	}

	tags := channel.items[0].Tags
	dep := channel.getDependency(0)
	if dep == nil {
		t.Fatal("Could not get dependency telemetry")
	}

	checkDataContract(t, "Tags[ai.operation.parentId]", tags["ai.operation.parentId"], "")
	checkPattern(t, "Tags[ai.operation.id]", tags["ai.operation.id"], uuidPattern)
	checkDataContract(t, "Tags[ai.cloud.role]", tags["ai.cloud.role"], "corr_headers_test")

	checkDataContract(t, "Dep.Name", dep.Name, "GET /foo")
	checkDataContract(t, "Dep.Id", dep.Id, "")
	checkDataContract(t, "Dep.ResultCode", dep.ResultCode, "200")
	checkDataContract(t, "Dep.Type", dep.Type, "Http (tracked component)")
	checkDataContract(t, "Dep.Target", dep.Target, "test-server | test_the_appId | roleName:test_the_role")
	checkDataContract(t, "Dep.Success", dep.Success, true)
	checkDataContract(t, "Dep.Data", dep.Data, "http://test-server/foo")
}

func TestIgnoreContext(t *testing.T) {
	channel, dummyrt, client, tc := setupHttpClient(nil)
	tc.Context().Tags.Cloud().SetRole("corr_headers_test")
	dummyrt.response.Header.Set("request-context", "appId=test_the_appId,roleName=test_the_role,otherstuff=foo")
	request, _ := http.NewRequest("GET", "http://test-server/foo", nil)
	op := newFakeOperation(tc, fakeOperationUuid)
	request = request.WithContext(appinsights.WrapContextOperation(request.Context(), op))
	request = appinsights.MarkRequestIgnore(request)
	client.Do(request)

	if len(dummyrt.request.Header) > 0 {
		t.Error("Expected no headers added to request")
	}

	if len(channel.items) > 0 {
		t.Error("Expected no dependencies to be tracked")
	}
}

func TestErrorResponse(t *testing.T) {
	channel, dummyrt, client, tc := setupHttpClient(nil)
	dummyrt.err = errors.New("Failed to send fake request")
	request, _ := http.NewRequest("GET", "http://test-server/foo", nil)
	op := newFakeOperation(tc, fakeOperationUuid)
	request = request.WithContext(appinsights.WrapContextOperation(request.Context(), op))
	_, err := client.Do(request)

	if err.Error() != "Get http://test-server/foo: Failed to send fake request" {
		t.Errorf("Expected roundtripper to return error; got: %v\n", err)
	}

	dep := channel.getDependency(0)
	if dep == nil {
		t.Fatal("Could not get dependency telemetry")
	}

	checkDataContract(t, "Dep.Name", dep.Name, "GET /foo")
	checkPattern(t, "Dep.Id", dep.Id, depidPattern)
	checkDataContract(t, "Dep.ResultCode", dep.ResultCode, "0")
	checkDataContract(t, "Dep.Type", dep.Type, "Http")
	checkDataContract(t, "Dep.Target", dep.Target, "test-server")
	checkDataContract(t, "Dep.Success", dep.Success, false)
	checkDataContract(t, "Dep.Data", dep.Data, "http://test-server/foo")
}

// Test helpers -----

func setupHttpClient(cfg *DependencyTrackingConfiguration) (*mockChannel, *dummyRoundTripper, *http.Client, appinsights.TelemetryClient) {
	tc, channel := newMockTelemetryClient(appinsights.NewTelemetryConfiguration("test_ikey"), "test_cid")
	dummyrt := newDummyRoundTripper()
	innerClient := *http.DefaultClient
	innerClient.Transport = dummyrt
	client := NewHTTPClient(&innerClient, tc, cfg)

	return channel, dummyrt, client, tc
}

type dummyRoundTripper struct {
	request  *http.Request
	response *http.Response
	err      error
}

func newDummyRoundTripper() *dummyRoundTripper {
	return &dummyRoundTripper{
		request: nil,
		response: &http.Response{
			Status:        "200 OK",
			StatusCode:    200,
			Proto:         "HTTP/1.1",
			ProtoMajor:    1,
			ProtoMinor:    1,
			Header:        make(http.Header),
			Body:          nil,
			ContentLength: -1,
			Close:         true,
			Uncompressed:  false,
			Trailer:       make(http.Header),
			Request:       nil,
			TLS:           nil,
		},
		err: nil,
	}
}

func (rt *dummyRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if rt.err != nil {
		return nil, rt.err
	}

	rt.response.Request = r
	rt.request = r
	return rt.response, nil
}

func newFakeOperation(client appinsights.TelemetryClient, rootId string) appinsights.Operation {
	oid := appinsights.OperationId(rootId)
	cc := appinsights.NewCorrelationContext(oid, oid.GenerateRequestId(), "GET /", nil)
	return appinsights.NewOperation(client, cc)
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
