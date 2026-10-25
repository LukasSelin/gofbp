package main

import (
	"math"
	"strings"
	"testing"

	"github.com/LukasSelin/gofbp/internal/oracle"
)

func ptr(x float64) *float64 { return &x }

// A register with one refusal over sloped C6 and one cffdrs defect over the
// eq. 94 rows, built in the test so it says exactly what is being exercised.
func testRegister() *oracle.Register {
	return &oracle.Register{Divergences: []oracle.Divergence{
		{
			ID: "c6-on-slope", Verdict: oracle.GoFBPRefuses, Section: "cases",
			When:    []oracle.Clause{{"fuel": {In: []any{"C6"}}, "gs": {Gt: ptr(0)}}},
			Columns: []string{"*"},
		},
		{
			ID: "eq94-misparenthesised", Verdict: oracle.CFFDRSWrong, Section: "theta_cases",
			When:      []oracle.Clause{{"kind": {In: []any{"eq94"}}}},
			Columns:   []string{"ros_theta"},
			Signature: "cffdrs-eq94-misparenthesised",
		},
	}}
}

func eq94Row(deg, rosTheta float64) map[string]any {
	return map[string]any{"kind": "eq94", "ros_in": 17.16, "fros_in": 4.88, "bros_in": 2.16,
		"theta": deg, "ros_theta": rosTheta}
}

// A move inside a registered divergence is told apart from one outside every
// entry, per moved case — the first is a verdict to re-read, the second is the
// ordinary finding.
func TestMovesAreAttributedToTheRegister(t *testing.T) {
	sloped := func(fuel string, ros float64) map[string]any {
		c := kase(fuel, 90, 40, map[string]any{"ros": ros})
		c["gs"] = 30.0
		return c
	}
	oldFx := mk("old", "1.9.2", sloped("C6", 5), sloped("C2", 9), kase("C6", 90, 40, map[string]any{"ros": 4.0}))
	newFx := mk("new", "1.9.3", sloped("C6", 6), sloped("C2", 10), kase("C6", 90, 40, map[string]any{"ros": 4.0}))

	rep := compare(oldFx, newFx, 0, 3)
	attribute(rep, testRegister())

	ros := find(rep, "ros")
	if ros.Moved != 2 {
		t.Fatalf("ros moved %d, want 2", ros.Moved)
	}
	if ros.Divergences["c6-on-slope"] != 1 || ros.Unexplained != 1 {
		t.Errorf("attribution = %v, %d unexplained; want 1 inside c6-on-slope and 1 outside",
			ros.Divergences, ros.Unexplained)
	}
	var sb strings.Builder
	printReport(&sb, rep)
	for _, want := range []string{"1 inside divergence c6-on-slope", "1 outside every registered divergence"} {
		if !strings.Contains(sb.String(), want) {
			t.Errorf("report does not say %q:\n%s", want, sb.String())
		}
	}
}

// cffdrs fixing a defect is a finding even if nothing else moved: the
// register's verdict about it is now wrong.
func TestABrokenSignatureIsAMove(t *testing.T) {
	sig := oracle.Formulas["cffdrs-eq94-misparenthesised"]
	defective := sig(oracle.MapGetter(eq94Row(15, 0)))
	fixed := oracle.EQ94Published(17.16, 4.88, 2.16, 15)

	still := mk("still", "1.9.2", kase("C2", 90, 40, nil))
	still.ThetaCases = []map[string]any{eq94Row(15, defective)}
	rep := &report{Signatures: checkSignatures(still, testRegister())}
	if len(rep.Signatures) != 1 || rep.Signatures[0].Rows != 1 || rep.Signatures[0].Broken != 0 {
		t.Fatalf("defect still present: %+v, want 1 row, 0 broken", rep.Signatures)
	}
	if rep.anyMoved() {
		t.Error("a signature that still holds counted as a move")
	}

	gone := mk("gone", "1.9.3", kase("C2", 90, 40, nil))
	gone.ThetaCases = []map[string]any{eq94Row(15, fixed)}
	rep = &report{Signatures: checkSignatures(gone, testRegister())}
	if rep.Signatures[0].Broken != 1 {
		t.Fatalf("defect fixed upstream: %+v, want 1 broken", rep.Signatures)
	}
	if !rep.anyMoved() {
		t.Error("a broken signature did not count as a move, so the exit status would be 0")
	}
	if math.Abs(fixed-14.29) > 0.01 {
		t.Fatalf("test premise: the published form at 15° is %v, not Table 3's 14.29", fixed)
	}
}

func TestRegisterFlag(t *testing.T) {
	if r, err := loadRegister("none", "../../testdata/cffdrs.json"); r != nil || err != nil {
		t.Errorf("none: %v, %v", r, err)
	}
	if r, err := loadRegister("", "../../testdata/cffdrs.json"); err != nil || r == nil {
		t.Errorf("default beside the fixture: %v, %v — want testdata/divergences.json", r, err)
	}
	if r, err := loadRegister("", t.TempDir()+"/cffdrs.json"); r != nil || err != nil {
		t.Errorf("no register beside the fixture: %v, %v — want none, silently", r, err)
	}
}
