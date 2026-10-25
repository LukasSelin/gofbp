package oracle

import (
	"math"
	"testing"
)

// The committed register has to load: this runs on a fresh clone, with no
// fixture, and is what stops a malformed entry reaching a TestCFFDRS* that
// would only find out once someone regenerated.
func TestRegisterLoads(t *testing.T) {
	r, err := Load("../../testdata/divergences.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d divergences", len(r.Divergences))
}

func f(x float64) *float64 { return &x }

func TestCondSemantics(t *testing.T) {
	row := MapGetter(map[string]any{"fuel": "C6", "gs": 30.0, "na": nil, "nan": math.NaN()})
	for _, tc := range []struct {
		name string
		col  string
		c    Cond
		want bool
	}{
		{"in string", "fuel", Cond{In: []any{"C6"}}, true},
		{"in miss", "fuel", Cond{In: []any{"C2"}}, false},
		{"not_in", "fuel", Cond{NotIn: []any{"C2"}}, true},
		{"gt", "gs", Cond{Gt: f(0)}, true},
		{"gt equal", "gs", Cond{Gt: f(30)}, false},
		{"ge equal", "gs", Cond{Ge: f(30)}, true},
		{"lt", "gs", Cond{Lt: f(30)}, false},
		{"le", "gs", Cond{Le: f(30)}, true},
		{"bound on a string", "fuel", Cond{Gt: f(0)}, false},
		{"null nil", "na", Cond{Null: true}, true},
		{"null NaN", "nan", Cond{Null: true}, true},
		{"null absent", "absent", Cond{Null: true}, true},
		{"null present", "gs", Cond{Null: true}, false},
		// A null satisfies nothing but Null — not even not_in, so an NA cell is
		// never swept into an entry about out-of-range values by accident.
		{"not_in on null", "na", Cond{NotIn: []any{1.0}}, false},
		{"gt on NaN", "nan", Cond{Gt: f(0)}, false},
	} {
		if got := tc.c.holds(row(tc.col)); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestFindIsFirstMatchInFileOrder(t *testing.T) {
	r := &Register{Divergences: []Divergence{
		{ID: "a", Section: "s", When: []Clause{{"x": {Gt: f(0)}}}, Columns: []string{"ros"}},
		{ID: "b", Section: "s", When: []Clause{{"x": {Gt: f(0)}}}, Columns: []string{"*"}},
	}}
	row := MapGetter(map[string]any{"x": 1.0})
	if d := r.Find("s", row, "ros"); d == nil || d.ID != "a" {
		t.Errorf("ros: %v, want a", d)
	}
	if d := r.Find("s", row, "cfb"); d == nil || d.ID != "b" {
		t.Errorf("cfb: %v, want b", d)
	}
	if d := r.Find("other", row, "ros"); d != nil {
		t.Errorf("other section: %v, want nil", d.ID)
	}
}

// The Table 3 triple through the published equation and through cffdrs'
// transcription: the first reproduces the paper, the second does not, and that
// difference is what the register's eq94-misparenthesised entry is about.
func TestEQ94FormulasAgainstTable3(t *testing.T) {
	row := func(deg float64) Getter {
		return MapGetter(map[string]any{"ros_in": 17.16, "fros_in": 4.88, "bros_in": 2.16, "theta": deg})
	}
	if got := Formulas["glcx10-eq94"](row(15)); math.Abs(got-14.29) > 0.01 {
		t.Errorf("published eq. 94 at 15° = %v, Table 3 prints 14.29", got)
	}
	if got := Formulas["cffdrs-eq94-misparenthesised"](row(15)); math.Abs(got-14.29) < 1 {
		t.Errorf("cffdrs' form at 15° = %v, which is Table 3's 14.29 — the defect is not reproduced", got)
	}
	if got := Formulas["glcx10-eq94"](row(90)); !math.IsNaN(got) {
		t.Errorf("published eq. 94 at 90° = %v, want NaN (the 0/0 the paper describes)", got)
	}
}
