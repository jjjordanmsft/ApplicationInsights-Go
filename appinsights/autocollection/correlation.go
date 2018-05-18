package autocollection

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights/contracts"
)

const (
	requestContextSourceKey         = "appId"
	requestContextTargetKey         = "appId"
	requestContextSourceRoleNameKey = "roleName"
	requestContextTargetRoleNameKey = "roleName"
	requestContextHeader            = "request-context"
	requestIdHeader                 = "request-id"
	correlationContextHeader        = "correlation-context"
	rootIdHeader                    = "x-ms-request-root-id"
	parentIdHeader                  = "x-ms-request-id"
)

var dependencyRequestNumber uint64 = 0

// correlationRequestHeaders encapsulates correlation data from incoming
// request headers
type correlationRequestHeaders struct {
	rootId         string
	parentId       appinsights.OperationId
	requestId      appinsights.OperationId
	properties     appinsights.CorrelationProperties
	requestContext appinsights.CorrelationProperties
}

// correlationResponseHeaders encapsulates correlation data from dependency
// response headers
type correlationResponseHeaders struct {
	properties     appinsights.CorrelationProperties
	correlationId  string
	targetRoleName string
}

// parseCorrelationRequestHeaders parses correlation data out of incoming
// request headers
func parseCorrelationRequestHeaders(r *http.Request) *correlationRequestHeaders {
	result := &correlationRequestHeaders{}
	result.requestContext = appinsights.ParseCorrelationProperties(r.Header.Get(requestContextHeader))

	if h := r.Header.Get(requestIdHeader); h != "" {
		result.parentId = appinsights.OperationId(h)
		result.requestId = result.parentId.GenerateRequestId()
		result.properties = appinsights.ParseCorrelationProperties(r.Header.Get(correlationContextHeader))
		result.rootId = string(result.requestId.GetRoot())
	} else {
		result.rootId = r.Header.Get(rootIdHeader)
		result.parentId = appinsights.OperationId(r.Header.Get(parentIdHeader))
		if string(result.rootId) != "" {
			result.requestId = appinsights.OperationId(result.rootId).GenerateRequestId()
		} else {
			result.requestId = result.parentId.GenerateRequestId()
		}

		result.properties = make(appinsights.CorrelationProperties)
	}

	return result
}

// getCorrelatedSource formats the Source field for incoming Requeset telemetry
func (headers *correlationRequestHeaders) getCorrelatedSource() string {
	sourceCorrelationId := headers.requestContext[requestContextSourceKey]
	sourceRoleName := headers.requestContext[requestContextSourceRoleNameKey]
	return sourceCorrelationId + " | roleName:" + sourceRoleName
}

// attachCorrelationRequestHeaders adds correlation headers to outgoing
// requesets
func attachCorrelationRequestHeaders(r *http.Request, operation appinsights.Operation) string {
	correlation := operation.Correlation()
	id := string(correlation.ParentId().AppendSuffix(nextDependencyNumber(), "."))
	r.Header.Set(requestIdHeader, id)
	r.Header.Set(rootIdHeader, id)
	r.Header.Set(parentIdHeader, correlation.Id().String())
	r.Header.Set(correlationContextHeader, correlation.Properties().Serialize())

	// Request context header
	requestContext := r.Header.Get(requestContextHeader)
	props := appinsights.ParseCorrelationProperties(requestContext)
	if correlationId := operation.CorrelationId(); props[requestContextSourceKey] == "" && correlationId != "" {
		props[requestContextSourceKey] = correlationId
	}
	if cloudRole := operation.Context().Tags[contracts.CloudRole]; props[requestContextSourceRoleNameKey] == "" && cloudRole != "" {
		props[requestContextSourceRoleNameKey] = cloudRole
	}
	r.Header.Set(requestContextHeader, props.Serialize())

	return id
}

// parseCorrelationResponseHeaders parses correlation data from dependency
// response headers
func parseCorrelationResponseHeaders(r *http.Response) *correlationResponseHeaders {
	properties := appinsights.ParseCorrelationProperties(r.Header.Get(requestContextHeader))
	return &correlationResponseHeaders{
		properties:     properties,
		correlationId:  properties[requestContextTargetKey],
		targetRoleName: properties[requestContextTargetRoleNameKey],
	}
}

// getCorrelatedTarget formats the Target field for dependency telemetry
func (headers *correlationResponseHeaders) getCorrelatedTarget(uri *url.URL) string {
	return fmt.Sprintf("%s | %s | roleName:%s", uri.Hostname(), headers.correlationId, headers.targetRoleName)
}

// writeCorrelationResponseHeaders writes correlation headers to responses
func getCorrelationResponseHeaders(operation appinsights.Operation) map[string]string {
	properties := make(appinsights.CorrelationProperties)
	properties[requestContextTargetKey] = operation.CorrelationId()
	properties[requestContextTargetRoleNameKey] = operation.Context().Tags[contracts.CloudRole]

	return map[string]string{
		requestContextHeader: properties.Serialize(),
	}
}

// Gets a monotonically increasing integer used for generating unique request ids.
func nextDependencyNumber() string {
	value := atomic.AddUint64(&dependencyRequestNumber, 1)
	return strconv.FormatUint(value, 10)
}
