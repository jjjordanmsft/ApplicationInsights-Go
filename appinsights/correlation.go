package appinsights

import (
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/satori/go.uuid"
)

const (
	correlationIdPrefix         = "cid-v1:"
	correlationRequestMaxLength = 1024
	correlationMaxRetry         = 4
	correlationRetryWait        = 10 * time.Second
)

type CorrelationIdManager struct {
	baseUri        string
	currentRootId  uint32
	lookupCache    map[string]string
	pendingLookups map[string]*correlationLookup
	queryChannel   chan *correlationQuery
	resultChannel  chan *CorrelationResult
}

type correlationQuery struct {
	instrumentationKey string
	result             chan *CorrelationResult
	cancel             bool
}

type correlationLookup struct {
	cancel  chan bool
	results []chan *CorrelationResult
}

type CorrelationResult struct {
	InstrumentationKey string
	CorrelationId      string
	Err                error
}

func NewCorrelationIdManager(baseUri string) *CorrelationIdManager {
	manager := &CorrelationIdManager{
		baseUri:        baseUri,
		currentRootId:  rand.Uint32(),
		queryChannel:   make(chan *correlationQuery),
		resultChannel:  make(chan *CorrelationResult),
		lookupCache:    make(map[string]*CorrelationResult),
		pendingLookups: make(map[string]*correlationLookup),
	}

	go manager.run()

	return manager
}

func (manager *CorrelationIdManager) Query(instrumentationKey string) chan<- *CorrelationResult {
	resultChan := make(chan *CorrelationResult, 1)
	query := &correlationQuery{
		instrumentationKey: instrumentationKey,
		result:             resultChan,
	}

	manager.queryChannel <- query
	return resultChan
}

func (manager *CorrelationIdManager) QuerySync(instrumentationKey string) (string, error) {
	result := <-manager.Query(instrumentationKey)
	return result.instrumentationKey, result.err
}

func (manager *CorrelationIdManager) Cancel(instrumentationKey string) {
	query := &correlationQuery{
		instrumentationKey: instrumentationKey,
		cancel:             true,
	}

	manager.queryChannel <- query
}

func (manager *CorrelationIdManager) run() {
	for {
		select {
		case query := <-manager.queryChannel:
			if !query.cancel {
				if resultValue, ok := manager.lookupCache[query.instrumentationKey]; ok {
					query.result <- &CorrelationResult{
						InstrumentationKey: query.instrumentationKey,
						ApplicationId:      resultValue,
						Err:                nil,
					}
				} else if lookup, ok := manager.pendingLookups[query.instrumentationKey]; ok {
					lookup.results = append(lookup.results, query.result)
				} else {
					pendingLookup = &pendingLookup{
						cancel:  make(chan bool, 1),
						results: []chan *CorrelationResult{query.result},
					}

					go manager.lookup(query.instrumentationKey, pendingLookup.cancel)
					manager.pendingLookups[query.instrumentationKey] = pendingLookup
				}
			} else {
				// Cancel query.
				if pending, ok := manager.pendingLookups[query.instrumentationKey]; ok {
					close(pending.cancel)
					delete(manager.pendingLookups, query.instrumentationKey)
					result := &CorrelationResult{
						InstrumentationKey: query.instrumentationKey,
						ApplicationId:      "",
						Err:                fmt.Error("Lookup canceled"),
					}

					for _, ch := range pending.results {
						ch <- result
					}
				}
			}

		case result := <-manager.resultChannel:
			if pending, ok := manager.pendingLookups[result.instrumentationKey]; ok {
				delete(manager.pendingLookups, result.instrumentationKey)
				for _, ch := range pending.results {
					ch <- result
				}

				manager.lookupCache[result.instrumentationKey] = result
				close(pending.cancel)
			}
		}
	}
}

func (manager *CorrelationIdManager) lookup(ikey string, cancel chan bool) {
	var lastError error
	url := fmt.Sprintf("%s/api/profiles/%s/appId", manager.baseUri, ikey)
	for i := 0; i < correlationMaxRetry; i++ {
		select {
		case <-cancel:
			// Received cancel notification -- give up.
			return
		}

		result, retry, err := tryLookupCorrelationId(url)
		if err == nil {
			// Send back answer.
			manager.resultChannel <- &CorrelationResult{
				InstrumentationKey: ikey,
				ApplicationId:      correlationIdPrefix + result,
			}

			return
		}

		if retry {
			lastError = err
			time.Sleep(correlationRetryWait)
			continue
		} else {
			// Fail
			lastError = err
			break
		}
	}

	manager.resultChannel <- &CorrelationResult{
		InstrumentationKey: ikey,
		Err:                lastError,
	}
}

func tryLookupCorrelationId(url string) (string, bool, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", true, err
	}

	client := http.DefaultClient
	resp, err := client.Do(req)
	if err != nil {
		return "", true, err
	}

	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		// Likely a bad key, do not try again.
		return "", false, fmt.Errorf("Received status code %d from server", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", true, err
	}

	return body, false, nil
}

func (manager *CorrelationIdManager) generateRequestId(parentId string) string {
	if parentId != "" {
		if parentId[0] != '|' {
			parentId = "|" + parentId
		}
		if parentId[len(parentId-1)] != '.' {
			parentId = parentId + "."
		}

		return appendSuffix(parentId, manager.nextRootId(), "_")
	} else {
		return generateRootId()
	}
}

func (manager *CorrelationIdManager) nextRootId() string {
	value := atomic.AddUint32(&manager.currentRootId, 1)
	return strconv.FormatUint(value, 16)
}

func generateRootId() string {
	return "|" + uuid.NewV4() + "."
}

func getRootId(id string) string {
	end := strings.IndexByte(id, '.')
	if end < 0 {
		end = len(id)
	}

	if id[0] == '|' {
		return id[1:end]
	} else {
		return id[:end]
	}
}

func appendSuffix(parentId, suffix, delimiter string) string {
	if (len(parentId) + len(suffix) + len(delimiter)) <= correlationRequestMaxLength {
		return parentId + suffix + delimiter
	}

	// Combined id too long: we need 9 characters of space, 8 for the
	// overflow ID and 1 for the overflow delimiter '#'.
	x := correlationRequestMaxLength - 9
	if len(parentId) > x {
		for ; x > 1; x-- {
			c := parentId[x-1]
			if c == '.' || c == '_' {
				break
			}
		}
	}

	if x <= 1 {
		// parentId must not have been valid
		return generateRootId()
	}

	return fmt.Sprintf("%s%08ux#", parentId[:x], rand.Uint32())
}
