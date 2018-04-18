package appinsights

import (
	"math"
	"regexp"
	"strings"
	"testing"
)

// uuid's are always lower-case.
const newIdPattern = `^\|([0-9a-f]{8})(-[0-9a-f]{4}){3}(-[0-9a-f]{12})\.`

func TestNewOperationId(t *testing.T) {
	id := NewOperationId()
	match, err := regexp.MatchString(newIdPattern, string(id))
	if !match || err != nil {
		t.Error("OperationID doesn't match")
	}
}

type getRootTestCase struct {
	id     string
	result string
}

func TestOperationIdGetRoot(t *testing.T) {
	testCases := []getRootTestCase{
		getRootTestCase{"", ""},
		getRootTestCase{"|.", ""},
		getRootTestCase{".", ""},
		getRootTestCase{"|", ""},
		getRootTestCase{"|foo.bar", "foo"},
		getRootTestCase{"|foo.bar.", "foo"},
		getRootTestCase{"foo.bar", "foo"},
		getRootTestCase{"foo|bar.baz", "foo|bar"},
	}

	for _, testCase := range testCases {
		root := OperationId(testCase.id).GetRoot().String()
		if root != testCase.result {
			t.Errorf("Test case failure: GetRoot(%s) == %s != %s", testCase.id, root, testCase.result)
		}
	}
}

type appendSuffixTestCase struct {
	id        string
	suffix    string
	delimiter string
	result    string
}

func TestOperationIdAppendSuffix(t *testing.T) {
	xs := strings.Repeat("x", 2048)

	testCases := []appendSuffixTestCase{
		appendSuffixTestCase{"a", "b", "c", "abc"},
		appendSuffixTestCase{xs[:1022], "b", "c", "x{1000}x{22}bc"},
		appendSuffixTestCase{xs[:1023], "b", "c", newIdPattern},
		appendSuffixTestCase{xs, "a", "b", newIdPattern},
		appendSuffixTestCase{xs[:512] + "." + xs + "_", "b", "_", `x{512}\.[0-9a-f]{8}#`},
		appendSuffixTestCase{xs[:1004] + ".a.b.c.d.e.f.g.h.i.j.k.l", "Y", "_", `x{1000}x{4}\.a\.b\.c\.d\.e\.[0-9a-f]{8}#`},
	}

	for _, testCase := range testCases {
		result := OperationId(testCase.id).AppendSuffix(testCase.suffix, testCase.delimiter).String()
		if match, err := regexp.MatchString("^" + testCase.result + "$", result); !match || err != nil {
			t.Errorf(`"%s".AppendSuffix("%s", "%s") == "%s" doesn't match pattern %s`, testCase.id, testCase.suffix, testCase.delimiter, result, testCase.result)
		}
	}
}

type generateRequestId struct {
	id     string
	result string
}

func TestOperationIdGenerateRequestId(t *testing.T) {
	testCases := []generateRequestId{
		generateRequestId{"", newIdPattern},
		generateRequestId{"foo", `\|foo\.[0-9a-f]+_`},
		generateRequestId{"|foo", `\|foo\.[0-9a-f]+_`},
		generateRequestId{"foo.", `\|foo\.[0-9a-f]+_`},
		generateRequestId{"|foo.", `\|foo\.[0-9a-f]+_`},
	}

	for _, testCase := range testCases {
		result := OperationId(testCase.id).GenerateRequestId().String()
		if match, err := regexp.MatchString("^" + testCase.result + "$", result); !match || err != nil {
			t.Errorf(`"%s".GenerateRequestId() == %s doesn't match %s`, testCase.id, result, testCase.result)
		}
	}
}

type hashTestCase struct {
	id   string
	hash float64
}

func TestOperationIdHash(t *testing.T) {
	testCases := []hashTestCase{
		hashTestCase{"", 0.0},
		hashTestCase{"a", 16.24909},
		hashTestCase{"aa", 16.24909},
		hashTestCase{"aaa", 61.53915},
		hashTestCase{"77bfa0f2-886f-4ed9-a9ed-0e6bbeca5173", 34.34043},
		hashTestCase{"77BFA0F2-886F-4ED9-A9ED-0E6BBECA5173", 49.62479},
		hashTestCase{"02811c0b-5663-4850-9f19-2a875aa524fc", 23.83879},
		hashTestCase{"5bfd603e-f6af-4e8e-846c-2cef14ce7369", 63.91241},
	}

	for _, testCase := range testCases {
		h := OperationId(testCase.id).Hash()
		if math.Abs(h-testCase.hash) > 0.00001 {
			t.Error("Hash test failed for id: " + testCase.id)
		}
	}
}
