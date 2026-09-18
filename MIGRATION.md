# Migration ledger — cffdrs (R) → gofbp (Go)

What has been read, what has been ported, what has deliberately been left out,
and what is still owed. The upstream is
[cffdrs/cffdrs_r](https://github.com/cffdrs/cffdrs_r); the daily procedure that
keeps this file honest is [DAILY-CHECK.md](DAILY-CHECK.md). `/migration-check`
audits this file daily; `/migration-sweep` is the same audit's judgement half fanned
out, for when ten minutes is not enough; `/migration-port` moves one row of it.

A row here is a claim about this repository, so it has to be checkable. "Ported"
means the Go function exists **and** a test asserts it; "Ported ✅" additionally
means a `TestCFFDRS*` asserts it against the fixture, which is the only status
that says the coefficients are *right* rather than merely self-consistent.

## Pins

| | value | checked |
|---|---|---|
| Upstream commit last read | `4d20a30` (2026-05-11) | 2026-09-18 |
| Upstream package version | 1.10.0 | 2026-09-18 |
| Oracle pins (`testdata/Dockerfile`) | cffdrs 1.9.2, R 4.6.1 | 2026-09-18 |
| Fixture sha256 | `148a5a9e…24cf8f` (`testdata/README.md`) | 2026-09-18 |

> **Open drift — re-measured 2026-09-18, and it is not what this note used to
> say.** The oracle pins cffdrs 1.9.2 because **1.9.2 is CRAN's current release**.
> 1.10.0 exists only in the upstream git tree:
> `remotes::install_version('cffdrs', version = '1.10.0')` fails with *version
> '1.10.0' is invalid for package 'cffdrs'*, and the CRAN archive runs 1.7 … 1.9.0
> with 1.9.2 current and no 1.10.x at all. So this was never a bump nobody had got
> to — there is nothing released to bump to, and the oracle is already pinned to
> the newest cffdrs a user can install.
>
> That makes the FMC row's blocker a question about provenance rather than
> scheduling, and it is a human's to answer: **porting FMC "against 1.10.0" means
> taking the oracle's numbers from upstream's git HEAD instead of from a CRAN
> release**, which is a different claim about where this package's reference
> numbers come from. Until that is decided, an FMC port can only be asserted
> against 1.9.2. Checked while here: 1.9.2's `fbp(output = "ALL")` already returns
> `D0`, so re-read what `4d20a30` actually changed before trusting the sentence
> this note used to carry.

> **Open question (needs a decision, not a daily check):** what status
> `rate_of_spread_at_theta.r` should carry now. Both upstream rates at a bearing
> were measured against the pinned oracle on 2026-09-18 and both are defective —
> `fbp()`'s `TROS` mixes radians and degrees inside one `cos()`, and eq. 94
> collapses to `ROS/cosθ` with an uncaught pole at 90°. The row's own evidence
> therefore says 🟢, "no oracle column exists", which is where it started; what
> has changed is that the reason is now checked rather than assumed. It is left
> at 🟡 because moving it is a status call, and because the one thing still
> unchecked — whether the *published* eq. 94 reads the way cffdrs implements it —
> would change what the row is a finding *about*. Reading Wotton et al. (2009)
> eq. 94 is the next concrete step, and it needs the paper.
>
> **Open question (needs a decision, not a daily check):** the `direction.r` row
> below claims an equivalence that does not hold. The honest placement is ⚪,
> alongside the `lros.r`/`pros.r` rows whose helper it is, with `AngleBetweenDeg`
> recorded as gofbp's own helper rather than a port of anything. That is a move out
> of scope, so it is left for a human to make deliberately.

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
| `rate_of_spread.r` | RSI, all 17 fuel types; M1/M2 PC and M3/M4 PDF blends; O1 curing | ✅ | `fbp.go` `RSI` |
| `buildup_effect.r` | BE | ✅ | `fbp.go` `BuildupEffect` |
| `Slopecalc.r` | WSE/WSV/RAZ slope back-solve, SF | ✅ | `slopewind.go` |
| `initial_spread_index.r` | ISI with FBP's high-wind wind function | ✅ | `fbp.go` `ISI` |
| `length_to_breadth.r` | LB | ✅ | `ellipse.go` `LengthToBreadth` |
| `back_rate_of_spread.r` | BROS via the back ISI ratio | ✅ | `ellipse.go` `BackISIRatio` |
| `flank_rate_of_spread.r` | FROS | ✅ | `ellipse.go` `FlankROS` |
| `CFBcalc.r` | CSI, RSO, CFB, FD | ✅ | `crown.go` |
| `rate_of_spread_at_theta.r` | ROS at an arbitrary bearing | 🟡 | `ellipse.go` `ROSAtAngle` — **it became a finding.** The 2026-09-18 audit corrected this row from 🟢 (on the false claim that `fbp()` returns no rate at a bearing) to 🟡, and predicted "emit `TROS` from the generator and the row becomes ✅, or it becomes a finding". Measured against the pinned oracle on 2026-09-18 — cffdrs 1.9.2 / R 4.6.1, the container the fixture comes from — and **that correction was itself wrong in two ways.** (1) It conflated two different upstream quantities. `fbp()`'s `TROS` is `ROS·(1-E)/(1-E·cos(THETA-RAZ))`, `E = sqrt(1-1/LB²)` — built from ROS and LB only, never FROS or BROS. Eq. 94's `rate_of_spread_at_theta(ROS, FROS, BROS, THETA)` — the file this row is keyed on — is a different formula, is **unexported**, and is **never called by `fbp()`**. Only the second is the quantity `ROSAtAngle` implements. (2) **Neither is usable as a reference, because both are numerically defective.** `TROS`: `THETA` is converted to radians (`THETA <- THETA * pi/180`) and `RAZ` to degrees (`RAZ <- RAZ * 180/pi`), then subtracted inside one `cos()` — so `TROS` does not return `ROS` even at the head, giving 2.835 against `ROS` = 16.149 for `RAZ` = 180°. Present at the pinned upstream commit `4d20a30` too, and it propagates to `TCFB`, `TFI`, `TTI`, `TROSt`. Eq. 94: its two leading terms `(ROS-BROS)/(2·cosθ)` and `(ROS+BROS)/(2·cosθ)` share a denominator and collapse to `ROS/cosθ`, so there is a pole at θ = 90° that the `ifelse(c1 == 0, …)` guard never catches (`cos(π/2)` is 6.12e-17, not 0) — it returns -2.34e17 at 90°, negative rates from 30° to 89.9°, and `ROS` at *both* 0° and 180°, so `BROS` is never recovered. `ROSAtAngle` on the same case returns `ROS` at 0° and 0.9674577 at 180°, which is `BROS` to seven figures. **Not checked: whether the published eq. 94 in Wotton et al. (2009) reads the same way** — that needs the paper, and it is the difference between a cffdrs transcription error and one in ST-X-3's successor. Consequence: ✅ is not one generator line away and may not be reachable at all. Emitting `TROS` would assert gofbp against an oracle bug, and under this repo's rules the mismatch would have read as gofbp's fault. Also note `theta` is fixed at `0` in all 23,532 cases (`base_row`), so any θ-dependent column needs the sweep widened — which moves the digest and the case count in four places. **Left 🟡 rather than moved: on this evidence the honest status is 🟢** (ported, no oracle column exists — the original status, for a reason that is finally true), but that is a status call reserved for a human. See the open question above. |
| `direction.r` | `.direction(bearingT1T2, bearingT1T3, ThetaAdeg)` — rotates a bearing by an offset, with quadrant handling; signed, roughly [-180, 180] | 🟢 | `ellipse.go` `AngleBetweenDeg` — **the equivalence was checked on 2026-09-06 and it is not real.** Upstream takes three arguments and rotates one bearing by an offset; ours takes two and returns their unsigned separation in [0, 180]. `.direction` is a helper of `pros()`/`lros()`, which are ⚪ below, so it has no caller on the FBP forward path at all. `AngleBetweenDeg` is a gofbp convenience with no upstream counterpart, asserted by `TestAngleBetweenDeg`. Reclassifying this row is a scope call — see the open question above. |
| `foliar_moisture_content.r`, `foliar_moisture_content_minimum.r` | FMC from lat/long/elev/date | 🔴 | — Caller must supply `Crown.FMC`. The `fmc` oracle column **already exists**, carried as a Go-side input exactly as `sfc` was before its port, and the LAT/Dj values were chosen to reach all three date branches (eq. 6 quadratic, eq. 7 linear, eq. 8 plateau). What the sweep does *not* reach: `ELV`, `LONG` and `D0` are fixed at 0, 15 and 0 in every one of the 23,532 cases, so the elevation branch (`LATN = 43 + 33.7·exp(-0.0351·(150-LONG))`, `D0 = 142.1·LAT/LATN + 0.0172·ELV`) and the caller-supplied-`D0` branch have no coverage at all. A port that adds no sweep would assert half the function and look complete. See the drift note above for which cffdrs to assert against. |
| `surface_fuel_consumption.r` | SFC per fuel from FFMC/BUI | ✅ | `consumption.go` `SurfaceFuelConsumption` — all eleven equations, asserted over all 23,532 cases. `Crown.SFC` is still the caller's field: this gives them something to fill it with, it does not fill it for them. |
| `crown_base_height.r` | per-fuel CBH defaults | 🔴 | — Caller must supply `Crown.CBH`. |
| `crown_fuel_load.r` | per-fuel CFL defaults | 🔴 | — Caller must supply `Crown.CFL`. This table is what keeps D1/S1–S3/O1A/O1B at zero crown; without it that behaviour is the caller's to get right. |
| `C6calc.r` | C6 crown ROS (RSC), CFB-blended final ROS | 🔴 | — C6 is the one fuel whose ROS depends on CFB. gofbp's C6 is surface-only and **every oracle test excludes C6 by name.** |
| `total_fuel_consumption.r` | CFC, TFC | 🔴 | — its blocker cleared on 2026-09-18: SFC is now ported. Still needs CFL for the crown half (CFC = CFB·CFL). |
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
| `tests/` | 🟡 | Not mirrored. gofbp asserts against a generated 23,532-case sweep instead (`testdata/gen_cffdrs_reference.R`). Upstream's own test cases are still worth reading for edge cases the sweep does not reach. |
| `man/`, `inst/` | ⚪ | Docs and package metadata. |
| `NEWS.md` | — | **Read on every version bump.** It is the cheapest signal that a coefficient moved. |

## Concepts still missing, in dependency order

The 🔴 rows are not independent. Doing them out of order means writing code that
cannot be oracle-tested yet.

Each item names the upstream file its row is keyed on, because `TestLedger…` in
`ledger_test.go` joins this list to the table above on exactly that name — every
🔴 row has to appear here, and nothing may appear here that is not still owed.

1. **FMC** (`foliar_moisture_content.r`) — the other `Crown` input a caller
   currently has to invent. Two things to settle first, neither of them
   transcription. **Which cffdrs to assert against:** 1.10.0 is not a CRAN
   release and the oracle already pins the newest one that is, so "port against
   1.10.0" means sourcing reference numbers from git HEAD — see the drift note,
   and ask before deciding it. **The sweep:** `ELV`/`LONG`/`D0` are constant
   across the fixture, so the elevation branch is unasserted and needs generator
   work. The `fmc` column itself is already there.
2. **CBH / CFL defaults** (`crown_base_height.r`, `crown_fuel_load.r`) — per-fuel
   tables; small, and the last thing standing between `Crown` and a
   fuel-code-only call. **How these are exposed is a design decision, not a
   porting one** — a lookup the caller opts into is a different package from a
   silent default inside `Crown`. Ask before choosing.
3. **C6 RSC** (`C6calc.r`) — removes the exclusion that every oracle test
   currently carries. Do not attempt before CBH/CFL land; C6's ROS *is* the CFB
   blend.
4. **TFC, HFI** (`total_fuel_consumption.r`, `fire_intensity.r`) — SFC landed on
   2026-09-18, so TFC's surface half (eq. 60) is unblocked today. Its crown half
   is CFC = CFB·CFL, so a *complete* TFC still waits on item 2, and HFI (eq. 66)
   waits on TFC. Left here rather than promoted: the order is a dependency order
   and nothing above it depends on it, so moving it up is a scheduling call for
   a human, not a correctness one.
5. **Acceleration model** (`rate_of_spread_at_time.r`, `distance_at_time.r`,
   `length_to_breadth_at_time.r`) — the largest remaining block, and the only one
   that changes the package's shape (it introduces time).
6. **An `fbp()`-shaped driver** (`fbp.r`) — last, or never. See the row above.

## Log

Newest first. One line per audit; a day with no change still gets a line, so a
gap in the dates is visible as a gap.

| Date | Upstream commit | What changed |
|---|---|---|
| 2026-09-18 | `4d20a30` | Upstream unchanged (`tools/upstream-drift` exit 0: HEAD still `4d20a30`, so there are no `NEWS.md` versions between the pin and now); no port. Gate green (`precheck` exit 0: READY, `go test ./...` green, 0 of 14 `TestCFFDRS*` skipping, fixture `148a5a9e…24cf8f`). **Targeted sweep on the two rows the previous audit left open — and the first overturns that audit's own correction.** `rate_of_spread_at_theta.r` was measured against the pinned container this time rather than read, and the row had conflated two different upstream quantities: `fbp()`'s `TROS` (`ROS·(1-E)/(1-E·cos(THETA-RAZ))`, built from ROS and LB only) is **not** eq. 94's `rate_of_spread_at_theta(ROS, FROS, BROS, THETA)`, which is unexported and never called by `fbp()`. **Both are numerically defective**, so the predicted "emit `TROS` and the row becomes ✅" was never on the table. `TROS` subtracts `RAZ` in degrees from `THETA` in radians inside one `cos()` (1.9.2 lines 134 / 232 / 249, and the same three lines at `4d20a30`), so it misses even the head rate — 2.835 where `ROS` is 16.149 at `RAZ` = 180° — and carries that into `TCFB`, `TFI`, `TTI` and `TROSt`. Eq. 94's two leading terms share a denominator and collapse to `ROS/cosθ`: an uncaught pole at 90° (-2.34e17, because the `ifelse(c1 == 0, …)` guard never fires — `cos(π/2)` is 6.12e-17), negative rates from 30° to 89.9°, and `ROS` returned at *both* 0° and 180° so `BROS` never comes back. `ROSAtAngle` on the same case gives `ROS` at 0° and `BROS` to seven figures at 180°. Row left 🟡 with the evidence written into it; on that evidence the honest status is 🟢, which is now an open question above — as is reading the *published* eq. 94, which was **not** checked and decides whether this is cffdrs' transcription error or the paper's. The disproved "no rate at a bearing" claim was still live in four places the previous fix missed (`ellipse.go` header and `ROSAtAngle` doc, `ellipse_test.go`, `README.md`) and in `/migration-port`, where it was the **worked example of oracle case (c)** — all five corrected, and the command now carries the lesson ("no column exists" is a claim about upstream, so check upstream for it) rather than the claim. Corrected in passing: `ROSAtAngle`'s doc credited "monotone decrease from head to back", which its own preceding paragraph and `TestROSAtAngleIsUnimodalWithTheHeadAsMaximum` both contradict. `direction.r` re-confirmed against the oracle and unchanged: `.direction` has three formals, is unexported, and is called by exactly `lros` and `pros` (both ⚪) — not by `fbp` or `fire_behaviour_prediction`. No generator change, no regeneration, digest untouched. |
| 2026-09-18 | `4d20a30` | Upstream unchanged (`tools/upstream-drift`: HEAD still `4d20a30`); no port. **First check in this worktree actually backed by the oracle.** The fixture was generated from the pinned container and reproduced `148a5a9e…24cf8f` byte for byte, so the recorded digest is now confirmed by regeneration rather than carried forward, and all 14 `TestCFFDRS*` assert instead of skipping (`precheck`: READY, 0 of 14 skipping). Three claims in this file were wrong and are corrected above. (a) **1.10.0 is not a CRAN release** — `remotes::install_version` rejects it and the archive stops at the current 1.9.2, which is exactly what the oracle pins, so the standing "open drift" was never a bump somebody had not got to; it is a question about whether reference numbers may come from git HEAD, and it is now written down as one. (b) **`rate_of_spread_at_theta.r` does have an oracle column** — `fbp(output = "ALL")` returns `TROS`; the row was 🟢 on the claim that no column exists, and is now 🟡 with the column named. (c) 1.9.2 already returns `D0`. Also fixed `testdata/regen-cffdrs.sh`: under Git Bash it handed `docker build` an MSYS-form context path (`/c/...`) with `MSYS_NO_PATHCONV` already exported for the `docker run` mount, so **every first-time image build failed on Windows** while re-running an already-built image worked — which is why it stayed hidden. Found by trying to build a 1.10.0 oracle; with the path fixed, that build ran far enough to establish (a). |
| 2026-09-18 | `4d20a30` | Upstream unchanged (`tools/upstream-drift`: HEAD still `4d20a30`). **Ported SFC** — `surface_fuel_consumption.r` → `consumption.go`, all eleven equations, 🔴 → ✅. Oracle case (a): `sfc` was already a fixture column carried as an input, so the generator did not change and the fixture is byte-identical — `tools/fixture-diff` over the pre-port copy reports *No column shared by both fixtures moved*, digest still `148a5a9e…24cf8f`. All 23,532 cases assert, worst relative error 4.01e-16. Two places cffdrs departs from FCFDG 1992 are now written down and both are oracle-confirmed: C1 uses GLC-X-10's eqs. 9a/9b, not eq. 9 (documented); C7's eq. 13 term is clamped at FFMC 70 (**not** documented upstream). The `SFC <= 0 → 1e-6` floor is reproduced from the reference implementations but the sweep never reaches it — `TestCFFDRSSurfaceFuelConsumptionFloorIsUnreached` says so out loud. No default for SFC or GFL was added; `Crown.SFC` is still the caller's field. |
| 2026-09-06 | `4d20a30` | No upstream change; no port. Turned the mechanical half of this procedure into code: `ledger_test.go` for the step-4 sweep, and `tools/precheck`, `tools/upstream-drift`, `tools/fixture-diff` for the gate, the upstream join and the reference-number diff. The digest re-baseline below was re-derived independently here and reached the same `148a5a9e…`; `tools/fixture-diff` against a rebuilt pre-M3/M4 fixture then showed no reference number moved, across the 18804 cases the two sweeps share. Sweep is 23,532 cases exactly. |
| 2026-09-06 | `4d20a30` | Upstream unchanged (HEAD still `4d20a30`); no port. Audit found three stale claims and fixed them: the fixture digest still named the pre-M3/M4 sweep (regenerated from the pinned container, byte-identical, `148a5a9e…24cf8f`), the `rate_of_spread.r` row still said 15 fuel types, and the sweep is ~23500 cases, not ~18400. Checked the `direction.r` equivalence the row asked about: it does not hold. |
| 2026-09-06 | `4d20a30` | Ledger created. Full `R/` inventory taken against upstream 1.10.0; oracle-vs-upstream version drift recorded as an open item. |
