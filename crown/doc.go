// Package crown is the FBP System's crown-fire threshold and everything it
// reads: critical surface intensity CSI, the surface spread rate RSO that
// reaches it, crown fraction burned CFB and the fire description FD (crown.go);
// C6's crown rate of spread (c6.go); foliar moisture content FMC (foliar.go);
// and the published per-fuel CBH and CFL tables (crowndefaults.go).
//
// What the threshold does and does not change is worth being exact about,
// because the natural assumption is wrong in both directions. CFB does not raise
// any spread rate: in the published system the final rate of spread IS the
// surface rate for every fuel type except C6, which alone has a separate crown
// rate of spread folded in through CFB. So for sixteen of the seventeen fuels, a
// crowning stand's ROS was already right, and CFB is the missing statement of
// what kind of fire that rate describes — surface, intermittent crown, or
// continuous crown. That statement was the actual gap, not an arithmetic one.
//
// C6 is the seventeenth, and its crown rate of spread RSC is implemented — see
// c6.go, where C6CrownFire returns the CFB-weighted blend of the surface and
// crown rates that the published system reports for that fuel.
//
// Surface fuel consumption SFC (package consumption) and foliar moisture content
// FMC (FoliarMoistureContent) are both implemented, but Crown calls NEITHER.
// Crown.SFC and Crown.FMC stay fields the caller fills, because computing them
// needs a grass fuel load and a location and date that only the caller has.
//
// The published per-fuel CBH and CFL tables are a lookup the caller opts into —
// DefaultCrownBaseHeight, DefaultCrownFuelLoad, and CrownBaseHeight /
// CrownFuelLoad for fbp()'s substitution rules — and Crown calls none of them,
// for the same reason it calls neither of the above: a measured stand must not
// be quietly replaced by a published average. CFL is the one with teeth: it is
// what keeps the fuels with no crown (D1, S1-S3, O1A, O1B) reporting zero, so the
// caller who fills Crown.CFL owns that.
//
// What is not implemented is C6's SLOPED path. It is narrower than it sounds and
// is worth stating, because C6's crown rate of spread landing does not close it.
// cffdrs back-solves the slope-equivalent wind from rate_of_spread(), which for
// C6 returns the BLENDED rate — so its equivalent wind for a sloped C6 stand
// depends on FMC and CBH, which no surface-only inversion reproduces.
// spread.NetEffectiveWind is surface-only for every fuel, so on sloped ground
// C6CrownFire should be fed an ISI the caller trusts rather than one back-solved
// by package spread. On flat ground the two agree exactly; see
// TestCFFDRSC6RateOfSpread.
//
// The crown sweep carries explicit crown base heights and a latitude/day-of-year
// spread, because CBH and FMC are the only handles on eqs. 56 and 58 and a
// fixture at one of each would reproduce them at a point rather than oracle them.
// CSI, RSO, CFB and FD all reproduce cffdrs to machine precision there, CFB both
// from cffdrs' own spread rate and end to end from package spread's.
package crown
