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

const correlationRequestMaxLength = 1024

var currentRootId uint32

func init() {
	currentRootId = rand.Uint32()
}

type OperationId string

func NewOperationId() OperationId {
	return OperationId("|" + uuid.NewV4().String() + ".")
}

func (id OperationId) GetRoot() string {
	idstr := string(id)
	end := strings.IndexByte(idstr, '.')
	if end < 0 {
		end = len(idstr)
	}

	if idstr[0] == '|' {
		return idstr[1:end]
	} else {
		return idstr[:end]
	}
}

func (id OperationId) AppendSuffix(suffix, delimiter string) OperationId {
	idstr := string(id)
	if (len(idstr) + len(suffix) + len(delimiter)) <= correlationRequestMaxLength {
		return OperationId(idstr + suffix + delimiter)
	}

	// Combined id too long; we need 9 characters of space: 8 for the
	// overflow ID and 1 for the overflow delimiter '#'.
	x := correlationRequestMaxLength - 9
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

	return OperationId(fmt.Sprintf("%s%08ux#", idstr[:x], rand.Uint32()))
}

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

func (id OperationId) Hash() float64 {
	idstr := string(id)
	if idstr == "" {
		return 0.0
	}

	for len(idstr) < 8 {
		id += id
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

func nextRootId() string {
	value := atomic.AddUint32(&currentRootId, 1)
	return strconv.FormatUint(uint64(value), 16)
}
