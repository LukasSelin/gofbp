package fbp

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/LukasSelin/gofbp/internal/ledger"
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
const registerPath = "testdata/divergences.json"

func loadRegister(t *testing.T) *oracle.Register {
	t.Helper()
	r, err := oracle.Load(registerPath)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// sentinelErrors are the errors a register entry can name, by the name it uses.
var sentinelErrors = map[string]error{
	"ErrC6OnSlope":    ErrC6OnSlope,
	"ErrUnknownFuel":  ErrUnknownFuel,
	"ErrGridFuelCode": ErrGridFuelCode,
}

// gofbpAnswer is this package's number for a row of a cffdrs-wrong entry, by
// entry id. It is what has to equal the entry's Reference.
var gofbpAnswer = map[string]func(get oracle.Getter) float64{
	"tros-degrees-in-radians": func(get oracle.Getter) float64 {
		n := func(c string) float64 { return oracle.Num(get, c) }
		return ROSAtAngle(n("ros"), n("fros"), n("bros"), n("theta")-n("raz"))
	},
	"eq94-misparenthesised": func(get oracle.Getter) float64 {
		n := func(c string) float64 { return oracle.Num(get, c) }
		return ROSAtAngle(n("ros_in"), n("fros_in"), n("bros_in"), n("theta"))
	},
}

var (
	caseFieldsOnce sync.Once
	caseFields     map[string]int
)

// caseGetter reads a cffdrsCase by its fixture column names.
func caseGetter(c cffdrsCase) oracle.Getter {
	caseFieldsOnce.Do(func() {
		caseFields = map[string]int{}
		ty := reflect.TypeOf(cffdrsCase{})
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

// gridGetter reads an fbpGridCase, with NA as nil.
func gridGetter(c fbpGridCase) oracle.Getter {
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

// sectionRows is every row of a fixture section, as getters.
func sectionRows(t *testing.T, f cffdrsFixture, section string) []oracle.Getter {
	t.Helper()
	var rows []oracle.Getter
	switch section {
	case "cases":
		for _, c := range f.Cases {
			rows = append(rows, caseGetter(c))
		}
	case "fbp_grid_cases":
		for _, c := range f.FBPGrid {
			rows = append(rows, gridGetter(c))
		}
	case "theta_cases":
		for _, m := range f.Theta {
			rows = append(rows, oracle.MapGetter(m))
		}
	default:
		t.Fatalf("no loader in package fbp for fixture section %q", section)
	}
	return rows
}

// Every register entry with fixture rows, held to what it says.
//
// For every entry: its scope reaches at least one row. An entry that matches
// nothing is not describing this fixture any more, and its explanation would
// otherwise sit in the register vouching for nothing.
//
// For a cffdrs-wrong entry, three things per row and column, each a different
// finding when it fails:
//
//   - cffdrs computes the entry's Signature. If not, cffdrs has changed what
//     it does here — fixed the defect, most likely — and the verdict is out of
//     date. That is a re-read for a human, not a number to re-record.
//   - gofbp computes the entry's Reference, the published equation. If not,
//     this package is wrong, and the register is not an excuse for it.
//   - Somewhere in scope, cffdrs and the Reference actually disagree. If they
//     never do, there is no divergence left to register.
//
// The refusal verdicts' behaviour is asserted where the refusing entry point is
// — TestCFFDRSPredict and TestCFFDRSPredictGrid — so here they only have to
// reach rows.
//
// ledger: fire_behaviour_prediction.r
func TestCFFDRSDivergences(t *testing.T) {
	f := loadCFFDRS(t)
	if len(f.Theta) == 0 {
		t.Fatal("the fixture has no theta_cases: it predates the divergence register " +
			"(testdata/README.md lists it among the earlier digests). Regenerate it.")
	}
	reg := loadRegister(t)
	const sigTol, refTol = 1e-9, 1e-9

	for i := range reg.Divergences {
		d := &reg.Divergences[i]
		if d.Section == "" {
			continue
		}
		var matched []oracle.Getter
		for _, row := range sectionRows(t, f, d.Section) {
			// Matched through Find, not Matches, so a row an earlier entry claims
			// is that entry's: the counts below are what precedence leaves this one.
			if e := reg.Find(d.Section, row, d.Columns[0]); e == d {
				matched = append(matched, row)
			}
		}
		if len(matched) == 0 {
			t.Errorf("%s: matches no row of %s — the scope no longer describes the fixture", d.ID, d.Section)
			continue
		}
		if d.Verdict != oracle.CFFDRSWrong {
			t.Logf("%-28s %-17s %5d rows of %s", d.ID, d.Verdict, len(matched), d.Section)
			continue
		}

		ours := gofbpAnswer[d.ID]
		if ours == nil {
			t.Errorf("%s: no gofbpAnswer — nothing says what this package computes for it", d.ID)
			continue
		}
		sig, ref := oracle.Formulas[d.Signature], oracle.Formulas[d.Reference]
		for _, col := range d.Columns {
			sigBad, oursBad, undefined, differ := 0, 0, 0, 0
			var worstSig, worstOurs float64
			for _, row := range matched {
				v := oracle.Num(row, col)
				s, r, o := sig(row), ref(row), ours(row)
				worstSig = maxF(worstSig, relErr(s, v))
				if !closeEnough(s, v, sigTol) {
					if sigBad++; sigBad <= 5 {
						t.Errorf("%s: cffdrs %s = %v, but the register's signature %s gives %v. "+
							"cffdrs no longer computes the defect this entry describes: re-read it",
							d.ID, col, v, d.Signature, s)
					}
				}
				if math.IsNaN(r) {
					undefined++
					continue
				}
				if !closeEnough(r, v, 1e-6) {
					differ++
				}
				worstOurs = maxF(worstOurs, relErr(o, r))
				if !closeEnough(o, r, refTol) {
					if oursBad++; oursBad <= 5 {
						t.Errorf("%s: gofbp gives %v where %s (%s) gives %v", d.ID, o, d.Reference, d.Authority, r)
					}
				}
			}
			if differ == 0 {
				t.Errorf("%s: cffdrs' %s agrees with %s on every row — the divergence is gone, "+
					"and the entry with it", d.ID, col, d.Reference)
			}
			t.Logf("%-28s %-17s %5d rows of %s.%s: signature worst %.3g; cffdrs differs from the reference "+
				"in %d; gofbp vs reference worst %.3g (%d where the reference is undefined)",
				d.ID, d.Verdict, len(matched), d.Section, col, worstSig, differ, worstOurs, undefined)
		}
	}
}

// The register against the repository, with no fixture: every name it uses
// has to resolve to something real, and it and MIGRATION.md's known gaps have
// to describe the same set of things.
func TestDivergenceRegisterMatchesTheRepository(t *testing.T) {
	reg := loadRegister(t)
	l, err := ledger.Load(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	rows := l.ByKey()

	tests := map[string]bool{}
	funcDecl := regexp.MustCompile(`(?m)^func (Test\w+)\(`)
	for _, dir := range goPackageDirs {
		files, _ := filepath.Glob(filepath.Join(dir, "*_test.go"))
		for _, p := range files {
			src, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range funcDecl.FindAllStringSubmatch(string(src), -1) {
				tests[m[1]] = true
			}
		}
	}

	gapEntries := map[string][]string{}
	for _, d := range reg.Divergences {
		row, ok := rows[d.Ledger]
		if !ok {
			t.Errorf("%s: ledger %q is not a row of %s", d.ID, d.Ledger, ledgerPath)
		}
		if d.Error != "" && sentinelErrors[d.Error] == nil {
			t.Errorf("%s: error %q is not one of this package's sentinels", d.ID, d.Error)
		}
		for _, name := range d.CheckedBy {
			if !tests[name] {
				t.Errorf("%s: checked_by names %s, which no test file declares", d.ID, name)
			}
		}
		if d.Verdict == oracle.CFFDRSWrong && d.Section != "" && gofbpAnswer[d.ID] == nil {
			t.Errorf("%s: no gofbpAnswer for a cffdrs-wrong entry with fixture rows", d.ID)
		}
		switch {
		case d.Gap != "":
			gapEntries[d.Gap] = append(gapEntries[d.Gap], d.ID)
		case d.Verdict == oracle.PinnedSemantics:
			// gofbp gives the pinned oracle's answer, so nothing inside a ✅ row
			// disagrees with it.
		case ok && row.Status == ledger.Ported:
			t.Errorf("%s: names no gap, but its row %s is ✅ — a divergence inside a ✅ row "+
				"belongs in MIGRATION.md's known gaps", d.ID, d.Ledger)
		}
	}
	for id := range gofbpAnswer {
		if reg.ByID(id) == nil {
			t.Errorf("gofbpAnswer has %q, which the register does not", id)
		}
	}

	bullets := map[string]bool{}
	for _, g := range l.KnownGaps {
		bullets[g.Title] = true
		if len(gapEntries[g.Title]) == 0 {
			t.Errorf("%s:%d: known gap %q has no entry in %s", ledgerPath, g.Line, g.Title, registerPath)
		}
	}
	gaps := make([]string, 0, len(gapEntries))
	for g := range gapEntries {
		gaps = append(gaps, g)
	}
	sort.Strings(gaps)
	for _, g := range gaps {
		if !bullets[g] {
			t.Errorf("%v name gap %q, which is not a bullet of MIGRATION.md's known gaps", gapEntries[g], g)
		}
	}

	// The sentinels have to be distinct, or naming one says nothing.
	for a, ea := range sentinelErrors {
		for b, eb := range sentinelErrors {
			if a != b && errors.Is(ea, eb) {
				t.Errorf("%s and %s are the same error", a, b)
			}
		}
	}
}
