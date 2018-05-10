package appinsights

import (
	"time"

	"github.com/Microsoft/ApplicationInsights-Go/appinsights/contracts"
)

// Application Insights telemetry client provides interface to track telemetry
// items.
type TelemetryClient interface {
	// Gets the telemetry context for this client. Values found on this
	// context will get written out to every telemetry item tracked by
	// this client.
	Context() *TelemetryContext

	// Gets the configuration object that was used to initialize this
	// client.  Changing the config will not have an effect on further
	// operations of the client.
	Config() *TelemetryConfiguration

	// Gets the instrumentation key assigned to this telemetry client.
	InstrumentationKey() string

	// Gets the telemetry channel used to submit data to the backend.
	Channel() TelemetryChannel

	// CorrelationId returns the unique ID used to represent this application
	// when correlating operations across services.  This is fetched from
	// Application Insights servers using the instrumentation key.
	CorrelationId() string

	// Gets whether this client is enabled and will accept telemetry.
	IsEnabled() bool

	// Enables or disables the telemetry client. When disabled, telemetry
	// is silently swallowed by the client. Defaults to enabled.
	SetIsEnabled(enabled bool)

	// GetSamplingPercentage gets the current sampling percentage for the
	// client.
	GetSamplingPercentage() float64

	// SetSamplingPercentage sets the sampling percentage for the client.
	SetSamplingPercentage(samplingPercentage float64)

	// Submits the specified telemetry item.
	Track(telemetry Telemetry)

	// Log a user action with the specified name
	TrackEvent(name string)

	// Log a numeric value that is not specified with a specific event.
	// Typically used to send regular reports of performance indicators.
	TrackMetric(name string, value float64)

	// Log a trace message with the specified severity level.
	TrackTrace(name string, severity contracts.SeverityLevel)

	// Log an HTTP request with the specified method, URL, duration and
	// response code.
	TrackRequest(method, url string, duration time.Duration, responseCode string)

	// Log a dependency with the specified name, type, target, and
	// success status.
	TrackRemoteDependency(name, dependencyType, target string, success bool)

	// Log an availability test result with the specified test name,
	// duration, and success status.
	TrackAvailability(name string, duration time.Duration, success bool)

	// Log an exception with the specified error, which may be a string,
	// error or Stringer. The current callstack is collected
	// automatically.
	TrackException(err interface{})
}

type telemetryClient struct {
	channel   TelemetryChannel
	context   *TelemetryContext
	config    *TelemetryConfiguration
	cid       string
	sampling  float64
	isEnabled bool
}

// Creates a new telemetry client instance that submits telemetry with the
// specified instrumentation key.
func NewTelemetryClient(iKey string) TelemetryClient {
	return NewTelemetryClientFromConfig(NewTelemetryConfiguration(iKey))
}

// Creates a new telemetry client instance configured by the specified
// TelemetryConfiguration object.
func NewTelemetryClientFromConfig(config *TelemetryConfiguration) TelemetryClient {
	client := &telemetryClient{
		channel:   NewInMemoryChannel(config),
		context:   config.setupContext(),
		config:    config,
		isEnabled: true,
		sampling:  100.0,
	}

	client.cid = correlationIdPrefix
	correlationManager.Query(config.getCidEndpoint(), config.InstrumentationKey, func(result *correlationResult) {
		client.cid = result.correlationId
	})

	return client
}

// Gets the configuration object that was used to initialize this client.
// Changing the config will not have an effect on further operations of the
// client.
func (tc *telemetryClient) Config() *TelemetryConfiguration {
	return tc.config
}

// CorrelationId returns the unique ID used to represent this application
// when correlating operations across services.  This is fetched from
// Application Insights servers using the instrumentation key.
func (tc *telemetryClient) CorrelationId() string {
	return tc.cid
}

// Gets the telemetry context for this client.  Values found on this context
// will get written out to every telemetry item tracked by this client.
func (tc *telemetryClient) Context() *TelemetryContext {
	return tc.context
}

// Gets the telemetry channel used to submit data to the backend.
func (tc *telemetryClient) Channel() TelemetryChannel {
	return tc.channel
}

// Gets the instrumentation key assigned to this telemetry client.
func (tc *telemetryClient) InstrumentationKey() string {
	return tc.context.InstrumentationKey()
}

// Gets whether this client is enabled and will accept telemetry.
func (tc *telemetryClient) IsEnabled() bool {
	return tc.isEnabled
}

// Enables or disables the telemetry client.  When disabled, telemetry is
// silently swallowed by the client.  Defaults to enabled.
func (tc *telemetryClient) SetIsEnabled(isEnabled bool) {
	tc.isEnabled = isEnabled
}

// GetSamplingPercentage gets the current sampling percentage for the client.
func (tc *telemetryClient) GetSamplingPercentage() float64 {
	return tc.sampling
}

// SetSamplingPercentage sets the sampling percentage for the client.
func (tc *telemetryClient) SetSamplingPercentage(samplingPercentage float64) {
	tc.sampling = samplingPercentage
}

// Submits the specified telemetry item.
func (tc *telemetryClient) Track(item Telemetry) {
	if tc.isEnabled && item != nil {
		envelope := tc.context.envelop(item)
		oid := envelope.Tags[contracts.OperationId]
		if !item.CanSample() || tc.sampling >= 100.0 || OperationId(oid).Hash() < tc.sampling {
			envelope.SampleRate = tc.sampling
			tc.channel.Send(envelope)
		}
	}
}

// Log a user action with the specified name
func (tc *telemetryClient) TrackEvent(name string) {
	tc.Track(NewEventTelemetry(name))
}

// Log a numeric value that is not specified with a specific event.
// Typically used to send regular reports of performance indicators.
func (tc *telemetryClient) TrackMetric(name string, value float64) {
	tc.Track(NewMetricTelemetry(name, value))
}

// Log a trace message with the specified severity level.
func (tc *telemetryClient) TrackTrace(message string, severity contracts.SeverityLevel) {
	tc.Track(NewTraceTelemetry(message, severity))
}

// Log an HTTP request with the specified method, URL, duration and response
// code.
func (tc *telemetryClient) TrackRequest(method, url string, duration time.Duration, responseCode string) {
	tc.Track(NewRequestTelemetry(method, url, duration, responseCode))
}

// Log a dependency with the specified name, type, target, and success
// status.
func (tc *telemetryClient) TrackRemoteDependency(name, dependencyType, target string, success bool) {
	tc.Track(NewRemoteDependencyTelemetry(name, dependencyType, target, success))
}

// Log an availability test result with the specified test name, duration,
// and success status.
func (tc *telemetryClient) TrackAvailability(name string, duration time.Duration, success bool) {
	tc.Track(NewAvailabilityTelemetry(name, duration, success))
}

// Log an exception with the specified error, which may be a string, error
// or Stringer.  The current callstack is collected automatically.
func (tc *telemetryClient) TrackException(err interface{}) {
	tc.Track(newExceptionTelemetry(err, 1))
}
