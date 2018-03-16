package aicollect

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
)

const (
	correlationMaxRetry  = 5
	correlationRetryWait = 10 * time.Second
)

type CidLookup interface {
	Query(baseUri, ikey string, callback correlationCallback)
}

type correlationIdManager struct {
	lock    sync.Mutex
	pending map[string]*correlationLookup
	results map[string]*correlationResult
}

type correlationCallback func(*correlationResult)

type correlationLookup struct {
	ikey      string
	id        string
	url       string
	callbacks []correlationCallback
}

type correlationResult struct {
	correlationId string
	err           error
}

func newCorrelationIdManager() *correlationIdManager {
	return &correlationIdManager{
		pending: make(map[string]*correlationLookup),
		results: make(map[string]*correlationResult),
	}
}

func (manager *correlationIdManager) Query(baseUri, ikey string, callback correlationCallback) {
	baseUrl, err := url.Parse(baseUri)
	if err != nil {
		callback(&correlationResult{"", err})
		return
	}

	baseUrl.RawQuery = ""
	baseUrl.Fragment = ""
	baseUrl.Path = "api/profiles/" + ikey + "/appId"
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

func (manager *correlationIdManager) lookup(lookup *correlationLookup) {
	// diagnosticsWriter.Printf("Looking up correlation ID for %s", lookup.ikey)

	var lastError error
	for i := 0; i < correlationMaxRetry; i++ {
		cid, retry, err := tryLookupCorrelationId(lookup.url)
		if err == nil {
			manager.postResult(lookup, cid, nil)
			return
		} else if retry {
			lastError = err
			time.Sleep(correlationRetryWait)
		} else {
			lastError = err
			break
		}
	}

	manager.postResult(lookup, "", lastError)
}

func (manager *correlationIdManager) postResult(lookup *correlationLookup, correlationId string, err error) {
	if err != nil {
		// diagnosticsWriter.Printf("Failed to lookup correlation ID for %s: %s", lookup.ikey, err.Error())
	} else {
		// diagnosticsWriter.Printf("Completed correlation ID lookup for %s", lookup.ikey)
	}

	manager.lock.Lock()
	defer manager.lock.Unlock()

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

func tryLookupCorrelationId(url string) (string, bool, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		// Invalid URL? Don't retry.
		return "", false, err
	}

	client := http.DefaultClient
	resp, err := client.Do(appinsights.MarkRequestIgnore(req))
	if err != nil {
		// Connection error: retry
		return "", true, err
	}

	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		// Bad key? Don't retry.
		return "", false, fmt.Errorf("Received status code %d from server", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", true, err
	}

	return string(body), false, nil
}
