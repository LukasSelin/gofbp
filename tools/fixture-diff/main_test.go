package main

import (
	"math"
	"strings"
	"testing"
)

// mk builds a fixture from cases written as maps, so a test can say exactly what
// changed between two of them and nothing else.
func mk(path string, cffdrs string, cases ...map[string]any) *fixture {
	return &fixture{
		CFFDRSVersion: cffdrs,
		RVersion:      "R version 4.6.1",
		Cases:         cases,
		path:          path,
	}
}

func kase(fuel string, ffmc, bui float64, out map[string]any) map[string]any {
	c := map[string]any{
		"fuel": fuel, "ffmc": ffmc, "bui": bui,
		"ws": 10.0, "wd": 0.0, "gs": 0.0, "pc": 50.0, "pdf": 35.0,
		"cc": 80.0, "cbh": -1.0, "cfl": -1.0, "lat": 55.0, "dj": 180.0,
	}
	for k, v := range out {
		c[k] = v
	}
	return c
}

func find(rep *report, col string) colStat {
	for _, c := range rep.Columns {
		if c.Column == col {
			return c
		}
	}
	return colStat{Column: col, Status: "ABSENT"}
}

// The case this whole tool exists for: /migration-port adds a fixture column,
// and the question the checklist asks is whether anything ELSE moved.
func TestAddingAColumnMovesNothing(t *testing.T) {
	oldFx := mk("old", "1.9.2",
		kase("C1", 90, 40, map[string]any{"ros": 5.0, "isi": 12.0}),
		kase("C2", 90, 40, map[string]any{"ros": 9.0, "isi": 12.0}),
	)
	newFx := mk("new", "1.9.2",
		kase("C1", 90, 40, map[string]any{"ros": 5.0, "isi": 12.0, "sfc": 1.5}),
		kase("C2", 90, 40, map[string]any{"ros": 9.0, "isi": 12.0, "sfc": 2.5}),
	)

	rep := compare(oldFx, newFx, 0, 3)

	if len(rep.Moved) != 0 {
		t.Fatalf("adding a column reported movement in %v", rep.Moved)
	}
	if got := find(rep, "sfc").Status; got != "new" {
		t.Errorf("sfc status = %q, want new", got)
	}
	if got := find(rep, "ros").Status; got != "unchanged" {
		t.Errorf("ros status = %q, want unchanged", got)
	}
	if rep.SharedCases != 2 {
		t.Errorf("shared = %d, want 2", rep.SharedCases)
	}
}

// A widened sweep reorders rows. Comparing by position would report every row as
// changed; comparing by input key must report only the genuinely new ones.
func TestWideningTheSweepIsNotAMove(t *testing.T) {
	oldFx := mk("old", "1.9.2",
		kase("C1", 90, 40, map[string]any{"ros": 5.0}),
		kase("C2", 90, 40, map[string]any{"ros": 9.0}),
	)
	// Same two cases, opposite order, plus a third.
	newFx := mk("new", "1.9.2",
		kase("C2", 90, 40, map[string]any{"ros": 9.0}),
		kase("C3", 90, 40, map[string]any{"ros": 7.0}),
		kase("C1", 90, 40, map[string]any{"ros": 5.0}),
	)

	rep := compare(oldFx, newFx, 0, 3)

	if len(rep.Moved) != 0 {
		t.Fatalf("reordering + widening reported movement in %v", rep.Moved)
	}
	if rep.SharedCases != 2 || rep.OnlyInNew != 1 || rep.OnlyInOld != 0 {
		t.Errorf("shared=%d onlyNew=%d onlyOld=%d, want 2/1/0",
			rep.SharedCases, rep.OnlyInNew, rep.OnlyInOld)
	}
}

func TestAMovedNumberIsCaughtAndQuantified(t *testing.T) {
	oldFx := mk("old", "1.9.2",
		kase("C1", 90, 40, map[string]any{"ros": 5.0}),
		kase("C2", 90, 40, map[string]any{"ros": 9.0}),
	)
	newFx := mk("new", "1.10.0",
		kase("C1", 90, 40, map[string]any{"ros": 5.0}),
		kase("C2", 90, 40, map[string]any{"ros": 9.25}),
	)

	rep := compare(oldFx, newFx, 0, 3)

	if len(rep.Moved) != 1 || rep.Moved[0] != "ros" {
		t.Fatalf("moved = %v, want [ros]", rep.Moved)
	}
	ros := find(rep, "ros")
	if ros.Moved != 1 {
		t.Errorf("moved count = %d, want 1", ros.Moved)
	}
	if math.Abs(ros.MaxAbs-0.25) > 1e-12 {
		t.Errorf("max abs = %v, want 0.25", ros.MaxAbs)
	}
	if len(ros.Examples) != 1 || !strings.Contains(ros.Examples[0], "fuel=C2") {
		t.Errorf("examples = %v, want one naming C2", ros.Examples)
	}
}

// The default tolerance is zero on purpose: a reference number that moved in the
// sixteenth digit still moved, and the repo's rule is to explain it, not to
// widen a threshold past it.
func TestDefaultToleranceIsExact(t *testing.T) {
	oldFx := mk("old", "1.9.2", kase("C1", 90, 40, map[string]any{"ros": 5.0}))
	newFx := mk("new", "1.9.2", kase("C1", 90, 40, map[string]any{"ros": math.Nextafter(5.0, 6.0)}))

	if rep := compare(oldFx, newFx, 0, 3); len(rep.Moved) != 1 {
		t.Errorf("one ULP was not reported as a move")
	}
	if rep := compare(oldFx, newFx, 1e-12, 3); len(rep.Moved) != 0 {
		t.Errorf("one ULP was reported as a move under -tol 1e-12")
	}
}

// null is how the generator writes a non-finite oracle result. A column that
// starts or stops having an answer is the largest kind of move there is, and it
// has no delta, so it must not be quietly skipped.
func TestNullFlipsAreMoves(t *testing.T) {
	oldFx := mk("old", "1.9.2", kase("C1", 90, 40, map[string]any{"rso": nil}))
	newFx := mk("new", "1.9.2", kase("C1", 90, 40, map[string]any{"rso": 3.0}))

	rep := compare(oldFx, newFx, 0, 3)
	rso := find(rep, "rso")
	if rso.Status != "moved" || rso.NullFlips != 1 {
		t.Errorf("rso = %+v, want moved with 1 null flip", rso)
	}
}

// FD is the one string column, and a fire that was reclassified surface-to-crown
// is exactly the kind of change the numbers alone would not show.
func TestStringColumnsAreCompared(t *testing.T) {
	oldFx := mk("old", "1.9.2", kase("C1", 90, 40, map[string]any{"fd": "S"}))
	newFx := mk("new", "1.9.2", kase("C1", 90, 40, map[string]any{"fd": "I"}))

	rep := compare(oldFx, newFx, 0, 3)
	if find(rep, "fd").Status != "moved" {
		t.Errorf("a changed fire description was not reported")
	}
}

// A column that stopped being emitted is a lost assertion: the TestCFFDRS* that
// reads it starts skipping rows instead of failing, which looks like success.
func TestARemovedColumnIsAMove(t *testing.T) {
	oldFx := mk("old", "1.9.2", kase("C1", 90, 40, map[string]any{"ros": 5.0, "csi": 100.0}))
	newFx := mk("new", "1.9.2", kase("C1", 90, 40, map[string]any{"ros": 5.0}))

	rep := compare(oldFx, newFx, 0, 3)
	if find(rep, "csi").Status != "removed" {
		t.Errorf("csi status = %q, want removed", find(rep, "csi").Status)
	}
	if len(rep.Moved) != 1 || rep.Moved[0] != "csi" {
		t.Errorf("moved = %v, want [csi] — a removed column must fail, not pass quietly", rep.Moved)
	}
}

// If a new INPUT dimension is added to the sweep without being added to
// inputCols, the key stops identifying a case and every verdict is garbage. That
// has to be visible rather than inferred from a suspiciously large move count.
func TestUnknownInputColumnIsReportedAsAmbiguousKeys(t *testing.T) {
	// Two cases identical in every known input, differing only in an unlisted one.
	a := kase("C1", 90, 40, map[string]any{"elev": 100.0, "ros": 5.0})
	b := kase("C1", 90, 40, map[string]any{"elev": 900.0, "ros": 4.0})
	oldFx := mk("old", "1.9.2", a, b)
	newFx := mk("new", "1.9.2", a, b)

	rep := compare(oldFx, newFx, 0, 3)
	if rep.AmbiguousOld == 0 || rep.AmbiguousNew == 0 {
		t.Fatalf("ambiguous keys not reported: old=%d new=%d", rep.AmbiguousOld, rep.AmbiguousNew)
	}
	if rep.DuplicateOld != 0 || rep.DuplicateNew != 0 {
		t.Errorf("conflicting rows were miscounted as harmless duplicates: %d/%d",
			rep.DuplicateOld, rep.DuplicateNew)
	}
	var sb strings.Builder
	printReport(&sb, rep)
	if !strings.Contains(sb.String(), "inputCols") {
		t.Errorf("the report does not tell the reader what to do about ambiguous keys:\n%s", sb.String())
	}
}

func TestReportNamesTheVersionDriftWhenOracleVersionsDiffer(t *testing.T) {
	oldFx := mk("old", "1.9.2", kase("C1", 90, 40, map[string]any{"ros": 5.0}))
	newFx := mk("new", "1.10.0", kase("C1", 90, 40, map[string]any{"ros": 5.0}))

	var sb strings.Builder
	printReport(&sb, compare(oldFx, newFx, 0, 3))
	if !strings.Contains(sb.String(), "oracle versions differ") {
		t.Errorf("a version bump was not called out:\n%s", sb.String())
	}
}

// The real sweep emits some cases twice: its slope block repeats the flat block at
// gs = 0. Those rows agree with their twin in every column, so dropping one cannot
// change a verdict — and calling that "the verdicts are not trustworthy", as this
// first did, sends the reader off to investigate a non-problem in the middle of
// the one comparison the repo most needs to believe.
func TestIdenticalDuplicateRowsAreHarmlessNotAmbiguous(t *testing.T) {
	dup := kase("C2", 90, 40, map[string]any{"ros": 5.0, "isi": 12.0})
	other := kase("C3", 90, 40, map[string]any{"ros": 7.0, "isi": 12.0})
	oldFx := mk("old", "1.9.2", dup, other, dup)
	newFx := mk("new", "1.9.2", dup, other, dup)

	rep := compare(oldFx, newFx, 0, 3)

	if rep.DuplicateOld != 1 || rep.DuplicateNew != 1 {
		t.Errorf("duplicates = %d/%d, want 1/1", rep.DuplicateOld, rep.DuplicateNew)
	}
	if rep.AmbiguousOld != 0 || rep.AmbiguousNew != 0 {
		t.Errorf("identical rows were reported as conflicting: %d/%d", rep.AmbiguousOld, rep.AmbiguousNew)
	}
	if len(rep.Moved) != 0 {
		t.Errorf("moved = %v, want none", rep.Moved)
	}

	var sb strings.Builder
	printReport(&sb, rep)
	out := sb.String()
	if !strings.Contains(out, "redundant rows") {
		t.Errorf("duplicates not reported at all:\n%s", out)
	}
	if strings.Contains(out, "not\ntrustworthy") || strings.Contains(out, "DISAGREE") {
		t.Errorf("harmless duplicates raised the untrustworthy alarm:\n%s", out)
	}
}
