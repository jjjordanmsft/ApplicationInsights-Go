package autocollection

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
)

func TestParseRequestHeaders(t *testing.T) {
	type testCase struct {
		headers map[string]string

		rootId     string
		parentId   string
		requestId  string
		properties map[string]string
		context    map[string]string
	}

	cases := []testCase{}

	for i, c := range cases {
		req := &http.Request{
			Header: make(http.Header),
		}

		for k, v := range c.headers {
			req.Header.Set(k, v)
		}

		result := parseCorrelationRequestHeaders(req)
		checkDataContract(t, "headers.parentId", result.parentId, c.parentId)
		checkDataContract(t, "headers.rootId", result.rootId, c.rootId)
		checkDataContract(t, "headers.requestId", result.requestId, c.requestId)

		if len(c.properties) != len(result.properties) {
			t.Errorf("Case %d: expected properties length does not match output properties length", i)
		} else {
			for k, v := range c.properties {
				if rp, ok := result.properties[k]; !ok || rp != v {
					t.Errorf("Case %d: Got wrong property value for key '%s': '%s' expected '%s'.", i, k, rp, v)
				}
			}
		}

		if len(c.context) != len(result.requestContext) {
			t.Errorf("Case %d: expected context length does not match output context length", i)
		} else {
			for k, v := range c.context {
				if rc, ok := result.requestContext[k]; !ok || rc != v {
					t.Errorf("Case %d: Got wrong context value for key '%s': '%s' expected '%s'.", i, k, rc, v)
				}
			}
		}
	}
}

func TestAttachCorrelationRequestHeader(t *testing.T) {
	type testCase struct {
		operation appinsights.Operation
		headers   map[string]string
		id        string
		pattern   bool
	}

	cases := []testCase{}

	for i, c := range cases {
		req := &http.Request{
			Header: make(http.Header),
		}

		id := attachCorrelationRequestHeaders(req, c.operation)
		if id != c.id {
			if c.pattern {
				checkPattern(t, fmt.Sprintf("Case %d ID", i), id, c.id)
			} else {
				checkDataContract(t, fmt.Sprintf("Case %d ID", i), id, c.id)
			}
		}

		if len(c.headers) != len(req.Header) {
			t.Errorf("Case %d: Headers length is unexpected", i)
		}

		for k, v := range c.headers {
			if actual := req.Header.Get(k); actual != v {
				t.Errorf("Case %d: unexpected header %s expected '%s' got '%s'.", i, k, v, actual)
			}
		}
	}
}

func TestParseResponseHeaders(t *testing.T) {
	type testCase struct {
		headers map[string]string

		correlationId  string
		targetRoleName string
	}

	cases := []testCase{}

	for i, c := range cases {
		resp := &http.Response{
			Header: make(http.Header),
		}

		for k, v := range c.headers {
			resp.Header.Set(k, v)
		}

		result := parseCorrelationResponseHeaders(resp)
		checkDataContract(t, fmt.Sprintf("Case %d: headers.correlationId", i), result.correlationId, c.correlationId)
		checkDataContract(t, fmt.Sprintf("Case %d: headers.targetRoleName", i), result.targetRoleName, c.targetRoleName)
	}
}
