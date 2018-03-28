package aicollect

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
)

var defaultCorrelationExcludedDomains = []string{
	"*.core.windows.net",
	"*.core.chinacloudapi.cn",
	"*.core.cloudapi.de",
	"*.core.usgovcloudapi.net",
	"dc.services.visualstudio.com",
}

const (
	dependencyTypeHttp  = "Http"
	dependencyTypeAI    = "Http (tracked component)"
	correlationIdPrefix = "cid-v1:" // TODO: Deduplicate
)

// Monotonically increasing request number
var dependencyRequestNumber uint64 = 0

func InstrumentDefaultHTTPClient(client appinsights.TelemetryClient) {
	http.DefaultClient = NewHTTPClient(http.DefaultClient, client)
}

func NewHTTPClient(httpClient *http.Client, aiClient appinsights.TelemetryClient) *http.Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	result := *httpClient
	result.Transport = NewHTTPTransport(httpClient.Transport, aiClient)
	return &result
}

func NewHTTPTransport(transport http.RoundTripper, aiClient appinsights.TelemetryClient) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}

	return &roundtripper{
		transport:            transport,
		client:               aiClient,
		correlationBlacklist: compileBlacklist(aiClient.Config().CorrelationHeaderExcludedDomains),
	}
}

type roundtripper struct {
	transport            http.RoundTripper
	client               appinsights.TelemetryClient
	correlationBlacklist *regexp.Regexp
}

func (t *roundtripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if appinsights.CheckContextIgnore(r.Context()) {
		// Ignore this request.
		return t.transport.RoundTrip(r)
	}

	operation := appinsights.OperationFromContext(r.Context())
	client := t.client
	var id string

	if operation != nil {
		if !t.correlationBlacklist.MatchString(r.URL.Host) {
			id = attachCorrelationRequestHeaders(r, operation)
		}

		client = operation
	}

	startTime := time.Now()
	response, err := t.transport.RoundTrip(r)
	duration := time.Since(startTime)

	telem := appinsights.NewRemoteDependencyTelemetry("", dependencyTypeHttp, r.URL.Host, false)
	telem.Name = r.Method + " " + r.URL.Path
	telem.Timestamp = startTime
	telem.Duration = duration
	telem.Data = r.URL.String()
	telem.Id = id

	if err != nil {
		telem.Success = false
		telem.ResultCode = "0"
		// TODO: Err into data?
	} else {
		telem.Success = response.StatusCode < 400
		telem.ResultCode = response.Status
	}

	headers := parseCorrelationResponseHeaders(response)
	if headers.correlationId != "" && headers.correlationId != correlationIdPrefix {
		telem.Target = fmt.Sprintf("%s | %s | roleName:%s", r.URL.Host, headers.correlationId, headers.targetRoleName)
		telem.Type = dependencyTypeAI
	}

	client.Track(telem)
	return response, err
}

func compileBlacklist(excludedDomains []string) *regexp.Regexp {
	var fullList []string
	fullList = append(fullList, excludedDomains...)
	fullList = append(fullList, defaultCorrelationExcludedDomains...)

	var pattern bytes.Buffer
	pattern.WriteString("(?i)^(")
	for i, glob := range fullList {
		if i > 0 {
			pattern.WriteByte('|')
		}

		pattern.WriteString(globToPattern(glob))
	}

	pattern.WriteString(")$")
	return regexp.MustCompile(pattern.String())
}

func globToPattern(glob string) string {
	var pattern bytes.Buffer
	for len(glob) > 0 {
		star := strings.IndexByte(glob, '*')
		if star < 0 {
			pattern.WriteString(regexp.QuoteMeta(glob))
			break
		}

		pattern.WriteString(regexp.QuoteMeta(glob[:star]))
		pattern.WriteString(".*")
		glob = glob[star+1:]
	}

	return pattern.String()
}

func nextDependencyNumber() string {
	value := atomic.AddUint64(&dependencyRequestNumber, 1)
	return strconv.FormatUint(value, 10)
}
