package autocollection

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
)

func TestEndToEnd(t *testing.T) {
	var s *testServer
	var httpClient *http.Client
	var requests []*http.Request
	var responseHeaders []http.Header

	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/e1", func(rw http.ResponseWriter, r *http.Request) {
		s.Lock()
		defer s.Unlock()
		requests = append(requests, r)
		fmt.Fprintf(rw, "Hello world!")
	})
	serveMux.HandleFunc("/e2", func(rw http.ResponseWriter, r *http.Request) {
		req, _ := s.newRequest("GET", "/e1", nil)
		resp, err := httpClient.Do(req.WithContext(r.Context()))
		if err != nil {
			rw.WriteHeader(500)
			fmt.Fprintf(rw, "Server error: %s", err.Error())
			return
		} else {
			defer resp.Body.Close()
			io.Copy(rw, resp.Body)
		}

		s.Lock()
		defer s.Unlock()
		requests = append(requests, r)
		responseHeaders = append(responseHeaders, resp.Header)
	})

	tc, ch := newMockTelemetryClient(appinsights.NewTelemetryConfiguration("my_ikey"), "cid-v1:foobar")
	httpClient = NewHTTPClient(nil, tc, nil)
	s = newTestServer(serveMux, tc, nil)
	defer s.Close()

	req, _ := s.newRequest("GET", "/e2", nil)
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
		if rt := ch.getRequest(i); rt != nil {
			fmt.Printf("Request:\n\tId: %s\n\tSource: %s\n", rt.Id, rt.Source)
		}
		if rd := ch.getDependency(i); rd != nil {
			fmt.Printf("Dependency:\n\tId: %s\n\tTarget: %s\n", rd.Id, rd.Target)
		}
	}
}
