// Package fuel is the FBP System's fuel types: the coefficient table Fuels and
// CanonicalFuelCode, which folds a caller's spelling of a fuel code onto its key.
// Every other FBP package takes a fuel code as a string and reads its
// coefficients from here.
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
// parameters throughout — a fuel map carrying both carries them in different
// columns, and one parameter serving both would turn a transposed column into a
// plausible number instead of a compile error.
//
// NOT implemented: D2, and cffdrs' non-fuel classes WA and NF.
//
// That absence needs an active decision from the caller, because the API cannot
// make it. Every FBP function takes a fuel code and returns a float64, so an
// unimplemented fuel comes back as a spread rate of 0 — indistinguishable from a
// cell that genuinely will not carry fire. Screen fuel codes with
// CanonicalFuelCode where classes enter, once per class rather than once per
// cell, and decide there what an unimplemented fuel means for the output. Left
// to the numbers, the answer is 0 and the reason is gone.
package fuel
