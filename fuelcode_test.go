package fbp

import "testing"

// The fuel code is the one input to this package that is not a number, and it is
// the one the caller is most likely to get from somewhere else — a raster
// attribute table, a shapefile column, a hand-maintained CSV. These tests pin
// what happens to it on the way in.

// TestCanonicalFuelCodeFoldsSpelling covers the fold itself: case, and the
// separators a fuel column picks up from whatever produced it.
func TestCanonicalFuelCodeFoldsSpelling(t *testing.T) {
	for _, tc := range []struct {
		in    string
		want  string
		known bool
	}{
		{"C2", "C2", true},
		{"c2", "C2", true},
		{"C-2", "C2", true},
		{" C2 ", "C2", true},
		{"c_2", "C2", true},
		// The spellings ST-X-3 itself prints for the grass fuels, against the
		// spelling Fuels is keyed by.
		{"O1a", "O1A", true},
		{"O1b", "O1B", true},
		{"o1a", "O1A", true},
		{"O-1a", "O1A", true},
		{"m1", "M1", true},
		// Real FBP fuels this package does not implement. They must fold to a
		// clean canonical spelling and still report false: a caller logging the
		// rejection wants to see "M3", not the raw input.
		{"M3", "M3", false},
		{"m4", "M4", false},
		{"D2", "D2", false},
		// cffdrs' non-fuel classes.
		{"WA", "WA", false},
		{"NF", "NF", false},
		// Neither a fuel nor plausible as one.
		{"", "", false},
		{"nonsense", "NONSENSE", false},
		{"a-very-long-fuel-code", "", false},
	} {
		got, known := CanonicalFuelCode(tc.in)
		if got != tc.want || known != tc.known {
			t.Errorf("CanonicalFuelCode(%q) = (%q, %v), want (%q, %v)",
				tc.in, got, known, tc.want, tc.known)
		}
	}
}

// TestCanonicalFuelCodeAcceptsEveryTableKey is the fold's fixed-point property:
// every key in Fuels must survive it unchanged and report known. Without this a
// future key added in a spelling the fold alters — anything lowercase, say —
// would be unreachable through the public API while sitting in plain sight in
// the table.
func TestCanonicalFuelCodeAcceptsEveryTableKey(t *testing.T) {
	for code := range Fuels {
		got, known := CanonicalFuelCode(code)
		if got != code || !known {
			t.Errorf("CanonicalFuelCode(%q) = (%q, %v), want (%q, true) — "+
				"the Fuels key is not its own canonical form", code, got, known, code)
		}
	}
}

// TestSpellingDoesNotChangeAnyAnswer is the regression this fold exists for.
//
// LengthToBreadth accepted "O1a" and returned the grass form; RSI did not, and
// returned 0. So a caller with a fuel raster labelled the way ST-X-3 labels it
// got a correct length-to-breadth ratio next to a zero spread rate — two
// functions in the same package disagreeing about whether the fuel existed, with
// nothing in either signature to say so. Every code-taking function must now
// agree, and the O1 pair is the case that actually broke.
func TestSpellingDoesNotChangeAnyAnswer(t *testing.T) {
	const (
		isi = 12.0
		bui = 60.0
		pc  = 50.0
		cc  = 90.0
	)
	for _, spellings := range [][]string{
		{"C2", "c2", "C-2", " C2 "},
		{"O1A", "O1a", "o1a", "O-1a"},
		{"O1B", "O1b", "o1b"},
		{"M1", "m1"},
		{"S3", "s3"},
	} {
		want := spellings[0]
		for _, got := range spellings[1:] {
			if a, b := RSI(want, isi, pc, cc), RSI(got, isi, pc, cc); a != b {
				t.Errorf("RSI: %q gave %v, %q gave %v", want, a, got, b)
			}
			if a, b := BuildupEffect(want, bui), BuildupEffect(got, bui); a != b {
				t.Errorf("BuildupEffect: %q gave %v, %q gave %v", want, a, got, b)
			}
			if a, b := LengthToBreadth(want, 25), LengthToBreadth(got, 25); a != b {
				t.Errorf("LengthToBreadth: %q gave %v, %q gave %v", want, a, got, b)
			}
			if a, b := ROS(want, isi, bui, pc, cc, 30), ROS(got, isi, bui, pc, cc, 30); a != b {
				t.Errorf("ROS: %q gave %v, %q gave %v", want, a, got, b)
			}
			sw := func(code string) SlopeWind {
				return SlopeWind{
					Code: code, FFMC: 92, SlopePct: 30, WindKmh: 15,
					WindAzimuthDeg: 45, UpslopeAzimuthDeg: 90, PC: pc, CuringPct: cc,
				}
			}
			if a, b := EquivalentWind(sw(want)), EquivalentWind(sw(got)); a != b {
				t.Errorf("EquivalentWind: %q gave %v, %q gave %v", want, a, got, b)
			}
			wsvA, razA := NetEffectiveWind(sw(want))
			wsvB, razB := NetEffectiveWind(sw(got))
			if wsvA != wsvB || razA != razB {
				t.Errorf("NetEffectiveWind: %q gave (%v, %v), %q gave (%v, %v)",
					want, wsvA, razA, got, wsvB, razB)
			}
		}
	}
}

// TestGrassSpellingReachesTheGrassBranch guards the fold against a fix that
// passes TestSpellingDoesNotChangeAnyAnswer by making every spelling equally
// wrong. "O1a" has to reach the curing factor and eqs. 80-81, not merely agree
// with "O1A" on some third value.
func TestGrassSpellingReachesTheGrassBranch(t *testing.T) {
	// Curing must bite: the grass RSI is scaled by CuringFactor, so two curing
	// levels cannot give the same answer.
	dry, damp := RSI("O1a", 12, 0, 100), RSI("O1a", 12, 0, 30)
	if !(dry > damp) {
		t.Errorf("RSI(O1a) at 100%% curing = %v, at 30%% = %v; curing did not apply", dry, damp)
	}
	// Eq. 80/81 is measurably more elongated than eq. 79 at the same wind.
	if grass, wooded := LengthToBreadth("O1a", 25), LengthToBreadth("C2", 25); !(grass > wooded) {
		t.Errorf("LengthToBreadth(O1a) = %v, LengthToBreadth(C2) = %v; "+
			"the grass form was not reached", grass, wooded)
	}
}

// TestUnimplementedFuelIsReportedRatherThanInferred is the other half of the
// silent-zero fix.
//
// The numbers cannot carry this. Every function here returns a float64, so a
// fuel this package does not implement can only arrive as 0 m/min, which is also
// what a cell that will not carry fire returns — and M3/M4 are real FBP fuels
// that cffdrs implements, so the input is not even a mistake. CanonicalFuelCode
// is the only thing in the API that can tell the caller which of the two it is
// looking at, and this asserts it does.
func TestUnimplementedFuelIsReportedRatherThanInferred(t *testing.T) {
	for _, code := range []string{"M3", "M4", "D2", "WA", "NF", "nonsense", ""} {
		if _, known := CanonicalFuelCode(code); known {
			t.Errorf("CanonicalFuelCode(%q) reports known; this package has no coefficients for it", code)
		}
		if got := RSI(code, 12, 50, 90); got != 0 {
			t.Errorf("RSI(%q) = %v, want 0", code, got)
		}
	}
	// And the contrast that makes the point: a zero spread rate from an
	// implemented fuel is a real prediction, and reports known.
	if _, known := CanonicalFuelCode("D1"); !known {
		t.Fatal("CanonicalFuelCode(D1) reports unknown")
	}
	if got := RSI("D1", 0, 0, 0); got != 0 {
		t.Errorf("RSI(D1) at ISI 0 = %v, want 0", got)
	}
}

// TestCanonicalFuelCodeDoesNotAllocateOnTableKeys pins the fast path. This runs
// per grid cell in any raster caller, and the fold allocates a string; the
// already-canonical shortcut is what keeps that off the common path. A change
// that removes the shortcut is not a correctness regression and no other test
// would notice it.
func TestCanonicalFuelCodeDoesNotAllocateOnTableKeys(t *testing.T) {
	for code := range Fuels {
		if n := testing.AllocsPerRun(100, func() {
			CanonicalFuelCode(code)
		}); n != 0 {
			t.Errorf("CanonicalFuelCode(%q) allocated %v times per call, want 0", code, n)
		}
	}
}
