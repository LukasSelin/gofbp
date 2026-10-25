// Package spread is the FBP System's rate of spread: RSI (initial spread), the
// buildup effect BE, the slope factor SF, the Initial Spread Index ISI, the
// equivalent-wind slope back-solve (EquivalentWind / NetEffectiveWind, giving
// WSE/WSV/RAZ), ROS, the fire ellipse (LengthToBreadth, BackISIRatio, FlankROS,
// ROSAtAngle), and the acceleration model (ROSAtTime, DistanceAtTime,
// LengthToBreadthAtTime).
//
// # Slope
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
// ROS is the surface rate for all seventeen fuels, C6 included: C6's crown rate
// of spread is package crown's C6CrownFire, because ROS's signature has nowhere
// to put the FMC, SFC, CBH and CFL its blend needs.
//
// # Coefficients and the oracle
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
package spread
