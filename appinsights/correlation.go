package appinsights

import (
	"bytes"
	"strings"
)

type CorrelationContext struct {
	Name     string
	Id       OperationId
	ParentId OperationId

	Properties CorrelationProperties
}

func NewCorrelationContext(operationId, parentId OperationId, name string, properties CorrelationProperties) *CorrelationContext {
	return &CorrelationContext{
		Name:       name,
		Id:         operationId,
		ParentId:   parentId,
		Properties: properties,
	}
}

type CorrelationProperties map[string]string

func ParseCorrelationProperties(header string) CorrelationProperties {
	result := make(CorrelationProperties)

	entries := strings.Split(header, ",")
	for _, entry := range entries {
		kv := strings.SplitN(entry, "=", 2)
		if len(kv) == 2 {
			result[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}

	return result
}

func (props CorrelationProperties) Serialize() string {
	var result bytes.Buffer
	for k, v := range props {
		if strings.ContainsRune(k, ',') || strings.ContainsRune(k, '=') || strings.ContainsRune(v, ',') || strings.ContainsRune(v, '=') {
			diagnosticsWriter.Printf("Custom properties must not contains '=' or ','. Dropping key \"%s\"", k)
		} else {
			if result.Len() > 0 {
				result.WriteRune(',')
			}
			result.WriteString(k)
			result.WriteRune('=')
			result.WriteString(v)
		}
	}

	return result.String()
}
