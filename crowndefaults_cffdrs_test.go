package fbp

import (
	"sort"
	"testing"
)

// The per-fuel crown defaults against the oracle.
//
// fbp() returns neither CBH nor CFL, so neither is asserted directly. Both are
// asserted FORWARD instead, through the first output each one enters: CSI is
// eq. 56 of the CBH fbp() settled on, and CFC is eq. 66 of the CFL it settled
// on. That makes every one of the fixture's rows an assertion about what
// CrownBaseHeight and CrownFuelLoad return for the CBH and CFL it SENT —
// including the thousands of -1 sentinel rows, which used to be the reason
// these columns could say nothing about crown inputs, and are now the table's
// main coverage.
//
// The generator's CBH/CFL defaults block (gen_cffdrs_reference.R) adds what the
// sentinel rows cannot reach: the other substitution triggers (0, > 50, > 2),
// the inclusive bounds (CBH 50, CFL 2), and C6's stand branch with its floor
// and both of fbp()'s screens. The tests below count each branch and fail if
// any is empty, so narrowing that block is caught.

// ledger: crown_base_height.r
func TestCFFDRSCrownBaseHeight(t *testing.T) {
	f := loadCFFDRS(t)
	const tol = 1e-9
	requireColumn(t, f, "sd/sh", func(c cffdrsCase) bool { return c.SD != 0 || c.SH != 0 })

	branch := map[string]int{}
	tableByFuel := map[string]int{}
	bad := 0
	var worst float64
	for i, c := range f.Cases {
		fuel := ourFuel(c.Fuel)
		cbh := CrownBaseHeight(fuel, c.CBH, c.SD, c.SH)

		var b string
		switch {
		case c.CBH > 0 && c.CBH <= MaxSuppliedCrownBaseHeightM:
			b = "supplied, honoured"
			if c.CBH == MaxSuppliedCrownBaseHeightM {
				branch["supplied at the 50 m bound"]++
			}
		case fuel == "C6" && c.SD > 0 && c.SH > 0 && c.SD <= MaxStandDensityStemsHa && c.SH <= MaxStandHeightM:
			b = "C6 stand equation"
			if cbh == MinCrownBaseHeightM {
				branch["C6 stand equation, floored"]++
			}
		case fuel == "C6" && c.SD > 0 && c.SH > 0:
			b = "C6 stand screened out to the table"
		default:
			b = "table"
			tableByFuel[fuel]++
			if c.CBH != -1 {
				branch["table via a trigger other than -1"]++
			}
		}
		branch[b]++

		got := CriticalSurfaceIntensity(c.FMC, cbh)
		if rel := relErr(got, c.CSI); rel > worst {
			worst = rel
		}
		if !closeEnough(got, c.CSI, tol) {
			if bad++; bad <= 10 {
				t.Errorf("case %d %s cbh=%v sd=%v sh=%v fmc=%v [%s]: CBH %v gives CSI %v, cffdrs %v",
					i, c.Fuel, c.CBH, c.SD, c.SH, c.FMC, b, cbh, got, c.CSI)
			}
		}
	}
	if bad > 10 {
		t.Errorf("... and %d more", bad-10)
	}

	t.Logf("CBH  %5d cases through eq. 56, %d mismatched, worst relative error %.3g", len(f.Cases), bad, worst)
	logCounts(t, "  ", branch)
	logCounts(t, "  table, ", tableByFuel)

	for _, b := range []string{
		"supplied, honoured", "supplied at the 50 m bound", "table",
		"table via a trigger other than -1", "C6 stand equation",
		"C6 stand equation, floored", "C6 stand screened out to the table",
	} {
		if branch[b] == 0 {
			t.Errorf("no case reaches %q — that branch of crown_base_height is unasserted", b)
		}
	}
	if len(tableByFuel) < len(Fuels) {
		t.Errorf("only %d of %d fuels reach the table — the rest of it is unasserted", len(tableByFuel), len(Fuels))
	}
}

// ledger: crown_fuel_load.r
func TestCFFDRSCrownFuelLoad(t *testing.T) {
	f := loadCFFDRS(t)
	const tol = 1e-9
	requireColumn(t, f, "sd/sh", func(c cffdrsCase) bool { return c.SD != 0 || c.SH != 0 })

	branch := map[string]int{}
	crowningByFuel := map[string]int{}
	gated := map[string]int{}
	bad := 0
	var worst float64
	for i, c := range f.Cases {
		fuel := ourFuel(c.Fuel)
		cfl := CrownFuelLoad(fuel, c.CFL)

		b := "table"
		switch {
		case c.CFL > 0 && c.CFL <= MaxSuppliedCrownFuelLoadKgM2:
			b = "supplied, honoured"
			if c.CFL == MaxSuppliedCrownFuelLoadKgM2 {
				branch["supplied at the 2 kg/m² bound"]++
			}
		case c.CFL != -1:
			branch["table via a trigger other than -1"]++
		}
		branch[b]++

		// fbp()'s gate: CFB <- ifelse(CFL > 0, CFB, 0). A zero load can only
		// come from the table here, and it must have zeroed the crown.
		if cfl == 0 {
			gated[fuel]++
			if c.CFB != 0 || c.CFC != 0 {
				if bad++; bad <= 10 {
					t.Errorf("case %d %s cfl=%v: table CFL is 0, but cffdrs cfb = %v, cfc = %v",
						i, c.Fuel, c.CFL, c.CFB, c.CFC)
				}
			}
			continue
		}
		if c.CFB > 0 && b == "table" {
			crowningByFuel[fuel]++
		}
		cfc := CrownFuelConsumption(fuel, cfl, c.CFB, c.PC, c.PDF)
		tfc := TotalFuelConsumption(fuel, cfl, c.CFB, c.SFC, c.PC, c.PDF)
		if rel := relErr(cfc, c.CFC); rel > worst {
			worst = rel
		}
		if !closeEnough(cfc, c.CFC, tol) || !closeEnough(tfc, c.TFC, tol) {
			if bad++; bad <= 10 {
				t.Errorf("case %d %s cfl=%v cfb=%v [%s]: CFL %v gives CFC %v / TFC %v, cffdrs %v / %v",
					i, c.Fuel, c.CFL, c.CFB, b, cfl, cfc, tfc, c.CFC, c.TFC)
			}
		}
	}
	if bad > 10 {
		t.Errorf("... and %d more", bad-10)
	}

	t.Logf("CFL  %5d cases through eqs. 66/67 or the CFL > 0 gate, %d mismatched, worst relative error %.3g",
		len(f.Cases), bad, worst)
	logCounts(t, "  ", branch)
	logCounts(t, "  crowning on the table, ", crowningByFuel)
	logCounts(t, "  zero table load, gated, ", gated)

	for _, b := range []string{
		"supplied, honoured", "supplied at the 2 kg/m² bound", "table", "table via a trigger other than -1",
	} {
		if branch[b] == 0 {
			t.Errorf("no case reaches %q — that branch of crown_fuel_load is unasserted", b)
		}
	}
	// The CFL table only shows in CFC where the row crowns, so every fuel with a
	// non-zero entry has to crown somewhere on the table for its entry to be
	// asserted; and every zero entry has to be seen gating.
	for code, load := range crownFuelLoadTable {
		if load > 0 && crowningByFuel[code] == 0 {
			t.Errorf("%s never crowns on its table CFL — its entry %v is unasserted", code, load)
		}
		if load == 0 && gated[code] == 0 {
			t.Errorf("%s never reaches the CFL > 0 gate on its table load of 0", code)
		}
	}
}

// requireColumn fails with a regeneration message when no case carries a column
// the test depends on — a fixture older than the column decodes it as 0 on every
// row, which would otherwise pass as "the branch is never taken".
func requireColumn(t *testing.T, f cffdrsFixture, name string, present func(cffdrsCase) bool) {
	t.Helper()
	for _, c := range f.Cases {
		if present(c) {
			return
		}
	}
	t.Fatalf("the fixture has no %s column: it predates the generator block that emits it "+
		"(testdata/README.md lists it among the earlier digests). Regenerate it.", name)
}

func logCounts(t *testing.T, prefix string, m map[string]int) {
	t.Helper()
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("%s%-40s %5d", prefix, k, m[k])
	}
}
