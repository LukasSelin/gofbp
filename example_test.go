package fbp_test

import (
	"fmt"

	"github.com/LukasSelin/gofbp"
)

// A boreal spruce stand (C2) on a 20° slope that rises towards the north-east,
// with a 15 km/h wind also blowing towards the north-east — that is, straight up
// the hill.
//
// Note the units, which are the published system's and not the ones weather data
// usually arrives in: wind in km/h rather than m/s, slope as percent RISE rather
// than degrees, and both azimuths as the direction the thing PUSHES TOWARDS
// rather than the meteorological "comes from".
func Example() {
	s := fbp.SlopeWind{
		Code:              "C2",
		FFMC:              92,
		SlopePct:          fbp.SlopePercentFromDegrees(20), // percent rise, not degrees
		WindKmh:           15,                              // km/h, not m/s
		WindAzimuthDeg:    45,                              // the wind pushes towards NE
		UpslopeAzimuthDeg: 45,                              // the ground rises towards NE
		PC:                100,                             // percent conifer; M1/M2 only
		// PDF (percent dead balsam fir, M3/M4 only) and CuringPct (O1A/O1B only)
		// are left at zero: C2 ignores both.
	}
	const bui = 80

	// Slope is not a multiplier. It is back-solved into an equivalent wind and
	// vector-added to the real one, giving the wind that actually drives the head
	// fire and the bearing it runs towards.
	wsv, raz := fbp.NetEffectiveWind(s)

	// The head rate. slopePct is 0 here on purpose: the slope is already inside
	// wsv, and passing it again would count it twice.
	isi := fbp.ISI(s.FFMC, wsv)
	head := fbp.ROS(s.Code, isi, bui, s.PC, s.PDF, s.CuringPct, 0)

	// The head rate is the single fastest direction, which for "how fast is this
	// coming at ME" is the wrong number almost everywhere. The ellipse answers the
	// question properly — here, for something due east of the ignition.
	back := fbp.ROS(s.Code, isi*fbp.BackISIRatio(wsv), bui, s.PC, s.PDF, s.CuringPct, 0)
	lb := fbp.LengthToBreadth(s.Code, wsv)
	flank := fbp.FlankROS(head, back, lb)
	east := fbp.ROSAtAngle(head, flank, back, fbp.AngleBetweenDeg(raz, 90))

	fmt.Printf("net effective wind: %.1f km/h towards %.0f°\n", wsv, raz)
	fmt.Printf("head:  %.2f m/min\n", head)
	fmt.Printf("flank: %.2f m/min\n", flank)
	fmt.Printf("back:  %.2f m/min\n", back)
	fmt.Printf("east:  %.2f m/min\n", east)
	// Output:
	// net effective wind: 30.7 km/h towards 45°
	// head:  44.96 m/min
	// flank: 5.81 m/min
	// back:  0.71 m/min
	// east:  4.65 m/min
}
