# Migration ledger — cffdrs (R) → gofbp (Go)

What has been read, what has been ported, what has deliberately been left out,
and what is still owed. The upstream is
[cffdrs/cffdrs_r](https://github.com/cffdrs/cffdrs_r); the daily procedure that
keeps this file honest is [DAILY-CHECK.md](DAILY-CHECK.md). `/migration-check`
audits this file; `/migration-port` moves one row of it.

A row here is a claim about this repository, so it has to be checkable. "Ported"
means the Go function exists **and** a test asserts it; "Ported ✅" additionally
means a `TestCFFDRS*` asserts it against the fixture, which is the only status
that says the coefficients are *right* rather than merely self-consistent.

## Pins

| | value | checked |
|---|---|---|
| Upstream commit last read | `4d20a30` (2026-05-11) | 2026-09-06 |
| Upstream package version | 1.10.0 | 2026-09-06 |
| Oracle pins (`testdata/Dockerfile`) | cffdrs 1.9.2, R 4.6.1 | 2026-09-06 |
| Fixture sha256 | `12b9a009…89a7f` (`testdata/README.md`) | 2026-09-06 |

> **Open drift:** the oracle pins 1.9.2, upstream is 1.10.0. `4d20a30` changes
> foliar moisture content handling and adds `D0` to `fbp()`'s output. gofbp does
> not implement FMC, so nothing here is *wrong* today — but the FMC row below
> must be ported against 1.10.0's behaviour, not 1.9.2's, and the bump has to be
> a deliberate reviewed step (regenerate, read the diff, record the new digest).

## Status key

| | meaning |
|---|---|
| ✅ | ported, and a `TestCFFDRS*` asserts it against the fixture |
| 🟢 | ported, asserted only by identity/invariant tests — no oracle column exists |
| 🟡 | partially ported — read the note, it is a real gap, not a rounding difference |
| 🔴 | not ported, and in scope |
| ⚪ | deliberately out of scope — the reason is the row's whole content |

## R/ — file by file

| Upstream file | Concept | Status | gofbp |
|---|---|---|---|
| `rate_of_spread.r` | RSI, all 15 fuel types; M1/M2 PC blend; O1 curing | ✅ | `fbp.go` `RSI` |
| `buildup_effect.r` | BE | ✅ | `fbp.go` `BuildupEffect` |
| `Slopecalc.r` | WSE/WSV/RAZ slope back-solve, SF | ✅ | `slopewind.go` |
| `initial_spread_index.r` | ISI with FBP's high-wind wind function | ✅ | `fbp.go` `ISI` |
| `length_to_breadth.r` | LB | ✅ | `ellipse.go` `LengthToBreadth` |
| `back_rate_of_spread.r` | BROS via the back ISI ratio | ✅ | `ellipse.go` `BackISIRatio` |
| `flank_rate_of_spread.r` | FROS | ✅ | `ellipse.go` `FlankROS` |
| `CFBcalc.r` | CSI, RSO, CFB, FD | ✅ | `crown.go` |
| `rate_of_spread_at_theta.r` | ROS at an arbitrary bearing | 🟢 | `ellipse.go` `ROSAtAngle` — `fbp()` returns ellipse parameters, not a rate at a bearing, so there is no column to assert against. Pinned by exact identities at 0°/180° plus a shape assertion. |
| `direction.r` | signed angle between two bearings | 🟢 | `ellipse.go` `AngleBetweenDeg` — **verify the equivalence is real**, not assumed; upstream's helper and ours were written for different call sites. |
| `foliar_moisture_content.r`, `foliar_moisture_content_minimum.r` | FMC from lat/long/elev/date | 🔴 | — Caller must supply `Crown.FMC`. Port against 1.10.0, which reshaped this and added `D0`. |
| `surface_fuel_consumption.r` | SFC per fuel from FFMC/BUI | 🔴 | — Caller must supply `Crown.SFC`. Blocks a self-contained crown path. |
| `crown_base_height.r` | per-fuel CBH defaults | 🔴 | — Caller must supply `Crown.CBH`. |
| `crown_fuel_load.r` | per-fuel CFL defaults | 🔴 | — Caller must supply `Crown.CFL`. This table is what keeps D1/S1–S3/O1A/O1B at zero crown; without it that behaviour is the caller's to get right. |
| `C6calc.r` | C6 crown ROS (RSC), CFB-blended final ROS | 🔴 | — C6 is the one fuel whose ROS depends on CFB. gofbp's C6 is surface-only and **every oracle test excludes C6 by name.** |
| `total_fuel_consumption.r` | CFC, TFC | 🔴 | — needs SFC first. |
| `fire_intensity.r` | HFI | 🔴 | — needs TFC first. |
| `rate_of_spread_at_time.r`, `distance_at_time.r`, `length_to_breadth_at_time.r` | acceleration model | 🔴 | — the whole time-dependent branch. Nothing here is time-aware. |
| `fbp.r`, `fire_behaviour_prediction.r` | the umbrella driver | 🟡 | — gofbp exposes the pieces, not one `fbp()`-shaped call. Deliberate for now; revisit only once the 🔴 rows above close, since a driver that silently defaults FMC/SFC/CBH/CFL is exactly the local judgement this package refuses to make. |
| `buildup_index.r`, `drought_code.r`, `duff_moisture_code.r`, `fine_fuel_moisture_code.r`, `fire_weather_index.r`, `fwi.r` | the FWI System | ⚪ | Inputs to FBP, not part of it. gofbp takes FFMC/BUI/ISI as given. |
| `gfmc.r`, `grass_fuel_moisture*.r`, `hffmc.r`, `hourly_fine_fuel_moisture_code.r`, `sdmc.r` | hourly/grass/duff moisture codes | ⚪ | Same boundary as above. |
| `fire_season.r`, `overwinter_drought_code.r` | seasonal bookkeeping | ⚪ | Not FBP. |
| `fbpRaster.r`, `fwiRaster.r`, `gfmcRaster.R`, `hffmcRaster.r` | raster wrappers | ⚪ | Gridding is the caller's; the Go package stays dependency-free and scalar. |
| `lros.r`, `pros.r` | line/point ROS from observed arrival times | ⚪ | Inference from observations, not the FBP System's forward equations. |
| `cffdrs-package.R` | roxygen package docs | ⚪ | Not code. |

## Non-`R/` directories

| Upstream | Status | Note |
|---|---|---|
| `data/` | 🟡 | Fuel-type tables. gofbp transcribed the ST-X-3 tables by hand and checks them against the fixture. The CBH/CFL defaults living here are the 🔴 rows above. |
| `tests/` | 🟡 | Not mirrored. gofbp asserts against a generated ~18400-case sweep instead (`testdata/gen_cffdrs_reference.R`). Upstream's own test cases are still worth reading for edge cases the sweep does not reach. |
| `man/`, `inst/` | ⚪ | Docs and package metadata. |
| `NEWS.md` | — | **Read on every version bump.** It is the cheapest signal that a coefficient moved. |

## Concepts still missing, in dependency order

The 🔴 rows are not independent. Doing them out of order means writing code that
cannot be oracle-tested yet.

Each item names the upstream file its row is keyed on, because `TestLedger…` in
`ledger_test.go` joins this list to the table above on exactly that name — every
🔴 row has to appear here, and nothing may appear here that is not still owed.

1. **SFC** (`surface_fuel_consumption.r`) — unlocks TFC, then HFI, and makes
   `Crown` usable without the caller supplying `SFC` by hand.
2. **FMC** (`foliar_moisture_content.r`, against 1.10.0 and including `D0`) — the
   other `Crown` input a caller currently has to invent.
3. **CBH / CFL defaults** (`crown_base_height.r`, `crown_fuel_load.r`) — per-fuel
   tables; small, and the last thing standing between `Crown` and a
   fuel-code-only call.
4. **C6 RSC** (`C6calc.r`) — removes the exclusion that every oracle test
   currently carries. Do not attempt before CBH/CFL land; C6's ROS *is* the CFB
   blend.
5. **TFC, HFI** (`total_fuel_consumption.r`, `fire_intensity.r`) — mechanical
   once SFC exists.
6. **Acceleration model** (`rate_of_spread_at_time.r`, `distance_at_time.r`,
   `length_to_breadth_at_time.r`) — the largest remaining block, and the only one
   that changes the package's shape (it introduces time).
7. **An `fbp()`-shaped driver** (`fbp.r`) — last, or never. See the row above.

## Log

Newest first. One line per audit; a day with no change still gets a line, so a
gap in the dates is visible as a gap.

| Date | Upstream commit | What changed |
|---|---|---|
| 2026-09-06 | `4d20a30` | Ledger created. Full `R/` inventory taken against upstream 1.10.0; oracle-vs-upstream version drift recorded as an open item. |
