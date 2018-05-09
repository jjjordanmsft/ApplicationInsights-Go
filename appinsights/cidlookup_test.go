package appinsights

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// Tests that the correlation ID gets propagated to the telemetryClient from the correlation manager.
func TestCidToClient(t *testing.T) {
	mockCidLookup(map[string]string{test_ikey: "test_cid"})
	defer resetCidLookup()

	client1 := NewTelemetryClient(test_ikey)
	defer client1.Channel().Close()
	if client1.CorrelationId() != "cid-v1:test_cid" {
		t.Error("Correlation ID lookup didn't propagate to client: " + client1.CorrelationId())
	}

	client2 := NewTelemetryClient("not_an_ikey")
	defer client2.Channel().Close()
	if client2.CorrelationId() != "" {
		t.Error("Failed correlation ID lookup produced incorrect result on client: " + client2.CorrelationId())
	}
}

// Tests a simple, successful CID lookup against a mock server.
func TestCidSuccessfulRequest(t *testing.T) {
	mockClock()
	defer resetClock()
	resetCidLookup()

	server := newCidServer(map[string]string{test_ikey: "test_cid", "abc": "xyz"})
	defer server.Close()

	result := exhaustCidRetry(queryCid(server, test_ikey))
	if result == nil {
		t.Fatal("Query was not answered within retry period")
	}

	if result.err != nil {
		t.Error("Error querying CID: " + result.err.Error())
	}
	if result.correlationId != "cid-v1:test_cid" {
		t.Error("CID lookup returned unexpected value: " + result.correlationId)
	}
}

// Tests that CID lookup fails immediately when a bad URL is provided.
func TestCidInvalidURL(t *testing.T) {
	mockClock()
	defer resetClock()
	defer resetCidLookup()

	ch := make(chan *correlationResult, 1)
	correlationManager.Query("**BAD_URL**", test_ikey, func(result *correlationResult) {
		ch <- result
	})

	slowTick(60)
	select {
	case result := <-ch:
		if result.err == nil {
			t.Error("Expected query result to contain an error")
		}
	case <-time.After(time.Second):
		t.Error("Timed out waiting for query result")
	}
}

// Tests that CID lookup is retried when the server sends a 500 response
func TestCidRetryOn500(t *testing.T) {
	mockClock()
	defer resetClock()
	resetCidLookup()
	defer resetCidLookup()

	server := newCidServer(map[string]string{test_ikey: "test_cid"})
	defer server.Close()
	server.response = 500

	resultCh := queryCid(server, test_ikey)

	// 500 shouldn't result in giving up just yet.
	server.waitForRequest(t)
	slowTick(1)
	select {
	case <-resultCh:
		t.Fatal("Correlation result was not expected yet")
	default:
		break
	}

	// Set to 200 and wait for another request.
	server.response = 200
	result := exhaustCidRetry(resultCh)
	server.waitForRequest(t)

	if result == nil {
		t.Error("Expected response after retry")
	} else if result.err != nil || result.correlationId != "cid-v1:test_cid" {
		t.Error("Unexpected result: %s, %s", result.err, result.correlationId)
	}
}

// Tests that CID lookup is not retried if the server returns a 404
func TestCidNoRetryOn404(t *testing.T) {
	mockClock()
	defer resetClock()
	resetCidLookup()
	defer resetCidLookup()

	server := newCidServer(map[string]string{})
	defer server.Close()

	resultCh := queryCid(server, test_ikey)
	server.waitForRequest(t)
	slowTick(1)
	select {
	case result := <-resultCh:
		if result.err == nil {
			t.Error("Expected an error result after only one query")
		}
	default:
		t.Error("Expected an error result after only one query")
	}
}

// Tests the correlation manager against relatively high parallel load
func TestCidParallelLookup(t *testing.T) {
	mockClock()
	defer resetClock()
	resetCidLookup()
	defer resetCidLookup()

	ikeys := []string{"ikey1", "ikey2", "ikey3"}
	answerKey := map[string]string{"ikey1": "result1", "ikey2": "result2", "ikey3": "result3"}

	server := newCidServer(answerKey)
	defer server.Close()

	// Start 60 goroutines that wait on one signal.
	var start sync.WaitGroup
	start.Add(1)
	var finish sync.WaitGroup

	// Ready...
	for i := 0; i < 60; i++ {
		n := i
		finish.Add(1)

		// Pick an ikey to lookup.  This will currently be spread across 3.
		lookup := ikeys[i%len(ikeys)]
		expected := "cid-v1:" + answerKey[lookup]

		go func(ikey, expected string) {
			defer finish.Done()
			start.Wait()

			select {
			case result := <-queryCid(server, ikey):
				if result == nil {
					t.Errorf("Goroutine %d failed to fetch CID", n)
				} else if result.err != nil {
					t.Errorf("Goroutine %d got error code: %s", n, result.err.Error())
				} else if result.correlationId != expected {
					t.Errorf("Goroutine %d got incorrect result: %s", n, result.correlationId)
				}
			case <-time.After(time.Second):
				t.Errorf("Goroutine %d timed out waiting for CID result", n)
			}
		}(lookup, expected)
	}

	// Set...
	slowTick(2)

	// Go!
	start.Done()

	// Make a channel to indicate when the wg has returned.
	allDone := make(chan struct{})
	go func() {
		finish.Wait()
		close(allDone)
	}()

	// Wait for requests for each ikey to come in...
	for i := 0; i < len(ikeys); i++ {
		server.waitForRequest(t)
	}

	// Wait for the signal
	select {
	case <-allDone:
		break
	case <-time.After(time.Second):
		t.Error("Goroutines did not all settle before timeout")
	}
}

// Tests that the correlation manager caches results and does not contact the endpoint unnecessarily.
func TestCidCache(t *testing.T) {
	mockClock()
	defer resetClock()
	resetCidLookup()
	defer resetCidLookup()

	server := newCidServer(map[string]string{test_ikey: "test_cid"})
	defer server.Close()

	// Perform first query
	result1Ch := queryCid(server, test_ikey)
	server.waitForRequest(t)
	result1 := exhaustCidRetry(result1Ch)
	if result1 == nil {
		t.Fatal("CID lookup timed out")
	} else if result1.err != nil {
		t.Fatalf("Failed to lookup cid: %s", result1.err.Error())
	} else if result1.correlationId != "cid-v1:test_cid" {
		t.Fatalf("Got wrong correlation ID: %s", result1.correlationId)
	}

	// Now set the server to return errors and try another query.
	server.response = 400
	result2 := exhaustCidRetry(queryCid(server, test_ikey))
	if result2 == nil {
		t.Fatal("CID lookup timed out")
	} else if result2.err != nil {
		t.Fatalf("Failed to lookup cid: %s", result2.err.Error())
	} else if result2.correlationId != "cid-v1:test_cid" {
		t.Fatalf("Got wrong correlation ID: %s", result2.correlationId)
	}
}

// Test helpers -----

// Mock cidLookup interface that services requests from the underlying map.
type fakeCidLookup map[string]string

// Query CID from cidLookup mock
func (lookup fakeCidLookup) Query(baseUri, ikey string, callback correlationCallback) {
	if result, ok := lookup[ikey]; ok {
		callback(&correlationResult{
			correlationId: correlationIdPrefix + result,
			err:           nil,
		})
	} else {
		callback(&correlationResult{
			correlationId: "",
			err:           fmt.Errorf("IKey not found"),
		})
	}
}

// Creates and uses a fakeCidLookup as the global correlation manager
func mockCidLookup(ikeys map[string]string) {
	if ikeys == nil {
		ikeys = make(map[string]string)
	}

	correlationManager = fakeCidLookup(ikeys)
}

// Resets the global correlation manager
func resetCidLookup() {
	correlationManager = newCorrelationIdManager()
}

// Test server that responds to CID lookups
type cidServer struct {
	*httptest.Server
	notify   chan struct{}
	ikeys    map[string]string
	response int
}

// Create a cidServer and place its address in the specified TelemetryConfiguration
func useCidServer(config *TelemetryConfiguration, ikeys map[string]string) *cidServer {
	server := newCidServer(ikeys)
	config.ProfileQueryEndpoint = "http://" + server.Listener.Addr().String()
	return server
}

// Creates a new cidServer
func newCidServer(ikeys map[string]string) *cidServer {
	result := &cidServer{
		notify:   make(chan struct{}, 16),
		ikeys:    ikeys,
		response: 200,
	}

	result.Server = httptest.NewServer(result)
	return result
}

// Closes this cidServer
func (server *cidServer) Close() {
	server.Server.Close()
	close(server.notify)
}

// Waits for the CID server to receive a request, and fails the specified test if one is not received
func (server *cidServer) waitForRequest(t *testing.T) {
	select {
	case <-server.notify:
		return
	case <-time.After(time.Second):
		t.Fatal("CID profile server did not receive request within a second")
	}
}

func (server *cidServer) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	code, response := server.serve(req)
	writer.WriteHeader(code)
	writer.Write([]byte(response))
	server.notify <- struct{}{}
}

func (server *cidServer) serve(req *http.Request) (int, string) {
	if req.Method != "GET" {
		return 405, ""
	}

	parts := strings.Split(req.URL.Path, "/")
	if len(parts) != 5 {
		return 400, ""
	}

	if parts[1] != "api" || parts[2] != "profiles" || parts[4] != "appId" {
		return 400, ""
	}

	if server.response != 200 {
		return server.response, ""
	}

	if cid, ok := server.ikeys[parts[3]]; !ok {
		return 404, ""
	} else {
		return 200, cid
	}
}

// Queries the specified CID server through the global correlation manager, returning a channel
// over which the result will be delivered.
func queryCid(server *cidServer, ikey string) chan *correlationResult {
	ch := make(chan *correlationResult, 1)
	correlationManager.Query("http://"+server.Listener.Addr().String(), ikey, func(result *correlationResult) {
		ch <- result
		close(ch)
	})

	return ch
}

// Ticks the clock until a correlation result is received on the specified channel.
func exhaustCidRetry(ch chan *correlationResult) *correlationResult {
	for i := 0; i < correlationMaxRetry; i++ {
		for t := 0; t < int(correlationRetryWait/time.Second); t++ {
			select {
			case result := <-ch:
				return result
			default:
				break
			}

			slowTick(1)
		}
	}

	return nil
}
