package cffdrstest

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/LukasSelin/gofbp/internal/oracle"
)

// The divergence register, testdata/divergences.json, is what the oracle knows
// on top of the fixture: every place this package deliberately does not give
// cffdrs' answer, and which side is right. See internal/oracle.
//
// The TestCFFDRS* tests consult it instead of excluding rows by name. A row the
// register does not cover must match cffdrs; a row it does is held to what the
// entry says. So a disagreement nobody has written down fails, and one somebody
// has is checked against its own explanation rather than skipped.
//
// RegisterPath is its path from the repository root.
const RegisterPath = "testdata/divergences.json"

// LoadRegister reads the divergence register at RegisterPath.
func LoadRegister(t *testing.T) *oracle.Register {
	t.Helper()
	r, err := oracle.Load(RepoPath(t, RegisterPath))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

var (
	caseFieldsOnce sync.Once
	caseFields     map[string]int
)

// CaseGetter reads a Case by its fixture column names.
func CaseGetter(c Case) oracle.Getter {
	caseFieldsOnce.Do(func() {
		caseFields = map[string]int{}
		ty := reflect.TypeOf(Case{})
		for i := 0; i < ty.NumField(); i++ {
			if tag := strings.Split(ty.Field(i).Tag.Get("json"), ",")[0]; tag != "" {
				caseFields[tag] = i
			}
		}
	})
	v := reflect.ValueOf(c)
	return func(col string) (any, bool) {
		i, ok := caseFields[col]
		if !ok {
			return nil, false
		}
		return v.Field(i).Interface(), true
	}
}

// GridGetter reads a GridCase, with NA as nil.
func GridGetter(c GridCase) oracle.Getter {
	return func(col string) (any, bool) {
		switch col {
		case "grid":
			return c.Grid, true
		case "cell":
			return float64(c.Cell), true
		}
		p, ok := c.v[col]
		if !ok || p == nil {
			return nil, ok
		}
		return *p, true
	}
}
