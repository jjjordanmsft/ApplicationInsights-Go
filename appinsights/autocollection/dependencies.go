package autocollection

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
	"github.com/gobwas/glob"
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

// DependencyTrackingConfiguration specifies flags that modify the behavior of
// automatic dependency tracking.  For better forward-compatibility, callers
// should modify the result from NewDependencyTrackingConfiguration() rather
// than instantiate this structure directly.
type DependencyTrackingConfiguration struct {
	// SendCorrelationHeaders specifies whether dependency calls should
	// ever emit correlation headers to upstream servers.
	SendCorrelationHeaders bool

	// ExcludeDomains is a list of domains to which we should never send
	// correlation headers.  Wildcards are supported a la
	// github.com/gobwas/glob
	ExcludeDomains []string
}

// NewDependencyTrackingConfiguration returns a new, default configuration for
// automatic upstream dependency tracking.
func NewDependencyTrackingConfiguration() *DependencyTrackingConfiguration {
	var domains []string
	domains = append(domains, defaultCorrelationExcludedDomains...)

	return &DependencyTrackingConfiguration{
		SendCorrelationHeaders: true,
		ExcludeDomains:         domains,
	}
}

// InstrumentDefaultHTTPClient installs a remote dependency tracker in
// http.DefaultClient.
func InstrumentDefaultHTTPClient(client appinsights.TelemetryClient, config *DependencyTrackingConfiguration) {
	http.DefaultClient = NewHTTPClient(http.DefaultClient, client, config)
}

// NewHTTPClient wraps the input http.Client and tracks remote dependencies to
// the specified TelemetryClient.
func NewHTTPClient(httpClient *http.Client, aiClient appinsights.TelemetryClient, config *DependencyTrackingConfiguration) *http.Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	result := *httpClient
	result.Transport = NewHTTPTransport(httpClient.Transport, aiClient, config)
	return &result
}

// NewHTTPTransport wraps the input http.RoundTripper and tracks remote
// dependencies to the specified TelemetryClient.
func NewHTTPTransport(transport http.RoundTripper, aiClient appinsights.TelemetryClient, config *DependencyTrackingConfiguration) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}

	if config == nil {
		config = NewDependencyTrackingConfiguration()
	}

	return &roundtripper{
		transport: transport,
		client:    aiClient,
		config:    config,
		globs:     compileGlobs(config.ExcludeDomains),
	}
}

type roundtripper struct {
	transport http.RoundTripper
	client    appinsights.TelemetryClient
	config    *DependencyTrackingConfiguration
	globs     []glob.Glob
}

func (t *roundtripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if appinsights.CheckContextIgnore(r.Context()) {
		// Ignore this request.
		return t.transport.RoundTrip(r)
	}

	operation := appinsights.OperationFromContext(r.Context())
	client := t.client
	var id string

	if t.config.SendCorrelationHeaders && !matchAny(r.URL.Hostname(), t.globs) {
		if operation != nil {
			id = attachCorrelationRequestHeaders(r, operation)
		} else {
			attachRequestContextHeader(r, client)
		}
	}

	if operation != nil {
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
		telem.ResultCode = strconv.Itoa(response.StatusCode)

		headers := parseCorrelationResponseHeaders(response)
		if headers.correlationId != "" && headers.correlationId != correlationIdPrefix {
			telem.Target = headers.getCorrelatedTarget(r.URL)
			telem.Type = dependencyTypeAI
		}
	}

	client.Track(telem)
	return response, err
}

func compileGlobs(strs []string) []glob.Glob {
	var result []glob.Glob
	for _, s := range strs {
		result = append(result, glob.MustCompile(s, '.'))
	}

	return result
}

func matchAny(domain string, globs []glob.Glob) bool {
	for _, g := range globs {
		if g.Match(domain) {
			return true
		}
	}

	return false
}
