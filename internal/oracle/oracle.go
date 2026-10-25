// Package oracle reads testdata/divergences.json, the register of every place
// gofbp deliberately does not give cffdrs' answer.
//
// The fixture says what cffdrs computes. On its own that makes cffdrs the
// definition of right, which it is not everywhere: its rates at a bearing are
// both wrong, and in a handful of places gofbp refuses where cffdrs fills a
// default in. Those used to be known only to the prose — exclusions by name in
// the TestCFFDRS* tests, notes in MIGRATION.md — so a disagreement nobody had
// written down and one somebody had looked at failed or passed the same way.
//
// The register is what the oracle knows on top of the fixture. Each entry names
// the rows it covers, what cffdrs does there, what gofbp does, and which is
// correct on what authority. For a cffdrs defect it also names two formulas
// kept here: the Signature, which reproduces cffdrs' wrong number, and the
// Reference, which is the published equation. The tests hold an entry both
// ways — cffdrs must still compute its Signature, and gofbp must compute the
// Reference — so a cffdrs that fixes a defect fails the test instead of quietly
// changing what this package departs from, and the verdict goes back to a human.
//
// The formulas take nothing from package fbp. They are independent of it on
// purpose: the Reference is what fbp is checked against, and the package that
// checks it cannot be the one it checks.
//
// Stdlib only.
package oracle

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
)

// Verdict is what an entry says about who is right.
type Verdict string

const (
	// CFFDRSWrong: cffdrs' number is wrong, and Correct and Authority say what
	// is right instead. gofbp computes the right one, or nothing.
	CFFDRSWrong Verdict = "cffdrs-wrong"
	// GoFBPRefuses: cffdrs answers and gofbp returns Error. Correct says why no
	// answer is better than cffdrs'.
	GoFBPRefuses Verdict = "gofbp-refuses"
	// GoFBPMayRefuse: cffdrs substitutes a default, and gofbp either refuses or
	// computes the same number cffdrs did. Either is fine; a third answer is not.
	GoFBPMayRefuse Verdict = "gofbp-may-refuse"
	// Unsupported: a cffdrs input gofbp has no counterpart for. No number
	// disagrees, so no fixture row is in scope.
	Unsupported Verdict = "unsupported"
	// PinnedSemantics: the pinned cffdrs and upstream HEAD differ, and gofbp
	// follows the pin.
	PinnedSemantics Verdict = "pinned-semantics"
)

var verdicts = map[Verdict]bool{
	CFFDRSWrong: true, GoFBPRefuses: true, GoFBPMayRefuse: true, Unsupported: true, PinnedSemantics: true,
}

// Sections are the fixture's top-level arrays an entry can scope itself to.
var Sections = []string{"cases", "fwi_cases", "fbp_grid_cases", "theta_cases"}

// Cond is a test on one column. Every field set must hold. A row that does not
// carry the column, or carries null or NaN, is null: it satisfies Null and
// nothing else.
type Cond struct {
	In    []any    `json:"in,omitempty"`
	NotIn []any    `json:"not_in,omitempty"`
	Gt    *float64 `json:"gt,omitempty"`
	Ge    *float64 `json:"ge,omitempty"`
	Lt    *float64 `json:"lt,omitempty"`
	Le    *float64 `json:"le,omitempty"`
	Null  bool     `json:"null,omitempty"`
}

// Clause is every Cond on its column holding at once.
type Clause map[string]Cond

// Divergence is one entry of the register.
type Divergence struct {
	ID      string  `json:"id"`
	Verdict Verdict `json:"verdict"`
	// Section, When and Columns are the entry's scope in the fixture: the rows
	// of Section matching any one of When's clauses, and of those rows, Columns
	// ("*" for all of them). Empty Section means no fixture row reaches it.
	Section string   `json:"section,omitempty"`
	When    []Clause `json:"when,omitempty"`
	Columns []string `json:"columns,omitempty"`

	CFFDRS    string `json:"cffdrs"`
	GoFBP     string `json:"gofbp"`
	Correct   string `json:"correct"`
	Authority string `json:"authority,omitempty"`

	// Signature and Reference name Formulas. Required for a CFFDRSWrong entry
	// with fixture rows, and only there.
	Signature string `json:"signature,omitempty"`
	Reference string `json:"reference,omitempty"`
	// Error is the gofbp sentinel a GoFBPRefuses entry's rows must be refused
	// with, by name.
	Error string `json:"error,omitempty"`

	// Gap is the bold title of the MIGRATION.md "Known gaps inside ✅ rows"
	// bullet this entry is, and Ledger the row it belongs to.
	Gap    string `json:"gap,omitempty"`
	Ledger string `json:"ledger"`
	// CheckedBy are the tests that hold the entry. HeldByGenerator says so when
	// the generator itself asserts cffdrs' side.
	CheckedBy       []string `json:"checked_by,omitempty"`
	HeldByGenerator string   `json:"held_by_generator,omitempty"`
}

// Register is the whole file.
type Register struct {
	Note        string       `json:"note"`
	Divergences []Divergence `json:"divergences"`
}

// Load reads and validates a register.
func Load(path string) (*Register, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Register
	dec := json.NewDecoder(bytesReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &r, nil
}

var idPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Validate checks the register's shape: everything that can be wrong with an
// entry without reading the fixture or the Go.
func (r *Register) Validate() error {
	if len(r.Divergences) == 0 {
		return fmt.Errorf("no divergences — an empty register would make every check over it pass")
	}
	seen := map[string]bool{}
	section := map[string]bool{}
	for _, s := range Sections {
		section[s] = true
	}
	for i := range r.Divergences {
		d := &r.Divergences[i]
		at := fmt.Sprintf("divergence %d (%q)", i, d.ID)
		switch {
		case !idPattern.MatchString(d.ID):
			return fmt.Errorf("%s: id must be kebab-case", at)
		case seen[d.ID]:
			return fmt.Errorf("%s: duplicate id", at)
		case !verdicts[d.Verdict]:
			return fmt.Errorf("%s: unknown verdict %q", at, d.Verdict)
		case d.CFFDRS == "" || d.GoFBP == "" || d.Correct == "" || d.Ledger == "":
			return fmt.Errorf("%s: cffdrs, gofbp, correct and ledger are all required", at)
		case d.Section != "" && !section[d.Section]:
			return fmt.Errorf("%s: unknown section %q", at, d.Section)
		case (d.Section == "") != (len(d.When) == 0) || (d.Section == "") != (len(d.Columns) == 0):
			return fmt.Errorf("%s: section, when and columns go together", at)
		case d.Verdict == CFFDRSWrong && d.Authority == "":
			return fmt.Errorf("%s: a cffdrs-wrong verdict needs the authority it rests on", at)
		case d.Verdict == GoFBPRefuses && d.Error == "":
			return fmt.Errorf("%s: a gofbp-refuses verdict needs the error gofbp refuses with", at)
		case (d.Verdict == Unsupported || d.Verdict == PinnedSemantics) && d.Section != "":
			return fmt.Errorf("%s: %s disagrees about no fixture number, so it has no section", at, d.Verdict)
		case d.Verdict != Unsupported && len(d.CheckedBy) == 0:
			return fmt.Errorf("%s: checked_by is empty — nothing holds this entry", at)
		}
		formulas := d.Verdict == CFFDRSWrong && d.Section != ""
		if formulas != (d.Signature != "") || formulas != (d.Reference != "") {
			return fmt.Errorf("%s: signature and reference are required for a cffdrs-wrong entry "+
				"with fixture rows, and meaningless anywhere else", at)
		}
		for _, name := range []string{d.Signature, d.Reference} {
			if name != "" && Formulas[name] == nil {
				return fmt.Errorf("%s: no formula %q in package oracle", at, name)
			}
		}
		seen[d.ID] = true
	}
	return nil
}

// Getter reads one column of a row. ok is false when the row does not carry it.
type Getter func(col string) (v any, ok bool)

// MapGetter reads a row decoded from JSON.
func MapGetter(m map[string]any) Getter {
	return func(col string) (any, bool) { v, ok := m[col]; return v, ok }
}

// Matches reports whether the row is in d's scope: any one clause holding.
func (d *Divergence) Matches(get Getter) bool {
	for _, c := range d.When {
		if c.holds(get) {
			return true
		}
	}
	return false
}

// Covers reports whether d's scope includes the column.
func (d *Divergence) Covers(col string) bool {
	for _, c := range d.Columns {
		if c == "*" || c == col {
			return true
		}
	}
	return false
}

func (c Clause) holds(get Getter) bool {
	for col, cond := range c {
		if !cond.holds(get(col)) {
			return false
		}
	}
	return true
}

func (c Cond) holds(v any, ok bool) bool {
	f, isNum := v.(float64)
	null := !ok || v == nil || (isNum && math.IsNaN(f))
	if c.Null {
		return null
	}
	if null {
		return false
	}
	if c.In != nil && !contains(c.In, v) {
		return false
	}
	if c.NotIn != nil && contains(c.NotIn, v) {
		return false
	}
	for _, b := range []struct {
		bound *float64
		ok    func(x, b float64) bool
	}{
		{c.Gt, func(x, b float64) bool { return x > b }},
		{c.Ge, func(x, b float64) bool { return x >= b }},
		{c.Lt, func(x, b float64) bool { return x < b }},
		{c.Le, func(x, b float64) bool { return x <= b }},
	} {
		if b.bound != nil && (!isNum || !b.ok(f, *b.bound)) {
			return false
		}
	}
	return true
}

func contains(list []any, v any) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Find returns the first entry, in file order, whose scope in section takes in
// this row and column, or nil. File order is precedence.
func (r *Register) Find(section string, get Getter, col string) *Divergence {
	for i := range r.Divergences {
		d := &r.Divergences[i]
		if d.Section == section && d.Covers(col) && d.Matches(get) {
			return d
		}
	}
	return nil
}

// ByID returns the entry with this id, or nil.
func (r *Register) ByID(id string) *Divergence {
	for i := range r.Divergences {
		if r.Divergences[i].ID == id {
			return &r.Divergences[i]
		}
	}
	return nil
}
