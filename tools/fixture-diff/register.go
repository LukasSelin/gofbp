package main

// What fixture-diff knows from testdata/divergences.json, the register of every
// place gofbp deliberately does not give cffdrs' answer (see internal/oracle).
//
// Two things, and they answer different questions about a regeneration.
//
// attribute says, of each moved case, whether it sits inside a registered
// divergence. A move outside every entry is the ordinary finding: cffdrs changed
// a number gofbp is meant to reproduce. A move inside one is cffdrs changing a
// number somebody already looked at and adjudicated, so the thing to re-read is
// that entry's verdict, not the Go.
//
// checkSignatures asks of the new fixture alone whether cffdrs still computes
// each defect the register describes. A move can be a defect being fixed
// upstream, and this is what tells those apart from a move inside a defect
// that is still there.

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/LukasSelin/gofbp/internal/oracle"
)

// sigStat is one cffdrs-wrong entry checked against the new fixture.
type sigStat struct {
	ID     string `json:"id"`
	Rows   int    `json:"rows"`
	Broken int    `json:"broken"`
	// Example is the worst row where cffdrs no longer computes the signature.
	Example string `json:"example,omitempty"`
}

// signatureTol is how close cffdrs' number must be to the entry's Signature.
// The two are the same arithmetic in two languages, so this is a few ulps of
// libm difference and nothing else — the tests use the same bound.
const signatureTol = 1e-9

// loadRegister reads the register named by flagVal: "none" for no register,
// empty for divergences.json beside the new fixture if there is one.
func loadRegister(flagVal, newPath string) (*oracle.Register, error) {
	switch flagVal {
	case "none":
		return nil, nil
	case "":
		p := filepath.Join(filepath.Dir(newPath), "divergences.json")
		if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		flagVal = p
	}
	return oracle.Load(flagVal)
}

// attribute fills in, for every moved column of every section, how many of its
// moved cases each register entry covers.
func attribute(rep *report, reg *oracle.Register) {
	for _, s := range append([]*report{rep}, rep.sides()...) {
		for i := range s.Columns {
			c := &s.Columns[i]
			if len(c.movedKeys) == 0 {
				continue
			}
			c.Divergences = map[string]int{}
			for _, k := range c.movedKeys {
				if d := reg.Find(s.Section, oracle.MapGetter(s.newIdx[k]), c.Column); d != nil {
					c.Divergences[d.ID]++
				} else {
					c.Unexplained++
				}
			}
		}
	}
}

// checkSignatures evaluates every cffdrs-wrong entry's Signature over the rows
// of the new fixture it covers.
func checkSignatures(fx *fixture, reg *oracle.Register) []sigStat {
	sections := map[string][]map[string]any{
		"cases": fx.Cases, "fwi_cases": fx.FWICases,
		"fbp_grid_cases": fx.FBPGridCases, "theta_cases": fx.ThetaCases,
	}
	var out []sigStat
	for i := range reg.Divergences {
		d := &reg.Divergences[i]
		if d.Signature == "" {
			continue
		}
		st := sigStat{ID: d.ID}
		sig := oracle.Formulas[d.Signature]
		worst := 0.0
		for _, m := range sections[d.Section] {
			row := oracle.MapGetter(m)
			if !d.Matches(row) {
				continue
			}
			st.Rows++
			for _, col := range d.Columns {
				want, got := sig(row), oracle.Num(row, col)
				rel := math.Abs(got-want) / math.Max(1, math.Abs(want))
				if rel <= signatureTol || (math.IsNaN(rel) && math.IsNaN(got) && math.IsNaN(want)) {
					continue
				}
				st.Broken++
				if math.IsNaN(rel) || rel > worst {
					worst = rel
					st.Example = fmt.Sprintf("%s %s = %.17g, signature %s gives %.17g", caseKey(m, rowKey(m)), col, got, d.Signature, want)
				}
			}
		}
		out = append(out, st)
	}
	return out
}

// rowKey is the columns caseKey should print for an example: every one the row
// carries that is an input of some section, which is enough to find it again.
func rowKey(m map[string]any) []string {
	var cols []string
	for _, list := range [][]string{inputCols, fwiInputCols, fbpGridInputCols, thetaInputCols} {
		for _, c := range list {
			if _, ok := m[c]; ok {
				cols = append(cols, c)
			}
		}
	}
	return dedupe(cols)
}

func dedupe(s []string) []string {
	seen := map[string]bool{}
	out := s[:0]
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func printSignatures(w io.Writer, sigs []sigStat) {
	if len(sigs) == 0 {
		return
	}
	p := func(f string, a ...any) { _, _ = fmt.Fprintf(w, f, a...) }
	p("\n=== the divergence register's known cffdrs defects, in the new fixture ===\n\n")
	for _, s := range sigs {
		switch {
		case s.Rows == 0:
			p("%-28s no rows in scope — the new fixture does not record it\n", s.ID)
		case s.Broken == 0:
			p("%-28s still computed by cffdrs in all %d rows\n", s.ID, s.Rows)
		default:
			p("!! %-25s cffdrs no longer computes it in %d values of %d rows: re-read the entry.\n", s.ID, s.Broken, s.Rows)
			p("!!   %s\n", s.Example)
		}
	}
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
