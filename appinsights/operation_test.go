package appinsights

import (
	"regexp"
	"testing"
	"time"
)

func TestNewCorrelationContext(t *testing.T) {
	// Test that a new operation ID is generated + the empty case.
	cc1 := NewCorrelationContext(OperationId(""), OperationId(""), "", nil)
	if cc1.Name() != "" {
		t.Error("Name wasn't as expected")
	}
	if match, _ := regexp.MatchString("^"+newIdPattern+"$", cc1.Id().String()); !match {
		t.Error("Expected a new OperationID in Id")
	}
	if cc1.Id() != cc1.ParentId() {
		t.Error("Expected ParentId == Id")
	}
	if cc1.Properties() == nil || len(cc1.Properties()) != 0 {
		t.Error("Expected a new, empty CorrelationProperties map")
	}

	// Test that the parent ID gets assigned to Operation ID if not provided
	cc2 := NewCorrelationContext(OperationId("rootOperation"), OperationId(""), "-name-", CorrelationProperties{})
	if cc2.Name() != "-name-" {
		t.Error("Name wasn't as expected")
	}
	if cc2.Id().String() != "rootOperation" {
		t.Error("Id wasn't as expected")
	}
	if cc2.ParentId().String() != "rootOperation" {
		t.Error("ParentId didn't match Id")
	}
	if cc2.Properties() == nil || len(cc2.Properties()) != 0 {
		t.Error("Expected an empty correlation properties map")
	}

	// Test the regular case
	cc3 := NewCorrelationContext(OperationId("rootOperation"), OperationId("|rootOperation.foo_"), "-name-", CorrelationProperties{"foo": "bar"})
	if cc3.Name() != "-name-" {
		t.Error("Name wasn't as expected")
	}
	if cc3.Id().String() != "rootOperation" {
		t.Error("Id wasn't as expected")
	}
	if cc3.ParentId().String() != "|rootOperation.foo_" {
		t.Error("ParentId wasn't as expected")
	}
	if v, ok := cc3.Properties()["foo"]; !ok || v != "bar" {
		t.Error("Properties wasn't propagated as expected")
	}
}

func TestOperation(t *testing.T) {
	mockCidLookup(nil)
	defer resetCidLookup()

	channel, client := newMockChannelClient(nil)

	// Generate some IDs.
	oid := NewOperationId()
	reqid := oid.GenerateRequestId()

	// Add a common property to root client
	client.Context().CommonProperties["common"] = "prop"

	// Make a request
	req := NewRequestTelemetry("GET", "http://localhost/foo", time.Second, "200")
	req.Id = reqid.String()
	req.Tags.Operation().SetParentId(oid.String())

	// Make an event
	ev := NewEventTelemetry("Hit")

	// Make an operation
	correlation := NewCorrelationContext(oid, reqid, req.Name, nil)
	op := NewOperation(client, correlation)

	// Log.
	op.Track(req)
	op.Track(ev)

	// Check...
	j := parsePayload(t, channel.items.serialize())
	j[0].assertPath(t, "data.baseType", "RequestData")
	j[0].assertPath(t, "tags.'ai.operation.id'", oid.String())
	j[0].assertPath(t, "tags.'ai.operation.parentId'", oid.String())
	j[0].assertPath(t, "tags.'ai.operation.name'", req.Name)

	j[1].assertPath(t, "data.baseType", "EventData")
	j[1].assertPath(t, "tags.'ai.operation.id'", oid.String())
	j[1].assertPath(t, "tags.'ai.operation.parentId'", reqid.String())
	j[1].assertPath(t, "tags.'ai.operation.name'", req.Name)
}

func TestOperationProperties(t *testing.T) {
	mockCidLookup(map[string]string{test_ikey: "test_cid"})
	defer resetCidLookup()

	// Act
	properties := ParseCorrelationProperties("a=b,c=d")
	correlation := NewCorrelationContext(OperationId("a"), OperationId("b"), "c", properties)
	channel, client := newMockChannelClient(nil)
	client.SetSampleRate(99.0)
	client.SetIsEnabled(false)
	operation := NewOperation(client, correlation)

	// Assert
	if operation.CorrelationId() != "cid-v1:test_cid" {
		t.Error("CorrelationID not propagated")
	}

	if operation.IsEnabled() {
		t.Error("IsEnabled not propagated")
	}

	if operation.SampleRate() != 99.0 {
		t.Error("Sampling percentage not propagated")
	}

	if operation.Channel() != channel {
		t.Error("Channel not propagated")
	}

	if operation.Correlation() != correlation {
		t.Error("Correlation not propagated")
	}
}

type parsePropertiesTestCase struct {
	str  string
	dict map[string]string
}

func TestParseCorrelationProperties(t *testing.T) {
	testCases := []parsePropertiesTestCase{
		parsePropertiesTestCase{"", map[string]string{}},
		parsePropertiesTestCase{"a=b,c=d", map[string]string{"a": "b", "c": "d"}},
		parsePropertiesTestCase{"=a,b=", map[string]string{"": "a", "b": ""}},
		parsePropertiesTestCase{"abcde", map[string]string{}},
		parsePropertiesTestCase{"abcde,abc=def", map[string]string{"abc": "def"}},
		parsePropertiesTestCase{",", map[string]string{}},
		parsePropertiesTestCase{",,,,", map[string]string{}},
		parsePropertiesTestCase{"=", map[string]string{"": ""}},
		parsePropertiesTestCase{"=,,,,", map[string]string{"": ""}},
		parsePropertiesTestCase{"a,b,c", map[string]string{}},
		parsePropertiesTestCase{"====", map[string]string{"": "==="}},
		parsePropertiesTestCase{"a=1,a=2,a=3", map[string]string{"a": "3"}},
		parsePropertiesTestCase{"  a=b  , c   = d  ", map[string]string{"a": "b", "c": "d"}},
	}

	for _, testCase := range testCases {
		result := ParseCorrelationProperties(testCase.str)
		if result == nil {
			t.Errorf("ParseCorrelationProperties returned nil for input: %s", testCase.str)
			continue
		}

		if len(result) != len(testCase.dict) {
			t.Errorf("ParseCorrelationProperties returned wrong sized map (%d, expected %d) for %s", len(result), len(testCase.dict), testCase.str)
		} else {
			for k, v := range result {
				if expected, ok := testCase.dict[k]; !ok || expected != v {
					t.Errorf("ParseCorrelationProperties {%s: %s} doesn't match expected %s for %s", k, v, expected, testCase.str)
				}
			}
		}
	}
}

type serializePropertiesTestCase struct {
	before map[string]string
	after  map[string]string
}

func TestSerializeCorrelationProperties(t *testing.T) {
	testCases := []serializePropertiesTestCase{
		serializePropertiesTestCase{map[string]string{}, map[string]string{}},
		serializePropertiesTestCase{map[string]string{"a": "b", "c": "d"}, map[string]string{"a": "b", "c": "d"}},
		serializePropertiesTestCase{map[string]string{"": ""}, map[string]string{"": ""}},
		serializePropertiesTestCase{map[string]string{"a": "=", "b": ",", ",": "a", "=": "b"}, map[string]string{}},
		serializePropertiesTestCase{map[string]string{"hello": "worl=d", "w,orld": "hello", "something": "innocuous"}, map[string]string{"something": "innocuous"}},
		serializePropertiesTestCase{map[string]string{"": "hi"}, map[string]string{"": "hi"}},
		serializePropertiesTestCase{map[string]string{"  spaces  ": " aren't ok "}, map[string]string{"spaces": "aren't ok"}},
	}

	for _, testCase := range testCases {
		// Need to use Parse here due to unpredictable ordering :-\
		serialized := CorrelationProperties(testCase.before).Serialize()
		result := ParseCorrelationProperties(serialized)
		if result == nil {
			t.Errorf("ParseCorrelationProperties returned nil for %s", serialized)
			continue
		}

		if len(result) != len(testCase.after) {
			t.Errorf("Unexpected sized map (%d, expected %d) for %v", len(result), len(testCase.after), testCase.before)
		} else {
			for k, v := range result {
				if expected, ok := testCase.after[k]; !ok || expected != v {
					t.Errorf("{%s: %s} doesn't match expected %s for %v", k, v, expected, testCase.before)
				}
			}
		}
	}
}
