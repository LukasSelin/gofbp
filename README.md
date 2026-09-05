# gofbp

[![CI](https://github.com/LukasSelin/gofbp/actions/workflows/ci.yml/badge.svg)](https://github.com/LukasSelin/gofbp/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/LukasSelin/gofbp.svg)](https://pkg.go.dev/github.com/LukasSelin/gofbp)

The Canadian Forest Fire Behaviour Prediction (FBP) System's head-fire rate of
spread, in Go. Zero dependencies — the whole package imports `math` and nothing
else. Checked against `cffdrs`, the Canadian Forest Service's own R
implementation.

```
go get github.com/LukasSelin/gofbp
```

The module is `gofbp`; the package is `fbp`. So
`import "github.com/LukasSelin/gofbp"` binds the identifier `fbp`, and no alias
is needed.

## What this is

ST-X-3 as published: the equations and coefficient tables of the FBP System, with
no local adaptation, no defaults chosen for a particular country, and no
judgement about what the ground is made of. That constraint is the point — the
claim "this is the Canadian FBP System, unmodified" is only checkable if the
package making it contains nothing else.

What it produces is a **rate of spread in metres per minute**: a physical
quantity, not a danger score. A fire-danger index is a unitless number whose
range is tied to whatever scale published it; a spread rate has units that make
it checkable against observed fire behaviour. They answer different questions —
*how dangerous are conditions today* versus *if it starts here, how fast does it
move* — and multiplying one by the other destroys both. Do not fold this into an
index.

Everything local — which fuel type a stand maps to, what curing to assume, what
to do with wetland — is the caller's, and reaches the package as the fuel code
its functions take as an argument.

## Units and conventions

This is where the package is most likely to be misused, because none of these are
the units weather data usually arrives in.

| Quantity | Unit |
|---|---|
| Wind speed | **km/h** (ST-X-3's unit — convert from m/s yourself) |
| Rate of spread | metres per minute |
| Slope | **percent rise**, not degrees — 45° is 100%, not 45%. Use `SlopePercentFromDegrees` |
| FFMC, BUI, ISI | the FWI System's own scales |
| Azimuths | degrees clockwise from true north |

Every azimuth is the direction the thing **pushes towards**, never where it comes
from:

- A meteorological *wind from* bearing — which is what almost all weather data
  gives you — is `WindAzimuthDeg - 180`.
- A *downslope* aspect — which is what a terrain aspect raster normally stores —
  is `UpslopeAzimuthDeg - 180`.

Only the difference of the two reaches the net wind **speed**, so getting both
conventions wrong in the same direction still gives you the right speed and the
wrong **direction**. Do not rely on that cancellation.

## Usage

```go
s := fbp.SlopeWind{
	Code:              "C2",
	FFMC:              92,
	SlopePct:          fbp.SlopePercentFromDegrees(20), // percent rise, not degrees
	WindKmh:           15,                              // km/h, not m/s
	WindAzimuthDeg:    45,                              // the wind pushes towards NE
	UpslopeAzimuthDeg: 45,                              // the ground rises towards NE
	PC:                100,
}
const bui = 80

// Slope is not a multiplier. It is back-solved into an equivalent wind and
// vector-added to the real one.
wsv, raz := fbp.NetEffectiveWind(s)

// The head rate. slopePct is 0 on purpose: the slope is already inside wsv.
isi := fbp.ISI(s.FFMC, wsv)
head := fbp.ROS(s.Code, isi, bui, s.PC, s.CuringPct, 0)

// The ellipse, for "how fast is this coming at something due east of me".
back := fbp.ROS(s.Code, isi*fbp.BackISIRatio(wsv), bui, s.PC, s.CuringPct, 0)
lb := fbp.LengthToBreadth(s.Code, wsv)
flank := fbp.FlankROS(head, back, lb)
east := fbp.ROSAtAngle(head, flank, back, fbp.AngleBetweenDeg(raz, 90))
```

```
net effective wind: 30.7 km/h towards 45°
head:  44.96 m/min
flank: 5.81 m/min
back:  0.71 m/min
east:  4.65 m/min
```

That spread — 45 m/min at the head against 0.71 at the back — is why the head
rate alone is the wrong number for "how fast is this coming at *me*". See
`example_test.go`, which is this program and is run by the test suite.

## What is implemented

- `RSI` — initial spread rate, all 15 ST-X-3 fuel types, including the M1/M2
  percent-conifer blends and the O1 grass curing factor
- `BuildupEffect` (BE), `SlopeFactor` (SF), `SlopePercentFromDegrees`
- `ISI` — the FWI System's Initial Spread Index with FBP's high-wind wind
  function
- `EquivalentWind` and `NetEffectiveWind` — the slope back-solve (WSE, WSV, RAZ)
- `ROS` — head-fire rate of spread
- The fire ellipse: `LengthToBreadth`, `BackISIRatio`, `FlankROS`, `ROSAtAngle`,
  `AngleBetweenDeg`

## What is not implemented

**Crown fire.** 3689 of the reference fixture's 11500 cases carry `CFB != 0` and
are excluded from every assertion here, so above the crowning threshold this
package reports **surface spread only**. That is the largest remaining gap
between it and the published system, and you should know it before using this
where crowning is plausible.

Also absent: FMC, SFC, TFC and HFI, foliar moisture, the acceleration model, and
everything else `cffdrs::fbp()` returns that is not spread geometry.

## Slope: two paths, and why `ROS` is an upper bound

The FBP System does not treat slope as a multiplier. It routes slope through a
zero-wind spread rate, back-solves the **equivalent wind** that the slope alone
implies, and vector-adds that to the real wind. `NetEffectiveWind` does this, and
it is why wind blowing *downhill* can cancel a slope rather than being multiplied
by it.

`ROS` takes the older, simpler path — `RSI · BE · SF` — because the back-solve
needs FFMC and wind, and a caller with neither cannot run it. Since `SF` is a
multiplier it applies whole regardless of wind direction, so `ROS` on sloped
ground is an **upper bound, never an under-estimate**:

| wind vs. slope | median | p95 | worst |
|---|---|---|---|
| driving upslope | 1.14x | — | 6.54x |
| cross-slope | 1.80x | — | — |
| opposing slope | 4.18x | 84x | 99x |

Note the upslope row: even with the wind helping, `ROS` is not a safe stand-in on
steep dry ground.

**With all four inputs**, do what the example above does — take `WSV` from
`NetEffectiveWind`, feed `ISI(FFMC, WSV)` into `ROS`, and pass `slopePct = 0`.
The slope is already inside `WSV`; passing it again counts it twice.

At zero wind the two paths usually agree exactly. Two exceptions, neither exotic:
M1/M2 never satisfy it (eq. 42 blends ISF, not RSF), and neither does any fuel
where the RSI clamp or the equivalent-wind cap binds — which on dry steep ground
is most of them. Both leave `ROS` reading high.

## Correctness

Every coefficient here was transcribed by hand from ST-X-3, and a transcription
error looks exactly like correct code. The `TestCFFDRS*` tests assert this
package against [cffdrs](https://cran.r-project.org/package=cffdrs), maintained
by the Canadian Forest Service authors of the FBP System — the only oracle that
can say the tables are *right* rather than merely self-consistent.

RSI, BE and SF reproduce it exactly across all 15 fuel types. ISI, WSV and the
full slope path reproduce it to machine precision over every sloped case.

It has earned that twice:

- A missing grass-curing branch had fully-cured grass spreading **2.2x too
  fast**. The error came from the 1992 source itself, so a second careful
  transcription would have agreed precisely in being wrong.
- The FFMC moisture constant was the rounded 147.2 rather than the exact
  250·59.5/101 — a systematic ~1e-3 relative bias in ISI that every back-solved
  equivalent wind would have inherited in the same direction.

`ROSAtAngle`'s ellipse geometry has no cffdrs column to check against — `fbp()`
returns the ellipse's parameters, not a rate at an arbitrary bearing — so it is
pinned by exact identities at 0° and 180° plus a shape assertion instead.

**CI does not run the oracle.** The 3.6 MB fixture is generated rather than
committed, so the twelve fixture-backed tests skip on a fresh clone and CI green
means the identities, round-trips, invariants and NaN sweeps pass. A separate
monthly workflow regenerates the fixture and runs the oracle for real.

## Regenerating the fixture

```
./testdata/regen-cffdrs.sh
```

Needs Docker and nothing else; it pins R 4.6.1 and cffdrs 1.9.2 in a container.
See [testdata/README.md](testdata/README.md) for why the fixture is not committed
and how to read a regeneration.

## References

- Forestry Canada Fire Danger Group (1992). *Development and Structure of the
  Canadian Forest Fire Behavior Prediction System.* Information Report ST-X-3.
  Coefficient tables 6–7; equations throughout.
- Wotton, B.M., Alexander, M.E. & Taylor, S.W. (2009). *Updates and Revisions to
  the 1992 Canadian Forest Fire Behavior Prediction System.* Information Report
  GLC-X-10. — the grass curing revision in `CuringFactor`.
- Van Wagner, C.E. (1987). *Development and Structure of the Canadian Forest Fire
  Weather Index System.* Forestry Technical Report 35. — the fine-fuel moisture
  term (eq. 10) and ISI (eqs. 24–26).
- Wang, X., Wotton, B.M., Cantin, A.S., Parisien, M.-A., Anderson, K., Moore, B.
  & Flannigan, M.D. *cffdrs: Canadian Forest Fire Danger Rating System.* R
  package — the oracle these tests assert against.

## Contributing

Issues and pull requests welcome. `go test ./...` is the whole build. A change to
any coefficient or equation needs the oracle: regenerate the fixture, run
`go test . -run TestCFFDRS`, and say in the PR what the reference numbers did.

## Licence

MIT — see [LICENSE](LICENSE). The `cffdrs` R package used to generate the test
fixture is GPL-2 and is not redistributed here; see
[testdata/README.md](testdata/README.md).
