package oracle

import (
	"bytes"
	"math"
)

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

// Formula computes one number for a fixture row. NaN means the formula is not
// defined for that row, which a caller must count rather than compare.
type Formula func(get Getter) float64

// Formulas are the Signatures and References the register names.
//
// A Signature is cffdrs' arithmetic, wrong as it is, written out so a test can
// say "cffdrs still does exactly this". A Reference is the published equation.
// Neither may call into package fbp.
var Formulas = map[string]Formula{
	// fbp()'s TROS, with THETA in radians and RAZ in degrees inside one cos().
	"cffdrs-tros-degree-mix": func(get Getter) float64 {
		ros, lb, raz, theta := Num(get, "ros"), Num(get, "lb"), Num(get, "raz"), Num(get, "theta")
		e := math.Sqrt(1 - 1/lb/lb)
		return ros * (1 - e) / (1 - e*math.Cos(theta*math.Pi/180-raz))
	},
	// rate_of_spread_at_theta(): both leading terms times the bracket, and the
	// cos == 0 nudge it applies first.
	"cffdrs-eq94-misparenthesised": func(get Getter) float64 {
		ros, fros, bros := Num(get, "ros_in"), Num(get, "fros_in"), Num(get, "bros_in")
		t := Num(get, "theta") * math.Pi / 180
		c, s := math.Cos(t), math.Sin(t)
		if c == 0 {
			c = math.Cos(t + 0.001)
		}
		return ((ros-bros)/(2*c) + (ros+bros)/(2*c)) *
			((fros*c*math.Sqrt(fros*fros*c*c+(ros*bros)*s*s) - ((ros*ros-bros*bros)/4)*s*s) /
				(fros*fros*c*c + ((ros+bros)/2)*((ros+bros)/2)*s*s))
	},
	// Eq. 94 as printed, on a row that hands it ROS, FROS, BROS and a bearing
	// relative to the head.
	"glcx10-eq94": func(get Getter) float64 {
		return EQ94Defined(Num(get, "ros_in"), Num(get, "fros_in"), Num(get, "bros_in"), Num(get, "theta"))
	},
	// Eq. 94 on an fbp() row, whose THETA is an azimuth: the bearing from the
	// head is THETA - RAZ.
	"glcx10-eq94-relative-to-raz": func(get Getter) float64 {
		return EQ94Defined(Num(get, "ros"), Num(get, "fros"), Num(get, "bros"), Num(get, "theta")-Num(get, "raz"))
	},
}

// EQ94Published is GLC-X-10 eq. 94 (Wotton, Alexander & Taylor 2009, p. 20)
// transcribed term for term, with c = cos θ and s = sin θ:
//
//	ROS(θ) = (ROS − BROS)/(2c)
//	       + (ROS + BROS)/(2c) × [ FROS·c·(FROS²c² + ROS·BROS·s²)^½ − ((ROS² − BROS²)/4)·s² ]
//	                             / [ FROS²c² + ((ROS + BROS)/2)²·s² ]
//
// Only the SECOND leading term multiplies the bracket. θ is measured from the
// head direction about the ignition point. It is 0/0 at exactly 90° and 270°,
// as the paper says.
func EQ94Published(ros, fros, bros, thetaDeg float64) float64 {
	t := thetaDeg * math.Pi / 180
	c, s := math.Cos(t), math.Sin(t)
	num := fros*c*math.Sqrt(fros*fros*c*c+ros*bros*s*s) - (ros*ros-bros*bros)/4*s*s
	den := fros*fros*c*c + ((ros+bros)/2)*((ros+bros)/2)*s*s
	return (ros-bros)/(2*c) + (ros+bros)/(2*c)*(num/den)
}

// EQ94Defined is EQ94Published where it can be evaluated, and NaN within 1e-6
// of the 90° and 270° hole. There cos θ is not 0 but ~6e-17, and the two
// leading terms are ~1e17 each and cancel to noise: a number, but not the
// paper's. The limit the paper describes is ROSAtAngle's to assert, unconditionally.
func EQ94Defined(ros, fros, bros, thetaDeg float64) float64 {
	if math.Abs(math.Cos(thetaDeg*math.Pi/180)) < 1e-6 {
		return math.NaN()
	}
	return EQ94Published(ros, fros, bros, thetaDeg)
}

// Num is a numeric column, NaN when absent or null.
func Num(get Getter, col string) float64 {
	if v, ok := get(col); ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return math.NaN()
}
