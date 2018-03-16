package aicollect

import (
	"net/http"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
)

const (
	requestContextSourceKey         = "appId"
	requestContextTargetKey         = "appId"
	requestContextSourceRoleNameKey = "roleName"
	requestContextTargetRoleNameKey = "roleName"
	requestContextHeader            = "request-context"
	requestIdHeader                 = "request-id"
	correlationContextHeader        = "correlation-context"
)

var cidLookup CidLookup

func init() {
	cidLookup = newCorrelationIdManager()
}

func parseCorrelationHeaders(r *http.Request) (*appinsights.CorrelationContext, appinsights.OperationId) {
	parentId := appinsights.OperationId(r.Header.Get(requestIdHeader))
	requestId := parentId.GenerateRequestId()
	id := requestId.GetRoot()
	properties := appinsights.ParseCorrelationProperties(r.Header.Get(requestContextHeader))

	return &appinsights.CorrelationContext{
		ParentId:   parentId,
		Id:         id,
		Properties: properties,
	}, requestId
}

func getCorrelatedSource(context *appinsights.CorrelationContext) string {
	if sourceCorrelationId, ok := context.Properties[requestContextSourceKey]; ok {
		if sourceRoleName, ok := context.Properties[requestContextSourceRoleNameKey]; ok {
			return sourceCorrelationId + " | roleName:" + sourceRoleName
		}
	}

	return ""
}
