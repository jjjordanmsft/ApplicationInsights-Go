package aicollect

import (
	"net/http"

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

type correlationRequestHeaders struct {
	rootId     string
	parentId   appinsights.OperationId
	requestId  appinsights.OperationId
	properties appinsights.CorrelationProperties
}

type correlationResponseHeaders struct {
	properties     appinsights.CorrelationProperties
	correlationId  string
	targetRoleName string
}

func parseCorrelationRequestHeaders(r *http.Request) *correlationRequestHeaders {
	result := &correlationRequestHeaders{}

	if h := r.Header.Get(requestIdHeader); h != "" {
		result.parentId = appinsights.OperationId(h)
		result.requestId = result.parentId.GenerateRequestId()
		result.properties = appinsights.ParseCorrelationProperties(r.Header.Get(requestContextHeader))
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

	return result //appinsights.NewCorrelationContext(requestId.GetRoot(), requestId, "", properties), requestId
}

func attachCorrelationRequestHeaders(r *http.Request, operation appinsights.Operation) string {
	correlation := operation.Correlation()
	id := string(correlation.ParentId.AppendSuffix(nextDependencyNumber(), "."))
	r.Header.Set(requestIdHeader, id)
	r.Header.Set(rootIdHeader, id)
	r.Header.Set(parentIdHeader, string(correlation.Id))
	r.Header.Set(correlationContextHeader, correlation.Properties.Serialize())

	// Request context header
	requestContext := r.Header.Get(requestContextHeader)
	props := appinsights.ParseCorrelationProperties(requestContext)
	if props[requestContextSourceKey] == "" {
		props[requestContextSourceKey] = operation.CorrelationId()
	}
	if props[requestContextSourceRoleNameKey] == "" {
		props[requestContextSourceRoleNameKey] = operation.Context().Tags[contracts.CloudRole]
	}
	r.Header.Set(requestContextHeader, props.Serialize())

	return id
}

func parseCorrelationResponseHeaders(r *http.Response) *correlationResponseHeaders {
	properties := appinsights.ParseCorrelationProperties(r.Header.Get(requestContextHeader))
	return &correlationResponseHeaders{
		properties:     properties,
		correlationId:  properties[requestContextTargetKey],
		targetRoleName: properties[requestContextTargetRoleNameKey],
	}
}

func writeCorrelationResponseHeaders(rw http.ResponseWriter, operation appinsights.Operation) {
	properties := make(appinsights.CorrelationProperties)
	properties[requestContextSourceKey] = operation.CorrelationId()
	properties[requestContextSourceRoleNameKey] = operation.Context().Tags[contracts.CloudRole]

	headers := rw.Header()
	headers.Set(requestContextHeader, properties.Serialize())
}

func getCorrelatedSource(context *appinsights.CorrelationContext) string {
	if sourceCorrelationId, ok := context.Properties[requestContextSourceKey]; ok {
		if sourceRoleName, ok := context.Properties[requestContextSourceRoleNameKey]; ok {
			return sourceCorrelationId + "|roleName:" + sourceRoleName
		}
	}

	return ""
}
