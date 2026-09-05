// Package fbp implements the Canadian Forest Fire Behaviour Prediction (FBP)
// System's head-fire rate of spread.
//
// What it produces is a rate of spread in **metres per minute** — a physical
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
// # Scope
//
// Implemented: RSI (initial spread), the buildup effect BE, the slope factor SF,
// the Initial Spread Index ISI, the equivalent-wind slope back-solve
// (EquivalentWind / NetEffectiveWind, giving WSE/WSV/RAZ), ROS, and the fire
// ellipse (LengthToBreadth, BackISIRatio, FlankROS, ROSAtAngle).
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
//	wind driving upslope   median 1.14x, worst 6.54x
//	wind cross-slope       median 1.80x
//	wind opposing slope    median 4.18x, p95 84x, worst 99x
//
// At zero wind the two usually agree exactly — with nothing to vector-add, the
// back-solve is an identity. Two exceptions, and neither is exotic: mixedwood
// never satisfies it (eq. 42 blends ISF, not RSF), and neither does any fuel
// where isfClampMin or EquivalentWindCapKmh binds, which on dry steep ground is
// most of them. Both leave ROS reading high.
//
// So ROS is an upper bound, never an under-estimate, and a caller falling back
// to it is trading accuracy for availability in one direction only. Note the
// upslope column: even with the wind helping, ROS is not a safe stand-in on
// steep dry ground.
//
// NOT implemented: crown fire. 3689 of the fixture's 11500 cases carry CFB != 0
// and are excluded from every assertion here, so above the crowning threshold
// this package reports surface spread only. That is now the largest remaining
// gap between it and the published system.
//
// Coefficients are the published FBP tables (Forestry Canada Fire Danger Group
// 1992, ST-X-3, Tables 6–7), with the grass curing revision from Wotton,
// Alexander & Taylor (2009) — see CuringFactor. They are transcribed by hand and
// checked against TestCFFDRS*, the cffdrs R package — the Canadian Forest
// Service's own implementation, and the only oracle here that can say the tables
// are right rather than merely self-consistent. RSI, BE and SF reproduce it
// exactly across all 15 fuel types; ISI, WSV and the full slope path reproduce it
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
// Regenerate the reference with:
//
//	testdata/regen-cffdrs.sh
//
// which pins R and cffdrs in a container, so it needs Docker and nothing else.
// It falls back to a local Rscript when one has cffdrs installed. The fixture is
// not committed; the TestCFFDRS* tests skip when it is absent.
package fbp
