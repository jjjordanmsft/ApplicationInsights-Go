package aicollect

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
)

var defaultCorrelationExcludedDomains = []string{
	"*.core.windows.net",
	"*.core.chinacloudapi.cn",
	"*.core.cloudapi.de",
	"*.core.usgovcloudapi.net",
}

func InstrumentDefaultHTTPClient(client appinsights.TelemetryClient) {
	http.DefaultClient = MakeHTTPClient(http.DefaultClient, client)
}

func MakeHTTPClient(httpClient *http.Client, aiClient appinsights.TelemetryClient) *http.Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	result := *httpClient
	result.Transport = MakeHTTPTransport(httpClient.Transport, aiClient)
	return &result
}

func MakeHTTPTransport(transport http.RoundTripper, aiClient appinsights.TelemetryClient) http.RoundTripper {
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
	operation := appinsights.GetContextOperation(r.Context())
	client := t.client
	if operation != nil {
		if !t.correlationBlacklist.MatchString(r.URL.Host) {
			// TODO: Add correlation headers..
		}

		client = operation
	}

	startTime := time.Now()
	response, err := t.transport.RoundTrip(r)
	duration := time.Since(startTime)

	telem := appinsights.NewRemoteDependencyTelemetry("", "HTTP", r.URL.Host, false)
	telem.Timestamp = startTime
	telem.Duration = duration
	telem.Data = r.URL.String()

	if err != nil {
		telem.Success = false
		telem.ResultCode = "0"
		// TODO: Err into data?
	} else {
		telem.Success = response.StatusCode < 400 || response.StatusCode == 401
		telem.ResultCode = response.Status
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
