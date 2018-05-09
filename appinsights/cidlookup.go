package appinsights

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	correlationMaxRetry  = 5
	correlationRetryWait = 10 * time.Second
	correlationIdPrefix  = "cid-v1:"
)

// cidLookup is the interface for the CID lookup implementation (allowing it to be mocked out)
type cidLookup interface {
	// Query looks up the specified instrumentation key's correlation ID from the specified endpoint,
	// and invokes callback with the result when it is available.
	Query(baseUri, ikey string, callback correlationCallback)
}

// correlationCallback is the function signature for receiving callbacks
type correlationCallback func(*correlationResult)

// correlationResult encapsulates the CID lookup response
type correlationResult struct {
	correlationId string
	err           error
}

// correlationIdManager Tracks pending and completed CID requests and maintains a cache of responses.
type correlationIdManager struct {
	lock    sync.Mutex
	pending map[string]*correlationLookup
	results map[string]*correlationResult
}

// correlationLookup represents an active CID lookup and tracks which callbacks must be invoked when
// the response is returned.
type correlationLookup struct {
	ikey      string
	id        string
	url       string
	callbacks []correlationCallback
}

// correlationManager is the singleton correlationIdManager that globally tracks all lookups for this process.
var correlationManager cidLookup

func init() {
	correlationManager = newCorrelationIdManager()
}

// newCorrelationIdManager creates a new, empty correlationIdManager.
func newCorrelationIdManager() *correlationIdManager {
	return &correlationIdManager{
		pending: make(map[string]*correlationLookup),
		results: make(map[string]*correlationResult),
	}
}

// Query looks up the specified instrumentation key's correlation ID from the specified endpoint,
// and invokes callback with the result when it is available.
func (manager *correlationIdManager) Query(baseUri, ikey string, callback correlationCallback) {
	baseUrl, err := url.Parse(baseUri)
	if err != nil {
		callback(&correlationResult{"", err})
		return
	}

	baseUrl.RawQuery = ""
	baseUrl.Fragment = ""
	baseUrl.Path = fmt.Sprintf("api/profiles/%s/appId", ikey)
	url := baseUrl.String()

	manager.lock.Lock()
	defer manager.lock.Unlock()

	id := strings.ToUpper(url)
	if result, ok := manager.results[id]; ok {
		callback(result)
	} else if pending, ok := manager.pending[id]; ok {
		pending.callbacks = append(pending.callbacks, callback)
	} else {
		lookup := &correlationLookup{
			ikey:      ikey,
			url:       url,
			id:        id,
			callbacks: []correlationCallback{callback},
		}

		manager.pending[id] = lookup
		go manager.lookup(lookup)
	}
}

// lookup is the background cid lookup routine that processes retries.
func (manager *correlationIdManager) lookup(lookup *correlationLookup) {
	diagnosticsWriter.Printf("Looking up correlation ID for %s", lookup.ikey)

	var lastError error
	for i := 0; i < correlationMaxRetry; i++ {
		cid, retry, err := tryLookupCorrelationId(lookup.url)
		if err == nil {
			manager.postResult(lookup, cid, nil)
			return
		} else if retry {
			lastError = err
			currentClock.Sleep(correlationRetryWait)
		} else {
			lastError = err
			break
		}
	}

	manager.postResult(lookup, "", lastError)
}

// postResult updates the correlationIdManager's internal tables and invokes all
// callbacks when a CID result is received.
func (manager *correlationIdManager) postResult(lookup *correlationLookup, correlationId string, err error) {
	if err != nil {
		diagnosticsWriter.Printf("Failed to lookup correlation ID for %s: %s", lookup.ikey, err.Error())
	} else {
		diagnosticsWriter.Printf("Completed correlation ID lookup for %s", lookup.ikey)
	}

	manager.lock.Lock()
	defer manager.lock.Unlock()

	if err == nil {
		correlationId = correlationIdPrefix + correlationId
	}

	result := &correlationResult{
		correlationId: correlationId,
		err:           err,
	}

	manager.results[lookup.id] = result
	delete(manager.pending, lookup.id)

	for _, callback := range lookup.callbacks {
		callback(result)
	}
}

// tryLookupCorrelationId performs the network request for the correlation ID.
func tryLookupCorrelationId(url string) (string, bool, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		// Invalid URL? Don't retry.
		return "", false, err
	}

	client := http.DefaultClient
	resp, err := client.Do(MarkRequestIgnore(req))
	if err != nil {
		// Connection error: retry
		return "", true, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		// Bad key? Don't retry.
		retry := (resp.StatusCode < 400 || resp.StatusCode >= 500)
		return "", retry, fmt.Errorf("Received status code %d from server", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", true, err
	}

	return string(body), false, nil
}
