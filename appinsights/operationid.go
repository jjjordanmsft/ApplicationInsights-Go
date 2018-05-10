package appinsights

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/satori/go.uuid"
)

// The longest allowed operation ID
const maxOperationIdLength = 1024

// Monotonically incrementing counter used for appending to operation IDs.
var currentRootId uint32

func init() {
	currentRootId = rand.Uint32()
}

// OperationId is a specially-formatted string that identifies an operation.
type OperationId string

// NewOperationId creates a new, random Operation ID.
func NewOperationId() OperationId {
	return OperationId("|" + uuid.NewV4().String() + ".")
}

// String returns the OperationId as a string
func (id OperationId) String() string {
	return string(id)
}

// GetRoot returns the root OperationId
func (id OperationId) GetRoot() OperationId {
	idstr := string(id)
	end := strings.IndexByte(idstr, '.')
	if end < 0 {
		end = len(idstr)
	}

	if len(idstr) > 0 && idstr[0] == '|' {
		return OperationId(idstr[1:end])
	} else {
		return OperationId(idstr[:end])
	}
}

// AppendSuffix appends a suffix and delimiter to id, creating a new ID if
// empty.
func (id OperationId) AppendSuffix(suffix, delimiter string) OperationId {
	idstr := string(id)
	if (len(idstr) + len(suffix) + len(delimiter)) <= maxOperationIdLength {
		return OperationId(idstr + suffix + delimiter)
	}

	// Combined id too long; we need 9 characters of space: 8 for the
	// overflow ID and 1 for the overflow delimiter '#'.
	x := maxOperationIdLength - 9
	if len(idstr) > x {
		for ; x > 1; x-- {
			c := idstr[x-1]
			if c == '.' || c == '_' {
				break
			}
		}
	}

	if x <= 1 {
		return NewOperationId()
	}

	return OperationId(fmt.Sprintf("%s%08x#", idstr[:x], rand.Uint32()))
}

// Generates a request ID parented on the specified ID.
func (id OperationId) GenerateRequestId() OperationId {
	idstr := string(id)
	if idstr != "" {
		if idstr[0] != '|' {
			idstr = "|" + idstr
		}
		if idstr[len(idstr)-1] != '.' {
			idstr += "."
		}

		return OperationId(idstr).AppendSuffix(nextRootId(), "_")
	} else {
		return NewOperationId()
	}
}

// Hash returns a float64 hashcode between 0.0 and 100.0, and is used for
// sampling.
func (id OperationId) Hash() float64 {
	idstr := string(id)
	if idstr == "" {
		return 0.0
	}

	for len(idstr) < 8 {
		idstr += idstr
	}

	// djb2, with + not ^
	var hash int32 = 5381
	for _, c := range idstr {
		hash = (hash << 5) + hash + int32(c)
	}

	if hash == math.MinInt32 {
		hash = math.MaxInt32
	}

	return (math.Abs(float64(hash)) / float64(math.MaxInt32)) * 100.0
}

// Returns a monotonically increasing root ID.
func nextRootId() string {
	value := atomic.AddUint32(&currentRootId, 1)
	return strconv.FormatUint(uint64(value), 16)
}
