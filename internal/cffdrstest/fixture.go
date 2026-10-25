package cffdrstest

import (
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"os"
	"strings"
	"testing"
)

// The cffdrs fixture is the only oracle that can say the ST-X-3 coefficients in
// the FBP packages are actually right, and the only cross-implementation check the
// package has at all. cffdrs is maintained by the Canadian Forest Service authors
// of the FBP System, so it is the reference implementation rather than a second
// opinion.
//
// Regenerate with:
//
//	testdata/regen-cffdrs.sh
//
// R is needed only to regenerate, and that script does not need it on the host:
// it pins R and cffdrs in a container. The fixture is NOT committed — see
// testdata/README.md — so these tests skip on a fresh clone until you make it.
type Case struct {
	Fuel string  `json:"fuel"`
	FFMC float64 `json:"ffmc"`
	BUI  float64 `json:"bui"`
	WS   float64 `json:"ws"`
	WD   float64 `json:"wd"`
	GS   float64 `json:"gs"`
	PC   float64 `json:"pc"`
	PDF  float64 `json:"pdf"`
	CC   float64 `json:"cc"`
	// CBH and CFL are the crown-fire inputs as SENT. They are -1 on the surface
	// sweeps, the sentinel that makes fbp() substitute its own per-fuel table —
	// and since fbp() returns neither, those rows cannot say what values the
	// oracle actually used. Only rows with CBH > 0 and CFL > 0 are usable for the
	// crown assertions; see UsableForCrown.
	CBH float64 `json:"cbh"`
	CFL float64 `json:"cfl"`
	// LAT, LONG, ELV, DJ and D0 are the whole of what FMC is a function of.
	// LONG is SIGNED as the generator sent it — fbp() folds the sign itself, so
	// FoliarMoistureContent has to be handed math.Abs(LONG) to match. See
	// TestCFFDRSFoliarMoistureContent.
	LAT  float64 `json:"lat"`
	LONG float64 `json:"long"`
	ELV  float64 `json:"elv"`
	DJ   float64 `json:"dj"`
	D0   float64 `json:"d0"`
	// SD and SH are C6's stand density (stems/ha) and stand height (m), the
	// inputs to crown_base_height's stand branch. 0 outside the CBH/CFL defaults
	// block, which is fbp()'s own default. See TestCFFDRSCrownBaseHeight.
	SD float64 `json:"sd"`
	SH float64 `json:"sh"`
	// HR is elapsed HOURS as sent (fbp() runs the model in minutes) and Accel is
	// fbp()'s point-ignition switch. Only Accel = 1 rows exercise the acceleration
	// model; elsewhere HR is 1 and the t-columns below carry equilibrium values.
	HR    float64 `json:"hr"`
	Accel float64 `json:"accel"`
	// The acceleration model's outputs — see TestCFFDRSAcceleration. DF is the
	// driver's (DH + DB) / (2·LBt), carried for the flank composition.
	HROSt float64 `json:"hrost"`
	BROSt float64 `json:"brost"`
	FROSt float64 `json:"frost"`
	LBt   float64 `json:"lbt"`
	DH    float64 `json:"dh"`
	DB    float64 `json:"db"`
	DF    float64 `json:"df"`
	// What fbp()'s driver computes inline — see TestCFFDRSPredict. RAZ is in
	// degrees, towards; TI/FTI/BTI are minutes to crown-fire initiation.
	RAZ  float64 `json:"raz"`
	FCFB float64 `json:"fcfb"`
	BCFB float64 `json:"bcfb"`
	FTFC float64 `json:"ftfc"`
	BTFC float64 `json:"btfc"`
	FFI  float64 `json:"ffi"`
	BFI  float64 `json:"bfi"`
	TI   float64 `json:"ti"`
	FTI  float64 `json:"fti"`
	BTI  float64 `json:"bti"`
	ISI  float64 `json:"isi"`
	BE   float64 `json:"be"`
	SF   float64 `json:"sf"`
	WSV  float64 `json:"wsv"`
	FMC  float64 `json:"fmc"`
	SFC  float64 `json:"sfc"`
	CSI  float64 `json:"csi"`
	RSO  float64 `json:"rso"`
	CFB  float64 `json:"cfb"`
	// CFC and TFC are eqs. 66a/66b/66c and 67. Both embed the CFL fbp() actually
	// used, which on a row that sent the −1 sentinel is the per-fuel table value
	// and not the −1 recorded in CFL above — so only UsableForConsumption rows
	// can say what CFL produced them. See TestCFFDRSTotalFuelConsumption.
	CFC float64 `json:"cfc"`
	TFC float64 `json:"tfc"`
	// HFI is eq. 69, 300 · TFC · ROS, with fbp()'s own TFC and its reported head
	// ROS — so it inherits TFC's CFL caveat above, and C6's blended rate. See
	// TestCFFDRSHeadFireIntensity.
	HFI  float64 `json:"hfi"`
	FD   string  `json:"fd"`
	ROS  float64 `json:"ros"`
	LB   float64 `json:"lb"`
	BROS float64 `json:"bros"`
	FROS float64 `json:"fros"`
}

// MaxSuppliedCBH and MaxSuppliedCFL are the largest CBH and CFL cffdrs'
// crown_base_height and crown_fuel_load honour as sent rather than replace with
// the per-fuel table. They are crown.MaxSuppliedCrownBaseHeightM and
// crown.MaxSuppliedCrownFuelLoadKgM2, restated here because this package cannot
// import crown (crown's own tests import this one); TestFixtureBoundsAreCrowns
// is what keeps the two equal.
const (
	MaxSuppliedCBH = 50.0
	MaxSuppliedCFL = 2.0
)

// UsableForCrown reports whether a case carries crown inputs the FBP packages can be
// fed: a CBH and a CFL that fbp() honoured verbatim. See the CBH/CFL note above.
//
// It was CBH > 0 && CFL > 0 until the CBH/CFL defaults block (2026-10-21) sent
// CBH 60 and CFL 3 on purpose, to reach crown_base_height's and
// crown_fuel_load's other substitution triggers — and those rows, positive and
// substituted, failed every crown test at once. The upper bounds are the same
// mechanism as the -1 sentinel, and CrownBaseHeight / CrownFuelLoad are what
// assert those rows instead.
func (c Case) UsableForCrown() bool {
	return c.CBH > 0 && c.CBH <= MaxSuppliedCBH &&
		c.UsableForConsumption()
}

// UsableForConsumption reports whether the CFL this case SENT is the one fbp()
// multiplied into its cfc and tfc columns.
//
// It is a narrower test than UsableForCrown, and the extra clause is not
// defensive. crown_fuel_load substitutes the per-fuel table whenever CFL is
// non-positive, NA, **or greater than 2** — so a row sending CFL = 3 would be
// recorded as 3 and computed at the table value, and reading it as an assertion
// would blame the FBP packages for the gap. No block sends one, and this is what
// keeps that true rather than assumed.
func (c Case) UsableForConsumption() bool {
	return c.CFL > 0 && c.CFL <= MaxSuppliedCFL
}

// Fixture is testdata/cffdrs.json: the oracle's versions and its three
// sections of cases.
type Fixture struct {
	Oracle        string `json:"oracle"`
	CFFDRSVersion string `json:"cffdrs_version"`
	RVersion      string `json:"r_version"`
	Cases         []Case `json:"cases"`
	// FBPGrid is fbpRaster()'s section, read by grid_cffdrs_test.go. Absent
	// from every fixture older than the fbpRaster() block.
	FBPGrid []GridCase `json:"fbp_grid_cases"`
	// Theta is the rate-at-a-bearing section: cffdrs' two defective rates, recorded
	// so testdata/divergences.json can be held against them. Read by
	// TestCFFDRSDivergences; absent from every fixture older than the register.
	Theta []map[string]any `json:"theta_cases"`
}

// Fuel maps a cffdrs fuel name onto the FBP packages's key. cffdrs spells the
// grass fuels O1a/O1b; ST-X-3's tables and Fuels use O1A/O1B.
//
// The package no longer needs this at a call site — CanonicalFuelCode folds the
// fixture's spelling itself, and TestSpellingDoesNotChangeAnyAnswer is what
// holds that. It stays because the per-fuel tallies below key their maps on the
// result, and a tally split between "O1a" and "O1B" would under-report coverage
// on both without failing anything.
func Fuel(code string) string { return strings.ToUpper(code) }

// Load reads testdata/cffdrs.json, skipping t when the fixture has not been
// generated.
func Load(t *testing.T) Fixture {
	t.Helper()
	raw, err := os.ReadFile(RepoPath(t, "testdata/cffdrs.json"))
	if errors.Is(err, fs.ErrNotExist) {
		t.Skip("testdata/cffdrs.json is absent; it is generated, not committed — " +
			"run testdata/regen-cffdrs.sh (needs Docker). See testdata/README.md.")
	}
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var f Fixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(f.Cases) == 0 {
		t.Fatal("fixture has no cases")
	}
	t.Logf("oracle: %s %s (%s), %d cases", f.Oracle, f.CFFDRSVersion, f.RVersion, len(f.Cases))
	return f
}

// CrownChangesROS reports whether cffdrs' reported ROS for a case is a different
// quantity from the surface rate the FBP packages computes.
//
// It is FUEL TYPE, not CFB > 0, and the difference recovered several thousand
// cases of oracle coverage. cffdrs' rate_of_spread() returns the surface rate RSS
// unchanged for every fuel except C6, folding CFB in only through C6's separate
// crown rate of spread — so a crowning C1 stand has exactly the same reported ROS
// it would have had with no crown model at all. These tests previously excluded
// every case with CFB != 0, which discarded 3689 of the fixture's 11500 rows to
// avoid a divergence that, outside C6, does not exist.
//
// If that reading is ever wrong, the tests using this fail loudly with a ratio
// attached. Do not widen it back to CFB != 0 to make a failure go away.
//
// C6's crown rate of spread landing (c6.go, 2026-09-19) does NOT make this
// removable, and the reason is not the crown path. ROS takes a fuel code, ISI,
// BUI and the three blend inputs, with nowhere to put the FMC, SFC, CBH and CFL
// C6's blend needs, so it is still the surface rate for C6 and the fixture's ros
// column still is not. TestCFFDRSC6RateOfSpread is where C6's coverage lives
// instead, and its header records a second reason for the sloped rows: cffdrs'
// own slope back-solve for C6 inverts a blended rate, which NetEffectiveWind
// does not reproduce for any fuel.
func CrownChangesROS(c Case) bool { return Fuel(c.Fuel) == "C6" }

// CloseEnough reports whether got is within tol of want, relative to want
// above 1 and absolute below it.
func CloseEnough(got, want, tol float64) bool {
	return math.Abs(got-want) <= tol*math.Max(1, math.Abs(want))
}

// RelErr is CloseEnough's metric, exposed for logging how much headroom the
// assertions actually have.
func RelErr(got, want float64) float64 {
	return math.Abs(got-want) / math.Max(1, math.Abs(want))
}

// SweepGrassFuelLoad is the GFL that gen_cffdrs_reference.R sends on every row.
//
// GFL is the one SFC driver the fixture does NOT carry a column for, because it
// is a constant in the generator rather than a swept input — see base_row(). So
// this has to be written down twice, and the two copies are joined by nothing
// but this comment. That is tolerable only because the join fails loudly: change
// GFL in the generator and TestCFFDRSSurfaceFuelConsumption's O1A/O1B cases go
// red on the next regeneration, naming both numbers.
//
// It is NOT a default. SurfaceFuelConsumption has no default grass fuel load and
// deliberately will not grow one; this is the value this particular sweep was
// generated with, nothing more.
const SweepGrassFuelLoad = 0.35
