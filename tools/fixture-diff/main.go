// Command fixture-diff compares two cffdrs oracle fixtures column by column and
// says, per column, whether the reference numbers moved.
//
// This exists because DAILY-CHECK.md and /migration-port both end in the same
// instruction — "read the diff in the reference numbers" — over a file with
// ~18400 cases and 27 columns. Read by eye that is not a check, it is a hope.
// Read by this it is an assertion: adding a column must move nothing, and a
// version bump that moves something must say which something.
//
// Usage:
//
//	fixture-diff [options] old.json new.json
//
//	-tol float     relative tolerance below which a difference is not a move
//	               (default 0 — any difference at all is a move)
//	-json          emit the report as JSON instead of a table
//	-examples n    worst-case examples to print per moved column (default 3)
//
// Exit status is 0 when no column shared by both fixtures moved, 1 when one did,
// and 2 on a usage or I/O error. The 1 is the point: this is meant to be run in
// the middle of a regeneration and believed, not read afterwards.
//
// Stdlib only, like everything else here.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

// inputCols are the sweep's inputs: the columns gen_cffdrs_reference.R copies
// from `inp` rather than reading back out of fbp(). Together they identify a
// case, which is what makes two fixtures comparable at all — the sweep's row
// ORDER is not a contract, and a widened sweep reorders it.
//
// A column not listed here is treated as an output and compared. That default is
// deliberate: a new output column shows up as "new", which is the expected result
// of a port, while a new INPUT column would silently make keys ambiguous — so
// duplicate keys are detected and reported rather than papered over.
//
// long, elv and d0 joined this list when the FMC block was added to the sweep.
// They were constants in the generator before that, so a fixture older than that
// block carries no such column and they cannot participate in the key for a diff
// against one — which is exactly the case the "present in BOTH fixtures" rule
// below handles, and why that block gives every one of its sites a distinct
// latitude.
var inputCols = []string{
	"fuel", "ffmc", "bui", "ws", "wd", "gs", "pc", "pdf", "cc", "cbh", "cfl",
	"lat", "long", "elv", "dj", "d0",
}

// fwiInputCols identify a case in the FWI System's section, "fwi_cases" — a
// second top-level array that package fwi's TestCFFDRS* read, beside the FBP
// "cases" above. The generator names inputs apart from outputs on purpose (the
// ISI grid's FFMC is ffmc_in, because ffmc is what the FFMC rows output), so a
// column is an input here if and only if it is in this list. kind, chain and
// day are what make a multi-day chain's rows distinct from each other: the same
// weather on two days of one chain is two cases, not a duplicate.
//
// A row carries only the columns its kind uses; the rest are absent, which
// keys as null on both sides and so compares equal. That is what lets one list
// serve every kind.
var fwiInputCols = []string{
	"kind", "chain", "day", "lat_adjust", "mon", "lat",
	"temp", "rh", "ws", "prec",
	"ffmc_yda", "dmc_yda", "dc_yda",
	"ffmc_in", "dmc_in", "dc_in", "isi_in", "bui_in",
}

type fixture struct {
	Oracle        string           `json:"oracle"`
	CFFDRSVersion string           `json:"cffdrs_version"`
	RVersion      string           `json:"r_version"`
	Cases         []map[string]any `json:"cases"`
	// FWICases is the FWI System's section. Absent from every fixture older than
	// the 2026-09-24 FWI block, which is a new section rather than a lost one.
	FWICases []map[string]any `json:"fwi_cases"`

	path string
}

// colStat is one column's verdict.
type colStat struct {
	Column   string  `json:"column"`
	Status   string  `json:"status"` // unchanged | moved | new | removed | key
	Compared int     `json:"compared"`
	Moved    int     `json:"moved"`
	MaxAbs   float64 `json:"max_abs,omitempty"`
	MaxRel   float64 `json:"max_rel,omitempty"`
	// NullFlips counts cases where one side is null — a non-finite oracle result —
	// and the other is a number. Those cannot be expressed as a delta, and they are
	// the most interesting kind of move, so they are counted separately.
	NullFlips int      `json:"null_flips,omitempty"`
	Examples  []string `json:"examples,omitempty"`
}

type report struct {
	// Section is which top-level array this report compares: "cases" (FBP) or
	// "fwi_cases" (the FWI System). The FBP report is the top level and carries
	// the FWI one in FWI, so a caller reading the old JSON shape still finds FBP
	// where it always was.
	Section      string    `json:"section"`
	Old          string    `json:"old"`
	New          string    `json:"new"`
	OldVersions  string    `json:"old_versions"`
	NewVersions  string    `json:"new_versions"`
	OldCases     int       `json:"old_cases"`
	NewCases     int       `json:"new_cases"`
	SharedCases  int       `json:"shared_cases"`
	OnlyInOld    int       `json:"only_in_old"`
	OnlyInNew    int       `json:"only_in_new"`
	AmbiguousOld int       `json:"ambiguous_old"`
	AmbiguousNew int       `json:"ambiguous_new"`
	DuplicateOld int       `json:"duplicate_old"`
	DuplicateNew int       `json:"duplicate_new"`
	KeyCols      []string  `json:"key_cols"`
	Columns      []colStat `json:"columns"`
	Moved        []string  `json:"moved"`

	// NewSection is true when only the new fixture has this section at all:
	// there is nothing to compare, and nothing can have moved.
	NewSection bool    `json:"new_section,omitempty"`
	FWI        *report `json:"fwi,omitempty"`
}

// anyMoved reports whether anything moved in this section or the one it carries.
// It is what the exit status is taken from.
func (r *report) anyMoved() bool {
	return len(r.Moved) > 0 || (r.FWI != nil && r.FWI.anyMoved())
}

func main() {
	tol := flag.Float64("tol", 0, "relative tolerance below which a difference is not a move")
	asJSON := flag.Bool("json", false, "emit the report as JSON")
	examples := flag.Int("examples", 3, "worst-case examples to print per moved column")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: fixture-diff [options] old.json new.json\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(2)
	}

	oldFx, err := load(flag.Arg(0))
	if err != nil {
		fatal(err)
	}
	newFx, err := load(flag.Arg(1))
	if err != nil {
		fatal(err)
	}

	rep := compare(oldFx, newFx, *tol, *examples)

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fatal(err)
		}
	} else {
		printReport(os.Stdout, rep)
	}
	if rep.anyMoved() {
		os.Exit(1)
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "fixture-diff: %v\n", err)
	os.Exit(2)
}

func load(path string) (*fixture, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f fixture
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(f.Cases) == 0 {
		return nil, fmt.Errorf("%s: no cases", path)
	}
	f.path = path
	return &f, nil
}

func (f *fixture) versions() string {
	return fmt.Sprintf("cffdrs %s, %s", f.CFFDRSVersion, f.RVersion)
}

// columns returns every key seen across a section's cases, not just the first
// one. A generator that emitted a column conditionally would otherwise vanish.
func columns(cases []map[string]any) map[string]bool {
	cols := map[string]bool{}
	for _, c := range cases {
		for k := range c {
			cols[k] = true
		}
	}
	return cols
}

// index keys every case by its input columns and reports what collided.
//
// Two cases sharing a key are one of two very different things, and conflating
// them cost a real investigation once. If their OTHER columns agree, the sweep
// simply emits a row twice — the generator's slope block repeats the flat block
// at gs = 0 — which is harmless here, because dropping a row identical to the one
// kept cannot change a verdict. If they DISAGREE, the key no longer identifies a
// case: some input the sweep varies is missing from inputCols, and every
// comparison below is untrustworthy.
func index(cases []map[string]any, keyCols []string) (idx map[string]map[string]any, identical, conflicting int) {
	idx = make(map[string]map[string]any, len(cases))
	for _, c := range cases {
		k := caseKey(c, keyCols)
		kept, seen := idx[k]
		if !seen {
			idx[k] = c
			continue
		}
		if sameCase(kept, c) {
			identical++
		} else {
			conflicting++
		}
	}
	return idx, identical, conflicting
}

// sameCase reports whether two cases agree on every column either one has.
func sameCase(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || format(av) != format(bv) {
			return false
		}
	}
	return true
}

func caseKey(c map[string]any, keyCols []string) string {
	var b strings.Builder
	for i, col := range keyCols {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteString(col)
		b.WriteByte('=')
		b.WriteString(format(c[col]))
	}
	return b.String()
}

// format renders a JSON value the same way on both sides. Numbers go through
// strconv with -1 precision, which round-trips a float64 exactly, so two equal
// numbers never key differently.
func format(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprint(t)
	}
}

// compare diffs two fixtures: the FBP section at the top level, and the FWI
// System's section inside it whenever either fixture has one.
func compare(oldFx, newFx *fixture, tol float64, maxExamples int) *report {
	rep := compareSection("cases", oldFx.Cases, newFx.Cases, inputCols, tol, maxExamples)
	rep.Old, rep.New = oldFx.path, newFx.path
	rep.OldVersions, rep.NewVersions = oldFx.versions(), newFx.versions()

	switch {
	case len(oldFx.FWICases) == 0 && len(newFx.FWICases) == 0:
		// A pre-FWI pair: nothing to say, and saying nothing keeps the report
		// for such a pair exactly what it always was.
	case len(oldFx.FWICases) == 0:
		rep.FWI = &report{Section: "fwi_cases", NewSection: true, NewCases: len(newFx.FWICases),
			OnlyInNew: len(newFx.FWICases)}
	default:
		// Including the case where only the OLD fixture has the section: every
		// column comes back "removed", which is a move — the FWI tests are about
		// to start failing, and a diff that shrugged would be wrong.
		rep.FWI = compareSection("fwi_cases", oldFx.FWICases, newFx.FWICases, fwiInputCols, tol, maxExamples)
	}
	return rep
}

func compareSection(section string, oldCases, newCases []map[string]any, inputs []string, tol float64, maxExamples int) *report {
	oldCols, newCols := columns(oldCases), columns(newCases)

	// Key on the input columns present in BOTH fixtures. An input column that
	// exists on only one side cannot participate, and if it varies it announces
	// itself as ambiguous keys below.
	var keyCols []string
	for _, c := range inputs {
		if oldCols[c] && newCols[c] {
			keyCols = append(keyCols, c)
		}
	}

	oldIdx, oldDupes, oldConflicts := index(oldCases, keyCols)
	newIdx, newDupes, newConflicts := index(newCases, keyCols)

	shared := make([]string, 0, len(oldIdx))
	onlyOld := 0
	for k := range oldIdx {
		if _, ok := newIdx[k]; ok {
			shared = append(shared, k)
		} else {
			onlyOld++
		}
	}
	sort.Strings(shared)
	onlyNew := 0
	for k := range newIdx {
		if _, ok := oldIdx[k]; !ok {
			onlyNew++
		}
	}

	rep := &report{
		Section:  section,
		OldCases: len(oldCases), NewCases: len(newCases),
		SharedCases: len(shared), OnlyInOld: onlyOld, OnlyInNew: onlyNew,
		AmbiguousOld: oldConflicts, AmbiguousNew: newConflicts,
		DuplicateOld: oldDupes, DuplicateNew: newDupes,
		KeyCols: keyCols,
	}

	// Every column either side has, key columns included — an input column that
	// appears or disappears is a change to the sweep and belongs in the report.
	all := map[string]bool{}
	for c := range oldCols {
		all[c] = true
	}
	for c := range newCols {
		all[c] = true
	}
	names := make([]string, 0, len(all))
	for c := range all {
		names = append(names, c)
	}
	sort.Strings(names)

	isKey := map[string]bool{}
	for _, c := range keyCols {
		isKey[c] = true
	}

	for _, col := range names {
		switch {
		case !oldCols[col]:
			rep.Columns = append(rep.Columns, colStat{Column: col, Status: "new"})
		case !newCols[col]:
			// A column that stopped being emitted is a lost assertion, which is the
			// same severity as a moved one: a TestCFFDRS* is about to start skipping
			// its rows rather than failing.
			rep.Columns = append(rep.Columns, colStat{Column: col, Status: "removed"})
			rep.Moved = append(rep.Moved, col)
		case isKey[col]:
			rep.Columns = append(rep.Columns, colStat{Column: col, Status: "key", Compared: len(shared)})
		default:
			rep.Columns = append(rep.Columns, compareCol(col, shared, oldIdx, newIdx, tol, maxExamples, rep))
		}
	}
	return rep
}

func compareCol(col string, shared []string, oldIdx, newIdx map[string]map[string]any, tol float64, maxExamples int, rep *report) colStat {
	st := colStat{Column: col, Status: "unchanged", Compared: len(shared)}

	// Worst offenders, kept sorted by absolute delta so the examples printed are
	// the ones worth looking at rather than the first three encountered.
	type ex struct {
		key      string
		what     string
		absDelta float64
	}
	var worst []ex

	for _, k := range shared {
		a, b := oldIdx[k][col], newIdx[k][col]
		af, aNum := a.(float64)
		bf, bNum := b.(float64)

		switch {
		case aNum && bNum:
			if af == bf {
				continue
			}
			d := math.Abs(af - bf)
			rel := d / math.Max(1, math.Abs(af))
			if rel <= tol {
				continue
			}
			st.Moved++
			st.MaxAbs = math.Max(st.MaxAbs, d)
			st.MaxRel = math.Max(st.MaxRel, rel)
			worst = append(worst, ex{k, fmt.Sprintf("%.17g -> %.17g", af, bf), d})
		case a == nil && b == nil:
			continue
		case aNum != bNum:
			// number <-> null: the non-finite flip. No delta to report, and it sorts
			// to the top of the examples because it is the biggest kind of move there
			// is — the oracle started or stopped having an answer at all.
			st.Moved++
			st.NullFlips++
			worst = append(worst, ex{k, format(a) + " -> " + format(b), math.Inf(1)})
		default:
			if format(a) == format(b) {
				continue
			}
			st.Moved++
			worst = append(worst, ex{k, format(a) + " -> " + format(b), math.Inf(1)})
		}
	}

	if st.Moved > 0 {
		st.Status = "moved"
		rep.Moved = append(rep.Moved, col)
		sort.SliceStable(worst, func(i, j int) bool { return worst[i].absDelta > worst[j].absDelta })
		for i, e := range worst {
			if i >= maxExamples {
				break
			}
			st.Examples = append(st.Examples, e.key+"  "+e.what)
		}
	}
	return st
}

func printReport(w io.Writer, rep *report) {
	p := func(f string, a ...any) { _, _ = fmt.Fprintf(w, f, a...) }

	p("old  %s\n     %s, %d cases\n", rep.Old, rep.OldVersions, rep.OldCases)
	p("new  %s\n     %s, %d cases\n", rep.New, rep.NewVersions, rep.NewCases)
	if rep.OldVersions != rep.NewVersions {
		p("\n!! the oracle versions differ. Movement below is explained by that, and\n" +
			"!! the explanation belongs in the PR — see testdata/README.md.\n")
	}
	printSection(w, rep)

	if f := rep.FWI; f != nil {
		p("\n=== fwi_cases (the FWI System) ===\n")
		if f.NewSection {
			p("\nnew section: %d cases, and no fwi_cases in the old fixture to compare them\n"+
				"against. Nothing in it can have moved.\n", f.NewCases)
		} else {
			p("old %d cases, new %d cases\n", f.OldCases, f.NewCases)
			printSection(w, f)
		}
	}

	p("\n")
	if !rep.anyMoved() {
		p("No column shared by both fixtures moved.\n")
		return
	}
	moved := append([]string(nil), rep.Moved...)
	if rep.FWI != nil {
		for _, c := range rep.FWI.Moved {
			moved = append(moved, "fwi_cases."+c)
		}
	}
	p("MOVED: %s\n", strings.Join(moved, ", "))
	p("A changed reference number is the oracle telling you something. Do not commit past it.\n")
}

// printSection prints one section's key, case accounting and column table.
func printSection(w io.Writer, rep *report) {
	p := func(f string, a ...any) { _, _ = fmt.Fprintf(w, f, a...) }

	p("\nkeyed on %s\n", strings.Join(rep.KeyCols, ", "))
	p("shared %d, only in old %d, only in new %d\n", rep.SharedCases, rep.OnlyInOld, rep.OnlyInNew)
	if rep.DuplicateOld > 0 || rep.DuplicateNew > 0 {
		p("\nredundant rows: %d old, %d new — same inputs AND same outputs as a row already\n"+
			"counted, so they change nothing here. The sweep emits those cases twice.\n",
			rep.DuplicateOld, rep.DuplicateNew)
	}
	if rep.AmbiguousOld > 0 || rep.AmbiguousNew > 0 {
		p("\n!! %d old and %d new cases share a key with a row they DISAGREE with. The input\n"+
			"!! columns no longer identify a case, so the per-column verdicts below are not\n"+
			"!! trustworthy — an input the sweep varies is missing from inputCols.\n",
			rep.AmbiguousOld, rep.AmbiguousNew)
	}

	p("\n%-10s %-10s %9s %8s %14s %12s\n", "column", "status", "compared", "moved", "max abs", "max rel")
	for _, c := range rep.Columns {
		abs, rel := "-", "-"
		if c.Moved > 0 && c.MaxAbs > 0 {
			abs = fmt.Sprintf("%.6g", c.MaxAbs)
			rel = fmt.Sprintf("%.3g", c.MaxRel)
		}
		moved, compared := "-", "-"
		if c.Status == "moved" || c.Status == "unchanged" {
			moved = strconv.Itoa(c.Moved)
			compared = strconv.Itoa(c.Compared)
		}
		p("%-10s %-10s %9s %8s %14s %12s\n", c.Column, c.Status, compared, moved, abs, rel)
		if c.NullFlips > 0 {
			p("%-10s   %d of those are null<->number flips\n", "", c.NullFlips)
		}
		for _, e := range c.Examples {
			p("%-10s   %s\n", "", e)
		}
	}
}
