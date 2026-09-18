package fbp

import "math"

// FOLIAR MOISTURE CONTENT: how wet the needles overhead are, on a given day, in
// a given place.
//
// This is the crown-fire threshold's other half. crown.go's eq. 56 reads two
// things — how far the flames have to reach (CBH) and how much energy the
// foliage will absorb before it ignites (FMC) — and only the first is something
// a caller measures. FMC is not measured at all in the FBP System: it is
// modelled, from latitude, longitude, elevation and the day of year, on the
// observation that conifer foliage dries to an annual minimum on a date that
// moves with the site and recovers on a fixed curve either side of it.
//
// So the whole model is "how many days is today from this stand's driest day",
// and the answer runs from 85 % at the minimum to a flat 120 % fifty days out.
// Over that range eq. 56's (460 + 25.9·FMC) term moves by a factor of 1.6, which
// is the difference between a stand that crowns this afternoon and one that does
// not.
//
// # Shape of the published system
//
// Eight equations in two stages. Eqs. 1-4 find D0, the date of the minimum:
// normalise the latitude for longitude (the drying date runs east to west across
// the continent), then convert that to a day of the year, with a separate pair of
// coefficients when elevation is known because height delays the minimum. Eqs.
// 5-8 are then a single curve in ND = |DJ - D0|: a shallow quadratic rise over
// the first thirty days, a steeper one for the next twenty, and a plateau after
// that.
//
// # What this file does NOT do
//
// It does not default anything. FMC is not given a value here for a caller who
// has no date, and Crown.FMC is still the caller's field — this gives them
// something to fill it with, the way SurfaceFuelConsumption does for Crown.SFC.
// It does not zero FMC for the fuels that have no crown either; that is fbp()'s
// doing and is discussed on FoliarMoistureContent.
//
// It does not fold the sign of a longitude. See FoliarMoistureContent.

// The FMC coefficients, by equation number.
//
// Grouped by branch rather than tabulated: eqs. 1/2 and eqs. 3/4 are the same
// two-step construction with different constants, and eqs. 6/7/8 are three
// pieces of one curve.
const (
	// Eqs. 1, 2 (FCFDG 1992) — the elevation-not-known branch, which cffdrs
	// selects on ELV <= 0. See the note on DateOfMinimumFoliarMoisture.
	latnFlatBase = 46.0
	latnFlatAmp  = 23.4
	latnFlatRate = 0.0360
	d0FlatScale  = 151.0

	// Eqs. 3, 4 (FCFDG 1992) — the elevation-known branch.
	latnElevBase = 43.0
	latnElevAmp  = 33.7
	latnElevRate = 0.0351
	d0ElevScale  = 142.1
	d0ElevPerM   = 0.0172

	// The meridian the normalisation is measured from, in DEGREES WEST — see
	// FoliarMoistureContent on the sign convention.
	latnReferenceLongitude = 150.0

	// Eq. 6 (FCFDG 1992) — the first thirty days either side of the minimum.
	fmcMinimumPct = 85.0
	fmcNearRate   = 0.0189

	// Eq. 7 (FCFDG 1992) — days 30 to 50.
	fmcMidBase   = 32.9
	fmcMidLinear = 3.17
	fmcMidSquare = 0.0288

	// The branch points of eqs. 6, 7 and 8, in days from the minimum.
	fmcNearDays = 30.0
	fmcFarDays  = 50.0
)

// MaxFoliarMoisturePct is eq. 8: the moisture content of foliage far enough from
// the annual minimum that the model stops distinguishing dates at all.
//
// It is also the ceiling of the whole model — no combination of inputs produces
// more — which is why cffdrs' fbp() reads a caller-supplied FMC above 120 as
// missing and computes its own instead.
const MaxFoliarMoisturePct = 120.0

// DateOfMinimumFoliarMoisture is D0, a day of the year (ST-X-3 eqs. 1-4): the
// date on which foliar moisture content reaches its annual minimum at a site.
//
// The two-step construction is worth reading once. Eqs. 1 and 3 do not produce a
// latitude of anything: LATN is a NORMALISING latitude, an east-to-west
// correction that encodes how much later spring arrives inland than it does on
// the coast. Eqs. 2 and 4 then take the site's real latitude as a fraction of
// that and scale it into a day of the year, so a site sitting exactly at its own
// normalising latitude minimises on day 151 (or on 142.1 plus the elevation
// term), and one further north minimises proportionally later.
//
// Longitude is DEGREES WEST and positive — see FoliarMoistureContent.
//
// # Where the R disagrees with the published paper
//
// Two places, and neither is in the arithmetic:
//
//  1. ST-X-3 selects between the two branches on whether elevation is KNOWN:
//     eqs. 1/2 where it is not, eqs. 3/4 where it is. cffdrs cannot ask that
//     question of a numeric column, so it selects on ELV <= 0 — which turns "no
//     elevation data" into "elevation is zero or below", and quietly puts every
//     site at or under sea level on the branch meant for missing data. A caller
//     with a real, measured elevation of 0 m gets the eq. 1/2 answer. This is
//     reproduced here because parity with the oracle is what this package is for,
//     and it is why elev is documented below as a sentinel rather than as a
//     measurement.
//
//  2. The paper does not round D0. cffdrs does — "because it is a date" — and
//     the rounding is R's, which is half-to-EVEN rather than half-away-from-zero.
//     math.Round would differ on an exact half, so this uses math.RoundToEven.
//     Half a day of D0 is up to a 0.77-percentage-point step in FMC at the eq. 6
//     end: small, but perfectly systematic. It is asserted rather than assumed —
//     the fixture carries sites whose D0 lands on an exact half.
//
// Returns NaN for a non-finite input rather than letting an infinity through the
// exponential, where it would arrive as a D0 of 0 and read as a real date.
func DateOfMinimumFoliarMoisture(lat, long, elev float64) float64 {
	if math.IsNaN(lat) || math.IsNaN(long) || math.IsNaN(elev) {
		return math.NaN()
	}
	if math.IsInf(lat, 0) || math.IsInf(long, 0) || math.IsInf(elev, 0) {
		return math.NaN()
	}

	var d0 float64
	if elev <= 0 {
		// Eqs. 1, 2.
		latn := latnFlatBase + latnFlatAmp*math.Exp(-latnFlatRate*(latnReferenceLongitude-long))
		d0 = d0FlatScale * (lat / latn)
	} else {
		// Eqs. 3, 4.
		latn := latnElevBase + latnElevAmp*math.Exp(-latnElevRate*(latnReferenceLongitude-long))
		d0 = d0ElevScale*(lat/latn) + d0ElevPerM*elev
	}
	return math.RoundToEven(d0)
}

// FoliarMoistureContent is FMC as a percentage (ST-X-3 eqs. 1-8): the moisture
// content of the conifer foliage overhead on day dj at the given site.
//
// The arguments are in the reference implementation's own order — LAT, LONG,
// ELV, DJ, D0 — so the transcription can be read against it line for line.
//
// Feed the result to Crown.FMC. This function does not fill that field, for the
// same reason SurfaceFuelConsumption does not fill Crown.SFC: a caller working
// outside North America has to override the model entirely, and a package that
// filled the field would have decided otherwise on their behalf.
//
// # The five inputs, and their sentinels
//
//	lat   decimal degrees north.
//	long  decimal degrees WEST, POSITIVE. That is the paper's convention and the
//	      one eqs. 1 and 3 are written in — (150 - LONG) is degrees east of the
//	      150th meridian. cffdrs' fbp() takes the absolute value of its LONG
//	      column before calling this, so a caller who hands IT the conventional
//	      signed longitude (-120 for 120 °W) gets the same answer. This function
//	      does NOT fold the sign, because doing so would silently map 120 °E onto
//	      120 °W, and this package has no driver whose job it is to normalise
//	      inputs. Pass math.Abs(long) to match fbp().
//	elev  metres. NOT a measurement — see DateOfMinimumFoliarMoisture. A
//	      non-positive value selects the branch the paper meant for "elevation
//	      unknown".
//	dj    day of the year.
//	d0    day of the year of the annual minimum, where the caller knows it.
//	      Non-positive means "work it out", which is what every caller without a
//	      local phenology record passes.
//
// # Where the R disagrees with the published paper
//
// Beyond the two on DateOfMinimumFoliarMoisture, one more — and this one is a
// disagreement between cffdrs VERSIONS rather than with the paper:
//
//	cffdrs 1.9.2, the version this package's oracle is pinned to, rounds the
//	CALLER-SUPPLIED d0 as well, because its round() sits after the ifelse that
//	chooses between the computed date and the given one. Upstream's git HEAD
//	(4d20a30) splits the two stages into separate functions and rounds only
//	inside the one that computes D0, so a supplied d0 of 150.7 is used as 151 by
//	1.9.2 and as 150.7 by HEAD. Every other number the two produce is identical,
//	which is checked rather than assumed — see MIGRATION.md.
//
//	1.9.2's behaviour is what is reproduced here, because 1.9.2 is what the
//	fixture is generated from and is the newest cffdrs on CRAN. An integer d0 —
//	the only kind a day of the year sensibly is — is unaffected either way.
//
// # Non-finite input
//
// NaN propagates. An infinity returns NaN rather than being allowed through the
// branch comparisons, where an infinite dj would land on eq. 8's plateau and come
// back as a perfectly ordinary 120.
func FoliarMoistureContent(lat, long, elev, dj, d0 float64) float64 {
	if math.IsNaN(dj) || math.IsInf(dj, 0) {
		return math.NaN()
	}
	if math.IsNaN(d0) || math.IsInf(d0, 0) {
		return math.NaN()
	}

	// cffdrs 1.9.2's ifelse, in its order: choose the date, then round whichever
	// one was chosen. See the version note above — the rounding of a SUPPLIED d0
	// is 1.9.2's and is not in upstream's HEAD.
	if d0 <= 0 {
		d0 = DateOfMinimumFoliarMoisture(lat, long, elev)
		if math.IsNaN(d0) {
			return math.NaN()
		}
	} else {
		d0 = math.RoundToEven(d0)
	}

	nd := math.Abs(dj - d0) // eq. 5
	switch {
	case nd < fmcNearDays:
		return fmcMinimumPct + fmcNearRate*nd*nd // eq. 6
	case nd < fmcFarDays:
		return fmcMidBase + fmcMidLinear*nd - fmcMidSquare*nd*nd // eq. 7
	default:
		return MaxFoliarMoisturePct // eq. 8
	}
}
