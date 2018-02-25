package appinsights

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	correlationMaxRetry  = 5
	correlationRetryWait = 10 * time.Second
)

type correlationIdManager struct {
	lock    sync.Mutex
	baseUri string
	pending map[string]*correlationLookup
	results map[string]*correlationResult
}

type correlationCallback func(*correlationResult)

type correlationLookup struct {
	ikey      string
	callbacks []correlationCallback
}

type correlationResult struct {
	ikey          string
	correlationId string
	err           error
}

func newCorrelationIdManager() *correlationIdManager {
	return &correlationIdManager{
		baseUri: "",
		pending: make(map[string]*correlationLookup),
		results: make(map[string]*correlationResult),
	}
}

func (manager *correlationIdManager) Query(ikey string, callback correlationCallback) {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	upperId := strings.ToLower(ikey)
	if result, ok := manager.results[upperId]; ok {
		callback(result)
	} else if pending, ok := manager.pending[upperId]; ok {
		pending.callbacks = append(pending.callbacks, callback)
	} else {
		lookup := &correlationLookup{
			ikey:      ikey,
			callbacks: []correlationCallback{callback},
		}

		manager.pending[upperId] = lookup
		go manager.lookup(lookup)
	}
}

func (manager *correlationIdManager) lookup(lookup *correlationLookup) {
	var lastError error
	url := fmt.Sprintf("%s/api/profiles/%s/appId", manager.baseUri, lookup.ikey)
	for i := 0; i < correlationMaxRetry; i++ {
		cid, retry, err := tryLookupCorrelationId(url)
		if err == nil {
			manager.postResult(lookup, cid, nil)
			return
		}

		if retry {
			lastError = err
			time.Sleep(correlationRetryWait)
			continue
		} else {
			lastError = err
			break
		}
	}

	manager.postResult(lookup, "", lastError)
}

func (manager *correlationIdManager) postResult(lookup *correlationLookup, correlationId string, err error) {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	result := &correlationResult{
		ikey:          lookup.ikey,
		correlationId: correlationId,
		err:           err,
	}

	upperId := strings.ToUpper(lookup.ikey)
	manager.results[upperId] = result
	delete(manager.pending, upperId)

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
	resp, err := client.Do(req)
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
