package collection

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"
	"time"
	
	"github.com/jjjordanmsft/ApplicationInsights-Go/appinsights"
)

var defaultIgnore = []string{
	"*.blob.core.windows.net",
	"*.blob.core.chinacloudapi.cn",
	"*.blob.core.cloudapi.de",
	"*.blob.core.usgovcloudapi.net",
}

func MakeHTTPClient(client *http.Client, tclient appinsights.TelemetryClient, ignoreHosts []string) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	
	ignore := append(ignoreRegexes(ignoreHosts), ignoreRegexes(defaultIgnore)...)
	
	result := *client
	result.Transport = &roundtripper{transport: transport, client: client, ignore: ignore}
	return &result
}

type roundtripper struct {
	transport  client.Transport
	client     appinsights.TelemetryClient
	ignore     []string
}

func (t *roundtripper) RoundTrip(r *http.Request) (*http.Response, error) {
	operation := ExtractOperationFromRequest(r)
	client := t.client
	
	if operation != nil {
		// Add correlation headers to request unless ignored
		if !t.ignore.contains(r.URL.Host()) {
			// TODO
		}
		
		// Use the operation as the client for correlation.
		client = operation
	}
	
	startTime := time.Now()
	response, err := t.transport.RoundTrip(r)
	duration := time.Since(startTime)

	telem := appinsights.NewRemoteDependencyTelemetry("", "HTTP", r.URL.Host(), false)
	telem.Timestamp = startTime
	telem.Duration = duration
	telem.Data = r.URL.String()
	
	if err != nil {
		telem.Success = false
		telem.ResultCode = "0"
		// TODO: What to do with err?
	} else {
		telem.Success = r.ResultCode < 400 || r.ResultCode == 401
		telem.ResultCode = r.ResultCode
	}
	
	client.Track(telem)
	return response, err
}

type hostGlobs []*regexp.Regexp

func (globs *hostGlobs) contains(host string) bool {
	for _, r := range globs {
		if r.MatchString(host) {
			return true
		}
	}
	
	return false
}

func ignoreRegexes(hosts []string) hostGlobs {
	var result []*regexp.Regexp
	for _, glob := range hosts {
		result = append(result, globToRegex(glob))
	}
	return result
}

func globToRegex(glob string) *regexp.Regexp {
	var result bytes.Buffer
	result.WriteString("(?i)^")
	for {
		star := strings.IndexByte(glob, '*')
		if star < 0 {
			result.WriteString(regexp.QuoteMeta(glob))
			break
		}
		
		result.WriteString(regexp.QuoteMeta(glob[:star]))
		result.WriteString(".*")
		glob = glob[star + 1:]
	}
	
	result.WriteString("$")
	return regexp.MustCompile(result.String())
}
