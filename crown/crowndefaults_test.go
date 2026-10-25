package crown

import (
	"math"
	"testing"

	"github.com/LukasSelin/gofbp/fuel"
	"github.com/LukasSelin/gofbp/internal/cffdrstest"
)

// The per-fuel crown defaults, without the fixture.
//
// The tables are transcribed literally here as well as in crowndefaults.go, on
// purpose: a test that read crownBaseHeightTable to check DefaultCrownBaseHeight
// would agree with any typo. The values are cffdrs' CBHs and CFLs vectors at the
// pinned commit; TestCFFDRSCrownBaseHeight and TestCFFDRSCrownFuelLoad are what
// tie them to fbp()'s output.

var publishedCrownDefaults = map[string][2]float64{ // CBH m, CFL kg/m²
	"C1": {2, 0.75}, "C2": {3, 0.8}, "C3": {8, 1.15}, "C4": {4, 1.2}, "C5": {18, 1.2},
	"C6": {7, 1.8}, "C7": {10, 0.5}, "D1": {0, 0},
	"M1": {6, 0.8}, "M2": {6, 0.8}, "M3": {6, 0.8}, "M4": {6, 0.8},
	"S1": {0, 0}, "S2": {0, 0}, "S3": {0, 0}, "O1A": {0, 0}, "O1B": {0, 0},
}

func TestDefaultCrownTablesArePublishedValues(t *testing.T) {
	if len(publishedCrownDefaults) != len(fuel.Fuels) {
		t.Fatalf("%d published entries for %d fuels", len(publishedCrownDefaults), len(fuel.Fuels))
	}
	for code := range fuel.Fuels {
		want, ok := publishedCrownDefaults[code]
		if !ok {
			t.Errorf("%s is in Fuels but has no published crown default here", code)
			continue
		}
		cbh, okH := DefaultCrownBaseHeight(code)
		cfl, okL := DefaultCrownFuelLoad(code)
		if !okH || !okL || cbh != want[0] || cfl != want[1] {
			t.Errorf("%s: defaults (%v, %v) ok=(%v, %v), want (%v, %v)", code, cbh, cfl, okH, okL, want[0], want[1])
		}
	}
	// The lookup folds codes the way the rest of the package does.
	if cbh, ok := DefaultCrownBaseHeight("o1-b"); !ok || cbh != 0 {
		t.Errorf("DefaultCrownBaseHeight(\"o1-b\") = %v, %v", cbh, ok)
	}
	if cfl, ok := DefaultCrownFuelLoad("c3"); !ok || cfl != 1.15 {
		t.Errorf("DefaultCrownFuelLoad(\"c3\") = %v, %v", cfl, ok)
	}
}

func TestDefaultCrownTablesUnknownFuel(t *testing.T) {
	for _, code := range []string{"D2", "WA", "NF", "", "C8"} {
		if v, ok := DefaultCrownBaseHeight(code); ok || v != 0 {
			t.Errorf("DefaultCrownBaseHeight(%q) = %v, %v; want 0, false", code, v, ok)
		}
		if v, ok := DefaultCrownFuelLoad(code); ok || v != 0 {
			t.Errorf("DefaultCrownFuelLoad(%q) = %v, %v; want 0, false", code, v, ok)
		}
		if v := CrownBaseHeight(code, -1, 0, 0); v != 0 {
			t.Errorf("CrownBaseHeight(%q, -1) = %v, want 0", code, v)
		}
		if v := CrownFuelLoad(code, -1); v != 0 {
			t.Errorf("CrownFuelLoad(%q, -1) = %v, want 0", code, v)
		}
	}
	// In range, a supplied value is returned whatever the fuel — the same as
	// cffdrs, which never looks at the fuel for an honoured value.
	if v := CrownBaseHeight("D2", 5, 0, 0); v != 5 {
		t.Errorf("CrownBaseHeight(\"D2\", 5) = %v, want 5", v)
	}
}

// The substitution bounds, from both sides. (0, 50] and (0, 2] are honoured.
func TestCrownDefaultsSubstitutionBounds(t *testing.T) {
	cases := []struct {
		supplied, want float64
	}{
		{-1, 8}, {0, 8}, {1e-9, 1e-9}, {0.5, 0.5}, {50, 50}, {50.0000001, 8}, {60, 8},
		{math.NaN(), 8}, {math.Inf(1), 8}, {math.Inf(-1), 8},
	}
	for _, c := range cases {
		if got := CrownBaseHeight("C3", c.supplied, 0, 0); got != c.want {
			t.Errorf("CrownBaseHeight(C3, %v) = %v, want %v", c.supplied, got, c.want)
		}
	}
	cfl := []struct {
		supplied, want float64
	}{
		{-1, 1.15}, {0, 1.15}, {0.01, 0.01}, {2, 2}, {2.0000001, 1.15}, {3, 1.15},
		{math.NaN(), 1.15}, {math.Inf(1), 1.15},
	}
	for _, c := range cfl {
		if got := CrownFuelLoad("C3", c.supplied); got != c.want {
			t.Errorf("CrownFuelLoad(C3, %v) = %v, want %v", c.supplied, got, c.want)
		}
	}
}

func TestC6StandCrownBaseHeight(t *testing.T) {
	cases := []struct {
		sd, sh, want float64
	}{
		{2000, 15, -11.2 + 1.06*15 + 0.0017*2000}, // 8.1
		{500, 30, -11.2 + 1.06*30 + 0.0017*500},   // 21.45
		{500, 5, MinCrownBaseHeightM},             // -5.05, floored
		{0, 0, MinCrownBaseHeightM},               // -11.2, floored
	}
	for _, c := range cases {
		if got := C6StandCrownBaseHeight(c.sd, c.sh); !cffdrstest.CloseEnough(got, c.want, 1e-15) {
			t.Errorf("C6StandCrownBaseHeight(%v, %v) = %v, want %v", c.sd, c.sh, got, c.want)
		}
	}
	if got := C6StandCrownBaseHeight(math.NaN(), 10); !math.IsNaN(got) {
		t.Errorf("C6StandCrownBaseHeight(NaN, 10) = %v, want NaN", got)
	}
}

// When CrownBaseHeight uses the stand equation, and when it falls back.
func TestCrownBaseHeightC6StandBranch(t *testing.T) {
	stand := C6StandCrownBaseHeight(2000, 15)
	cases := []struct {
		name        string
		code        string
		cbh, sd, sh float64
		want        float64
	}{
		{"stand data replaces the table", "C6", -1, 2000, 15, stand},
		{"a supplied CBH wins over the stand", "C6", 10, 2000, 15, 10},
		{"both SD and SH are needed", "C6", -1, 2000, 0, 7},
		{"SD above 1e5 is screened out", "C6", -1, 150000, 15, 7},
		{"SD at 1e5 is honoured", "C6", -1, 1e5, 15, C6StandCrownBaseHeight(1e5, 15)},
		{"SH above 100 is screened out", "C6", -1, 2000, 150, 7},
		{"SH at 100 is honoured", "C6", -1, 2000, 100, C6StandCrownBaseHeight(2000, 100)},
		{"NaN stand data is missing", "C6", -1, math.NaN(), 15, 7},
		{"only C6 has a stand branch", "C3", -1, 2000, 15, 8},
		{"the code is folded", "c6", -1, 2000, 15, stand},
	}
	for _, c := range cases {
		if got := CrownBaseHeight(c.code, c.cbh, c.sd, c.sh); !cffdrstest.CloseEnough(got, c.want, 1e-15) {
			t.Errorf("%s: CrownBaseHeight(%s, %v, %v, %v) = %v, want %v",
				c.name, c.code, c.cbh, c.sd, c.sh, got, c.want)
		}
	}
}

// The reason the CFL table matters: on the published defaults the six fuels
// without a crown never crown, under any fire, and the rest can.
func TestDefaultCrownLoadsGateCrowning(t *testing.T) {
	for code := range fuel.Fuels {
		cbh, _ := DefaultCrownBaseHeight(code)
		cfl, _ := DefaultCrownFuelLoad(code)
		cfb := CrownFractionBurned(Crown{FMC: 100, SFC: 2, CBH: cbh, CFL: cfl, SurfaceROS: 80})
		if cfl == 0 && cfb != 0 {
			t.Errorf("%s: published CFL 0 but CFB = %v at 80 m/min", code, cfb)
		}
		if cfl > 0 && !(cfb > 0) {
			t.Errorf("%s: published CFL %v but CFB = %v at 80 m/min — the defaults cannot crown", code, cfl, cfb)
		}
	}
}
