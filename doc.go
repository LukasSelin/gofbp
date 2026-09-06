// Package fbp implements the Canadian Forest Fire Behaviour Prediction (FBP)
// System's head-fire rate of spread.
//
// What it produces is a rate of spread in metres per minute — a physical
// quantity, not a danger score. Keeping the two apart matters: a fire-danger
// index is a unitless number whose range is tied to whatever scale published it,
// while a spread rate has units that make it checkable against observed fire
// behaviour. They answer different questions — "how dangerous are conditions
// today" versus "if it starts here, how fast does it move" — and multiplying one
// by the other destroys both. Do not fold this into an index.
//
// Inputs are ISI and BUI from any FWI System source, a fuel type chosen by the
// caller, and slope, aspect, wind and FFMC.
//
// # The published system and nothing else
//
// This package is the published system and only the published system: ST-X-3's
// equations and coefficient tables, with no local adaptation, no defaults chosen
// for a particular country, and no judgement about what the ground is made of.
// That is a deliberate constraint, not an accident of layout — the claim "this is
// the Canadian FBP System, unmodified" is only checkable if the package making it
// contains nothing else.
//
// Every local decision — which fuel type a stand maps to, what curing to assume,
// what to do with wetland — belongs to the caller, which supplies the fuel code
// this package's functions take as an argument. If you find yourself adding a
// coefficient, a threshold or a default here because of something about a
// particular landscape, it belongs there instead.
//
// # Fuel codes
//
// Implemented: C1-C7, D1, M1-M4, S1-S3, O1A and O1B — the fifteen fuel types of
// ST-X-3 plus M3 and M4, the dead-balsam-fir mixedwoods from Wotton, Alexander &
// Taylor (2009). That is every fuel cffdrs implements. Codes are folded by
// CanonicalFuelCode, so case and separators do not matter: "O1a", "o1b" and
// "C-2" reach the same coefficients as "O1A", "O1B" and "C2". The lowercase
// grass spellings are the ones ST-X-3 itself prints, and a raster labelled the
// way the source document labels it must not read as a different fuel from one
// labelled the way Fuels is keyed.
//
// The mixedwoods take two different blend inputs and they are not
// interchangeable. M1/M2 are weighted by percent conifer PC (eq. 27, against the
// C2 curve); M3/M4 by percent dead balsam fir PDF (eqs. 29/33, against the
// fuel's own eq. 30 curve). Both are "how much of this stand is the flammable
// component" and both are percentages, which is exactly why they are separate
// parameters here — a fuel map carrying both carries them in different columns,
// and one parameter serving both would turn a transposed column into a plausible
// number instead of a compile error.
//
// NOT implemented: D2, and cffdrs' non-fuel classes WA and NF.
//
// That absence needs an active decision from the caller, because the API cannot
// make it. Every function here takes a fuel code and returns a float64, so an
// unimplemented fuel comes back as a spread rate of 0 — indistinguishable from a
// cell that genuinely will not carry fire. Screen fuel codes with
// CanonicalFuelCode where classes enter, once per class rather than once per
// cell, and decide there what an unimplemented fuel means for the output. Left
// to the numbers, the answer is 0 and the reason is gone.
//
// # Scope
//
// Implemented: CanonicalFuelCode (fuel code folding and validation), RSI
// (initial spread), the buildup effect BE, the slope factor SF,
// the Initial Spread Index ISI, the equivalent-wind slope back-solve
// (EquivalentWind / NetEffectiveWind, giving WSE/WSV/RAZ), ROS, the fire
// ellipse (LengthToBreadth, BackISIRatio, FlankROS, ROSAtAngle), and the
// crown-fire threshold (CriticalSurfaceIntensity, CriticalSurfaceROS,
// CrownFractionBurned, DescribeFire, giving CSI/RSO/CFB/FD).
//
// Full FBP does not treat slope as a multiplier at all. It routes slope through a
// zero-wind spread rate RSZ, scales it by SF to get RSF, back-solves the ISI (and
// hence the equivalent wind speed WSE) that would produce RSF, then vector-adds
// WSE in the upslope direction to the real wind before recomputing ROS from the
// net effective wind:
//
//	RSI(ISI(FFMC, WSV)) · BE      the published slope path
//	ROS = RSI · BE · SF           a documented upper bound
//
// The first has no function of its own here — see the note in slopewind.go for
// why, and slopeAdjustedROS in cffdrs_test.go for the composition and its
// assertion against the oracle. It is what lets this package express wind–slope
// alignment: whether the wind is driving up the slope or fighting it.
//
// ROS is kept because the back-solve needs FFMC and wind, and a caller that has
// neither cannot run it. It is the honest degraded-mode answer, and
// TestCFFDRSSlopeDivergence is now the specification of exactly how wrong it is.
// The error is entirely a wind-DIRECTION effect, because RSI · BE · SF applies
// the full slope factor no matter which way the wind blows:
//
//	wind driving upslope   median 1.28x, p95 5.91x, worst 7.61x
//	wind cross-slope       median 2.02x, p95 5.95x, worst 7.77x
//	wind opposing slope    median 3.29x, p95 67x,   worst 99x
//
// At zero wind the two usually agree exactly — with nothing to vector-add, the
// back-solve is an identity. Two exceptions, and neither is exotic: mixedwood
// never satisfies it (eqs. 42/42b/42c blend ISF, not RSF), and neither does any fuel
// where isfClampMin or EquivalentWindCapKmh binds, which on dry steep ground is
// most of them. Both leave ROS reading high.
//
// So ROS is an upper bound, never an under-estimate, and a caller falling back
// to it is trading accuracy for availability in one direction only. Note the
// upslope column: even with the wind helping, ROS is not a safe stand-in on
// steep dry ground.
//
// # Crown fire
//
// The THRESHOLD is implemented — critical surface intensity CSI, the surface
// spread rate RSO that reaches it, crown fraction burned CFB, and the fire
// description FD. See crown.go.
//
// What that does and does not change is worth being exact about, because the
// natural assumption is wrong in both directions. It does not raise any spread
// rate: in the published system the final rate of spread IS the surface rate for
// every fuel type except C6, which alone has a separate crown rate of spread
// folded in through CFB. So for sixteen of the seventeen fuels here, a crowning
// stand's ROS was already right, and CFB is the missing statement of what kind of
// fire that rate describes — surface, intermittent crown, or continuous crown.
// That statement was the actual gap, not an arithmetic one.
//
// Not implemented, and all of it caller-supplied instead: foliar moisture content
// FMC (from latitude, longitude, elevation and date), surface fuel consumption
// SFC (from FFMC and BUI per fuel), and the published per-fuel CBH and CFL
// default tables. The last is the one with teeth — without those tables a caller
// has no source for crown base height or crown fuel load inside this package and
// must bring its own, and CFL in particular is what keeps the fuels with no crown
// (D1, S1-S3, O1A, O1B) reporting zero.
//
// Also not implemented: C6's crown rate of spread RSC, so C6's ROS here remains
// surface-only and every oracle test excludes it by name; and the consumption and
// intensity outputs CFC, TFC and HFI, which depend on CFB but are a separate
// quantity from spread.
//
// Coefficients are the published FBP tables (Forestry Canada Fire Danger Group
// 1992, ST-X-3, Tables 6–7), with the grass curing revision from Wotton,
// Alexander & Taylor (2009) — see CuringFactor. They are transcribed by hand and
// checked against TestCFFDRS*, the cffdrs R package — the Canadian Forest
// Service's own implementation, and the only oracle here that can say the tables
// are right rather than merely self-consistent. RSI, BE and SF reproduce it
// exactly across all 17 fuel types; ISI, WSV and the full slope path reproduce it
// to machine precision over every sloped case. It is what caught the missing curing
// branch, which had this package's fully-cured grass spreading 2.2x too fast, and
// it is what caught the FFMC moisture constant being the rounded 147.2 rather than
// the exact 250·59.5/101 — see ffmcMoistureScale.
//
// The sloped sweep covers M1/M2 and FFMC 85 and 95, so the three branches that
// are easiest to get wrong and hardest to reach — the M1/M2 ISF blend, the
// high-wind inverse above HighWindKmh, and the EquivalentWindCapKmh saturation —
// are checked against the oracle rather than against our own reasoning.
//
// The crown sweep carries explicit crown base heights and a latitude/day-of-year
// spread, because CBH and FMC are the only handles on eqs. 56 and 58 and a
// fixture at one of each would reproduce them at a point rather than oracle them.
// CSI, RSO, CFB and FD all reproduce cffdrs to machine precision there, CFB both
// from cffdrs' own spread rate and end to end from this package's.
//
// Regenerate the reference with:
//
//	testdata/regen-cffdrs.sh
//
// which pins R and cffdrs in a container, so it needs Docker and nothing else.
// It falls back to a local Rscript when one has cffdrs installed. The fixture is
// not committed; the TestCFFDRS* tests skip when it is absent.
package fbp
