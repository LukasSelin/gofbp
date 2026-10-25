// Package fbp implements the Canadian Forest Fire Behaviour Prediction (FBP)
// System: Predict for one stand and PredictGrid over a grid, composed from the
// equations in the packages below it.
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
// # Packages
//
// The equations live in four packages, each importing only fuel and `math`:
//
//	fuel         the fuel types: the coefficient table and CanonicalFuelCode
//	spread       RSI, BE, SF, ISI, the slope back-solve, ROS, the fire ellipse
//	             and the acceleration model
//	crown        the crown-fire threshold (CSI, RSO, CFB, FD), C6's crown rate,
//	             foliar moisture content and the per-fuel CBH/CFL tables
//	consumption  surface, crown and total fuel consumption, and fire intensity
//
// This package composes them the way cffdrs' fbp() does — Predict (predict.go)
// for one stand, PredictGrid (grid.go) for fbpRaster()'s layers — and applies
// none of crown's opt-in lookups unless the Input names them. The FWI System is
// package fwi, and nothing here imports it.
//
// # The published system and nothing else
//
// These packages are the published system and only the published system:
// ST-X-3's equations and coefficient tables, with no local adaptation, no
// defaults chosen for a particular country, and no judgement about what the
// ground is made of. That is a deliberate constraint, not an accident of layout —
// the claim "this is the Canadian FBP System, unmodified" is only checkable if
// the packages making it contain nothing else.
//
// Every local decision — which fuel type a stand maps to, what curing to assume,
// what to do with wetland — belongs to the caller, which supplies the fuel code
// these packages' functions take as an argument. If you find yourself adding a
// coefficient, a threshold or a default here because of something about a
// particular landscape, it belongs there instead. Package fuel says what an
// unimplemented fuel code comes back as, and why the caller has to screen for it.
//
// # The oracle
//
// Every equation is checked against TestCFFDRS*, the cffdrs R package — the
// Canadian Forest Service's own implementation. Regenerate the reference with:
//
//	testdata/regen-cffdrs.sh
//
// which pins R and cffdrs in a container, so it needs Docker and nothing else.
// It falls back to a local Rscript when one has cffdrs installed. The fixture is
// not committed; the TestCFFDRS* tests skip when it is absent.
package fbp
