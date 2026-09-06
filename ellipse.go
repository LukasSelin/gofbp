package fbp

import "math"

// The fire ELLIPSE: what the head-fire rate alone cannot say.
//
// ROS is the rate at the head — the single fastest direction. A fire is not a
// point moving that way; under a steady wind its perimeter grows as an ellipse
// whose long axis lies along the net effective wind. The head rate is one point
// on that perimeter, and for a caller asking "how fast is this coming at ME"
// it is the wrong point almost everywhere: a fire two kilometres away whose head
// runs the other way arrives at the BACK rate, which is an order of magnitude
// slower.
//
// This file adds the three published quantities that close that gap — the
// length-to-breadth ratio LB, the back rate BROS and the flank rate FROS — plus
// the ellipse geometry that turns them into a rate in an arbitrary direction.
//
// Same rule as the rest of this package: the published system and nothing else.
// LB, BROS and FROS are asserted against cffdrs in cffdrs_test.go. The ellipse
// geometry in ROSAtAngle is NOT oracled against cffdrs — see its doc for why,
// and for the identities that pin it instead.

// BackISIRatio is BISI/ISI at the same FFMC and net effective wind: the factor
// that converts a forward Initial Spread Index into the backing one.
//
// FBP's backing ISI replaces the forward wind function fW with a decaying one,
// BFW = exp(-0.05039·WSV) (FCFDG 1992 eqs. 46-47), leaving the fine-fuel-moisture
// term fF untouched. So the two differ by nothing but their wind functions, and
// fF cancels exactly in the ratio.
//
// That cancellation is the point. A caller anchored on a published ISI, which
// never recomputes it from FFMC, can use this to express "the same ISI, backing"
// without ever forming an ISI of its own — the same trick, and the same
// justification, as the forward ratio in ISI's doc.
//
// Note BFW has no high-wind branch: unlike windFunction it is the plain
// exponential at every wind speed. That asymmetry is in the published system.
func BackISIRatio(wsvKmh float64) float64 {
	fw := windFunction(wsvKmh)
	if fw <= 0 {
		return 0
	}
	return math.Exp(-0.05039*wsvKmh) / fw
}

// LengthToBreadth is the fire's length-to-breadth ratio at a net effective wind
// of wsvKmh km/h: ST-X-3 eq. 79 for the wooded fuels, eqs. 80-81 for the two
// grass types, which are measurably more elongated at the same wind.
//
// It is 1.0 at zero wind (a circle) and grows without practical bound with it.
// Everything about the ellipse's SHAPE is this number; BROS supplies its
// position along the head axis.
//
// The code is folded by CanonicalFuelCode. An unimplemented one takes eq. 79,
// the wooded form, because that is what every fuel but the two grasses uses —
// see RSI's doc for why that silent fallback is the caller's to screen for.
func LengthToBreadth(code string, wsvKmh float64) float64 {
	if wsvKmh < 0 {
		wsvKmh = 0
	}
	canonical, _ := CanonicalFuelCode(code)
	switch canonical {
	case "O1A", "O1B":
		// Eq. 80/81. Below 1 km/h the grass form dips under 1.0, which is not a
		// ratio a fire can have, so the published system pins it.
		if wsvKmh >= 1.0 {
			return 1.1 * math.Pow(wsvKmh, 0.464)
		}
		return 1.0
	}
	// Eq. 79.
	return 1.0 + 8.729*math.Pow(1-math.Exp(-0.030*wsvKmh), 2.155)
}

// FlankROS is the flank rate of spread in m/min: ST-X-3 eq. 89, the half-width
// of the ellipse at its widest point.
//
// It is measured at the ELLIPSE CENTRE, not at the ignition point, which is why
// ROSAtAngle(…, 90) is smaller than this rather than equal to it. That is
// geometry, not a discrepancy — see ROSAtAngle.
func FlankROS(rosHead, rosBack, lb float64) float64 {
	if lb <= 0 {
		return 0
	}
	return (rosHead + rosBack) / (2 * lb)
}

// ROSAtAngle is the rate of spread in m/min at thetaDeg degrees off the head
// direction, measured FROM THE IGNITION POINT.
//
// The FBP fire perimeter after time t is an ellipse: it reaches rosHead·t
// forward, rosBack·t backward, and is rosFlank·t wide at its half-length. Those
// three fix the ellipse completely — semi-major a = (rosHead+rosBack)/2, semi-
// minor b = rosFlank, centre displaced h = (rosHead-rosBack)/2 ahead of the
// ignition point — and the rate at angle theta is the distance from the ignition
// point to the perimeter along that bearing, per unit time. Solving the ray
// against the ellipse is the quadratic below; everything scales linearly in t,
// so t drops out.
//
// theta = 0 returns rosHead and theta = 180 returns rosBack, exactly, by
// construction. The rate does NOT fall monotonically between them: on an
// elongated fire the minimum is the FLANK, because a long thin ellipse spreads
// faster backwards along its own axis than sideways across it, so the curve
// turns back up somewhere before 180 degrees. That happens on about 9 % of the
// oracle fixture's cases and it is the geometry, not a defect — see
// TestROSAtAngleIsUnimodalWithTheHeadAsMaximum, which asserts the shape that
// actually holds (single interior minimum, head is the global maximum).
//
// theta = 90 returns b·sqrt(1-(h/a)²), which is LESS than
// rosFlank: the flank rate is the ellipse's half-width at its own centre, while
// this is its half-width at the ignition point, and the ignition point sits
// behind the centre. Reading that gap as a bug and "fixing" it is the mistake
// this paragraph exists to prevent.
//
// Unlike LB, BROS and FROS, this geometry has no cffdrs column to assert
// against — fbp() returns the ellipse's parameters, not a rate at an arbitrary
// bearing. It is pinned instead by the two exact identities above plus monotone
// decrease from head to back (TestROSAtAngle*), which together determine the
// curve at its endpoints and forbid every plausible transcription error in
// between.
func ROSAtAngle(rosHead, rosFlank, rosBack, thetaDeg float64) float64 {
	if rosHead <= 0 {
		return 0
	}
	// A degenerate ellipse (no width) is a line: the fire only moves along the
	// head axis, so anything off it gets nothing.
	if rosFlank <= 0 {
		switch {
		case math.Mod(math.Abs(thetaDeg), 360) == 0:
			return rosHead
		case math.Mod(math.Abs(thetaDeg), 360) == 180:
			return rosBack
		}
		return 0
	}

	a := (rosHead + rosBack) / 2 // semi-major, along the head axis
	b := rosFlank                // semi-minor
	h := (rosHead - rosBack) / 2 // ignition point sits h behind the centre
	if a <= 0 {
		return 0
	}

	rad := thetaDeg * math.Pi / 180
	sinT, cosT := math.Sincos(rad)

	// ((r·cosθ - h)/a)² + (r·sinθ/b)² = 1, solved for the positive root.
	qa := cosT*cosT/(a*a) + sinT*sinT/(b*b)
	qb := -2 * h * cosT / (a * a)
	qc := h*h/(a*a) - 1

	disc := qb*qb - 4*qa*qc
	if disc < 0 || qa <= 0 {
		return 0
	}
	r := (-qb + math.Sqrt(disc)) / (2 * qa)
	if r < 0 {
		return 0
	}
	return r
}

// AngleBetweenDeg is the unsigned angular separation between two bearings, in
// [0, 180]. It is what turns "the fire heads 210°, my location is at 40°" into
// theta ROSAtAngle wants.
//
// Unsigned because the ellipse is symmetric about its head axis: 30° left of the
// head and 30° right of it spread at the same rate.
func AngleBetweenDeg(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), 360)
	if d > 180 {
		d = 360 - d
	}
	return d
}
