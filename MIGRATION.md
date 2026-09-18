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
| Upstream commit last read | `4d20a30` (2026-05-11) | 2026-09-18 |
| Upstream package version | 1.10.0 | 2026-09-18 |
| Oracle pins (`testdata/Dockerfile`) | cffdrs 1.9.2, R 4.6.1 | 2026-09-18 |
| Fixture sha256 | `e0e51ff1…7db8cc` (`testdata/README.md`) | 2026-09-18 |

> **Open drift — re-measured 2026-09-18, and it is not what this note used to
> say.** The oracle pins cffdrs 1.9.2 because **1.9.2 is CRAN's current release**.
> 1.10.0 exists only in the upstream git tree:
> `remotes::install_version('cffdrs', version = '1.10.0')` fails with *version
> '1.10.0' is invalid for package 'cffdrs'*, and the CRAN archive runs 1.7 … 1.9.0
> with 1.9.2 current and no 1.10.x at all. So this was never a bump nobody had got
> to — there is nothing released to bump to, and the oracle is already pinned to
> the newest cffdrs a user can install.
>
> That makes the question one of provenance rather than scheduling, and it is
> still a human's to answer: **taking reference numbers "from 1.10.0" means taking
> them from upstream's git HEAD instead of from a CRAN release**, which is a
> different claim about where this package's numbers come from.
>
> **It did not block the FMC port, and the reason is measured rather than
> assumed.** Both versions' source was read side by side on 2026-09-18 — 1.9.2's
> out of the pinned container, HEAD's from `4d20a30` — and they are the same
> arithmetic. HEAD splits `foliar_moisture_content` into two files, adds an
> `is.na` guard and an `FMCo` override, and warns outside North America; none of
> that moves a number. The one difference that *can*: 1.9.2's `round(D0, 0)` sits
> after the ifelse and so rounds a **caller-supplied** `D0` as well, where HEAD
> rounds only a computed one. A supplied `D0` of 150.7 is 151 to 1.9.2 and 150.7
> to HEAD. Integer dates — the only kind the sweep or a sane caller passes — are
> unaffected, and `foliar.go` reproduces 1.9.2 and says so. So the decision now has
> a named consequence attached to it instead of an open-ended one.
>
> Checked while here: 1.9.2's `fbp(output = "ALL")` does return `D0`, but it is
> the **echoed input**, not the computed date of minimum — verified by sending
> `D0 = 150` and `D0 = 0` and reading the column back. So there is no oracle
> column for eqs. 1-4 on their own; they are asserted through FMC.

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
| `rate_of_spread_at_theta.r` | ROS at an arbitrary bearing | 🟡 | `ellipse.go` `ROSAtAngle` — **this row was 🟢 on the claim that `fbp()` returns ellipse parameters and not a rate at a bearing, "so there is no column to assert against". Checked on 2026-09-18: that is false.** `fbp(output = "ALL")` returns `TROS` (and `TROSt`), the rate at the input bearing `theta`, and upstream's `rate_of_spread_at_theta(ROS, FROS, BROS, THETA)` solves for the same ellipse radius this does — in a different closed form, and with `THETA` in radians where `ROSAtAngle` takes degrees. The sweep already passes `theta = 0` and simply does not emit the column. This is therefore an *unported* oracle column, not an absent one: emit `TROS` from the generator and the row becomes ✅, or it becomes a finding. Until then it is pinned only by exact identities at 0°/180° plus a shape assertion, which is what 🟡 means here. |
| `direction.r` | `.direction(bearingT1T2, bearingT1T3, ThetaAdeg)` — rotates a bearing by an offset, with quadrant handling; signed, roughly [-180, 180] | 🟢 | `ellipse.go` `AngleBetweenDeg` — **the equivalence was checked on 2026-09-06 and it is not real.** Upstream takes three arguments and rotates one bearing by an offset; ours takes two and returns their unsigned separation in [0, 180]. `.direction` is a helper of `pros()`/`lros()`, which are ⚪ below, so it has no caller on the FBP forward path at all. `AngleBetweenDeg` is a gofbp convenience with no upstream counterpart, asserted by `TestAngleBetweenDeg`. Reclassifying this row is a scope call — see the open question above. |
| `foliar_moisture_content.r`, `foliar_moisture_content_minimum.r` | FMC from lat/long/elev/date | ✅ | `foliar.go` `FoliarMoistureContent`, `DateOfMinimumFoliarMoisture` — eqs. 1-8, asserted over 17,364 cases at zero relative error. The sweep was widened to reach the two branches it never had: `ELV`, `LONG` and `D0` were constants, so eqs. 3/4 and the caller-supplied-`D0` path had no coverage and eqs. 1/2's longitude exponential was pinned at a single point. `Crown.FMC` is still the caller's field. The 6,384 D1/S1/S2/S3/O1A/O1B rows are excluded by name: `fbp()` zeroes FMC for the crownless fuels *after* computing it, so their column describes the driver, not eqs. 1-8. |
| `surface_fuel_consumption.r` | SFC per fuel from FFMC/BUI | ✅ | `consumption.go` `SurfaceFuelConsumption` — all eleven equations, asserted over all 23,748 cases. `Crown.SFC` is still the caller's field: this gives them something to fill it with, it does not fill it for them. |
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
| `tests/` | 🟡 | Not mirrored. gofbp asserts against a generated 23,748-case sweep instead (`testdata/gen_cffdrs_reference.R`). Upstream's own test cases are still worth reading for edge cases the sweep does not reach. |
| `man/`, `inst/` | ⚪ | Docs and package metadata. |
| `NEWS.md` | — | **Read on every version bump.** It is the cheapest signal that a coefficient moved. |

## Concepts still missing, in dependency order

The 🔴 rows are not independent. Doing them out of order means writing code that
cannot be oracle-tested yet.

Each item names the upstream file its row is keyed on, because `TestLedger…` in
`ledger_test.go` joins this list to the table above on exactly that name — every
🔴 row has to appear here, and nothing may appear here that is not still owed.

1. **CBH / CFL defaults** (`crown_base_height.r`, `crown_fuel_load.r`) — per-fuel
   tables; small, and the last thing standing between `Crown` and a
   fuel-code-only call. **How these are exposed is a design decision, not a
   porting one** — a lookup the caller opts into is a different package from a
   silent default inside `Crown`. Ask before choosing.
2. **C6 RSC** (`C6calc.r`) — removes the exclusion that every oracle test
   currently carries. Do not attempt before CBH/CFL land; C6's ROS *is* the CFB
   blend.
3. **TFC, HFI** (`total_fuel_consumption.r`, `fire_intensity.r`) — SFC landed on
   2026-09-18, so TFC's surface half (eq. 60) is unblocked today. Its crown half
   is CFC = CFB·CFL, so a *complete* TFC still waits on item 1, and HFI (eq. 66)
   waits on TFC. Left here rather than promoted: the order is a dependency order
   and nothing above it depends on it, so moving it up is a scheduling call for
   a human, not a correctness one.
4. **Acceleration model** (`rate_of_spread_at_time.r`, `distance_at_time.r`,
   `length_to_breadth_at_time.r`) — the largest remaining block, and the only one
   that changes the package's shape (it introduces time).
5. **An `fbp()`-shaped driver** (`fbp.r`) — last, or never. See the row above.

## Log

Newest first. One line per audit; a day with no change still gets a line, so a
gap in the dates is visible as a gap.

| Date | Upstream commit | What changed |
|---|---|---|
| 2026-09-18 | `4d20a30` | Upstream unchanged; no drift check run (this was a `/migration-port` run, not an audit). **Ported FMC** — `foliar_moisture_content.r` and `foliar_moisture_content_minimum.r` → `foliar.go`, eqs. 1-8, 🔴 → ✅. Oracle case **(b)**: the `fmc` column existed but `LONG`, `ELV` and `D0` were constants at 15, 0 and 0, so the fixture reached exactly one of the model's three paths — eqs. 3/4 and the caller-supplied-`D0` branch had no coverage and eqs. 1/2's longitude exponential was pinned at one point. The generator grew a 216-row FMC block (18 sites × 12 dates) and those three input columns; the sweep is **23,532 → 23,748** cases and the digest **`148a5a9e…24cf8f` → `e0e51ff1…7db8cc`**. **`tools/fixture-diff` over the pre-port copy: *No column shared by both fixtures moved*, exit 0** — 23,388 shared cases, 0 only-in-old, 216 only-in-new, and every output column (`fmc` included) unchanged. 17,364 cases assert FMC at **worst relative error 0**; 6,384 are excluded by name because `fbp()` zeroes FMC for D1/S1/S2/S3/O1A/O1B *after* computing it, which is the driver's decision and not eqs. 1-8's. Three places the R departs from FCFDG 1992, all now written down and two of them newly oracle-backed: cffdrs branches on `ELV <= 0` where the paper branches on "elevation known", so a measured 0 m gets the missing-data equations; cffdrs **rounds D0** (the paper does not) with R's half-to-**even** rule, which `math.Round` would get wrong — sites at `D0 = 150.5` and `151.5` now separate the two rules and the oracle sides with half-to-even; and `fbp()` folds the sign of `LONG` before calling, which one deliberately negative-longitude site pins. **The 1.10.0 provenance question did not block this and is still open** — the two versions' FMC source was read side by side and is the same arithmetic, with one named exception now recorded in the drift note above. No default for FMC was added; `Crown.FMC` is still the caller's field. |
| 2026-09-18 | `4d20a30` | Upstream unchanged (`tools/upstream-drift`: HEAD still `4d20a30`); no port. **First check in this worktree actually backed by the oracle.** The fixture was generated from the pinned container and reproduced `148a5a9e…24cf8f` byte for byte, so the recorded digest is now confirmed by regeneration rather than carried forward, and all 14 `TestCFFDRS*` assert instead of skipping (`precheck`: READY, 0 of 14 skipping). Three claims in this file were wrong and are corrected above. (a) **1.10.0 is not a CRAN release** — `remotes::install_version` rejects it and the archive stops at the current 1.9.2, which is exactly what the oracle pins, so the standing "open drift" was never a bump somebody had not got to; it is a question about whether reference numbers may come from git HEAD, and it is now written down as one. (b) **`rate_of_spread_at_theta.r` does have an oracle column** — `fbp(output = "ALL")` returns `TROS`; the row was 🟢 on the claim that no column exists, and is now 🟡 with the column named. (c) 1.9.2 already returns `D0`. Also fixed `testdata/regen-cffdrs.sh`: under Git Bash it handed `docker build` an MSYS-form context path (`/c/...`) with `MSYS_NO_PATHCONV` already exported for the `docker run` mount, so **every first-time image build failed on Windows** while re-running an already-built image worked — which is why it stayed hidden. Found by trying to build a 1.10.0 oracle; with the path fixed, that build ran far enough to establish (a). |
| 2026-09-18 | `4d20a30` | Upstream unchanged (`tools/upstream-drift`: HEAD still `4d20a30`). **Ported SFC** — `surface_fuel_consumption.r` → `consumption.go`, all eleven equations, 🔴 → ✅. Oracle case (a): `sfc` was already a fixture column carried as an input, so the generator did not change and the fixture is byte-identical — `tools/fixture-diff` over the pre-port copy reports *No column shared by both fixtures moved*, digest still `148a5a9e…24cf8f`. All 23,532 cases assert, worst relative error 4.01e-16. Two places cffdrs departs from FCFDG 1992 are now written down and both are oracle-confirmed: C1 uses GLC-X-10's eqs. 9a/9b, not eq. 9 (documented); C7's eq. 13 term is clamped at FFMC 70 (**not** documented upstream). The `SFC <= 0 → 1e-6` floor is reproduced from the reference implementations but the sweep never reaches it — `TestCFFDRSSurfaceFuelConsumptionFloorIsUnreached` says so out loud. No default for SFC or GFL was added; `Crown.SFC` is still the caller's field. |
| 2026-09-06 | `4d20a30` | No upstream change; no port. Turned the mechanical half of this procedure into code: `ledger_test.go` for the step-4 sweep, and `tools/precheck`, `tools/upstream-drift`, `tools/fixture-diff` for the gate, the upstream join and the reference-number diff. The digest re-baseline below was re-derived independently here and reached the same `148a5a9e…`; `tools/fixture-diff` against a rebuilt pre-M3/M4 fixture then showed no reference number moved, across the 18804 cases the two sweeps share. Sweep is 23,532 cases exactly. |
| 2026-09-06 | `4d20a30` | Upstream unchanged (HEAD still `4d20a30`); no port. Audit found three stale claims and fixed them: the fixture digest still named the pre-M3/M4 sweep (regenerated from the pinned container, byte-identical, `148a5a9e…24cf8f`), the `rate_of_spread.r` row still said 15 fuel types, and the sweep is ~23500 cases, not ~18400. Checked the `direction.r` equivalence the row asked about: it does not hold. |
| 2026-09-06 | `4d20a30` | Ledger created. Full `R/` inventory taken against upstream 1.10.0; oracle-vs-upstream version drift recorded as an open item. |
