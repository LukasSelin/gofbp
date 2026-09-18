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
> **Open questions raised by the 2026-09-18 orchestrated sweep (`/migration-sweep`),
> all status or scope calls:**
>
> - **Reorder the dependency order?** Only two of its edges are real (see the note
>   above that list). Items 3 and 4 can both be oracle-tested today, and item 3
>   against the fixture already on disk. The list is unchanged pending this.
> - **Split `inst/` out of the `man/`, `inst/` ⚪ row?** `inst/extdata` ships
>   upstream's own FBP reference output, which is not "docs and package metadata".
>   Related: does upstream-published output count as a reference source here at
>   all, given it is not generated from the pinned container? That is the same
>   provenance question the 1.10.0 note parks above.
> - **`data/` 🟡 → ⚪?** Once the two wrong claims are removed, there is nothing in
>   it for gofbp to port.
> - **Narrow `TestCFFDRSCrownFractionBurned`'s C6 exclusion?** Its second pass
>   matches all 768 C6 crown rows exactly, so the exclusion currently hides
>   agreement. Narrowing an exclusion changes what a test asserts, so it is
>   reported, not done.
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
| `rate_of_spread_at_theta.r` | ROS at an arbitrary bearing | 🟡 | `ellipse.go` `ROSAtAngle` — **it became a finding.** The 2026-09-18 audit corrected this row from 🟢 (on the false claim that `fbp()` returns no rate at a bearing) to 🟡, and predicted "emit `TROS` from the generator and the row becomes ✅, or it becomes a finding". Measured against the pinned oracle on 2026-09-18 — cffdrs 1.9.2 / R 4.6.1, the container the fixture comes from — and **that correction was itself wrong in two ways.** (1) It conflated two different upstream quantities. `fbp()`'s `TROS` is `ROS·(1-E)/(1-E·cos(THETA-RAZ))`, `E = sqrt(1-1/LB²)` — built from ROS and LB only, never FROS or BROS. Eq. 94's `rate_of_spread_at_theta(ROS, FROS, BROS, THETA)` — the file this row is keyed on — is a different formula, is **unexported**, and is **never called by `fbp()`**. Only the second is the quantity `ROSAtAngle` implements. (2) **Neither is usable as a reference, because both are numerically defective.** `TROS`: `THETA` is converted to radians (`THETA <- THETA * pi/180`) and `RAZ` to degrees (`RAZ <- RAZ * 180/pi`), then subtracted inside one `cos()` — so `TROS` does not return `ROS` even at the head. The case, named so the next reader does not have to search for it: **C2, FFMC 90, BUI 60, WS 20, GS 0, WD 0 — at `THETA = RAZ = 180`, `TROS` = 2.835 against `ROS` = 16.149.** (Those are oracle outputs for that input, not fixture rows; the fixture emits no `theta` or `tros` column.) Present at the pinned upstream commit `4d20a30` too, and it propagates to `TCFB`, `TFI`, `TTI`, `TROSt` and `TTFC`. Eq. 94: its two leading terms `(ROS-BROS)/(2·cosθ)` and `(ROS+BROS)/(2·cosθ)` share a denominator and collapse to `ROS/cosθ`, so there is a pole at θ = 90° that the `ifelse(c1 == 0, …)` guard never catches (`cos(π/2)` is 6.12e-17, not 0) — it returns -2.34e17 at 90°, negative rates from 30° to 89.9°, and `ROS` at *both* 0° and 180°, so `BROS` is never recovered. `ROSAtAngle` on the same case returns `ROS` at 0° and 0.9674577 at 180°, which is `BROS` to seven figures. **Not checked: whether the published eq. 94 in Wotton et al. (2009) reads the same way** — that needs the paper, and it is the difference between a cffdrs transcription error and one in ST-X-3's successor. Consequence: ✅ is not one generator line away and may not be reachable at all. Emitting `TROS` would assert gofbp against an oracle bug, and under this repo's rules the mismatch would have read as gofbp's fault. Also note `theta` is fixed at `0` in all 23,532 cases (`base_row`), so any θ-dependent column needs the sweep widened — which moves the digest and the case count in four places. **Left 🟡 rather than moved: on this evidence the honest status is 🟢** (ported, no oracle column exists — the original status, for a reason that is finally true), but that is a status call reserved for a human. See the open question above. |
| `direction.r` | `.direction(bearingT1T2, bearingT1T3, ThetaAdeg)` — rotates a bearing by an offset, with quadrant handling; signed, roughly [-180, 180] | 🟢 | `ellipse.go` `AngleBetweenDeg` — **the equivalence was checked on 2026-09-06 and it is not real.** Upstream takes three arguments and rotates one bearing by an offset; ours takes two and returns their unsigned separation in [0, 180]. `.direction` is a helper of `pros()`/`lros()`, which are ⚪ below, so it has no caller on the FBP forward path at all. `AngleBetweenDeg` is a gofbp convenience with no upstream counterpart, asserted by `TestAngleBetweenDeg`. **Re-checked by query on 2026-09-18, and the 🟢 legend's "no oracle column exists" is false as stated for this row:** `pros()`/`lros()` are both *exported* and return `data.frame(Ros, Direction)`, where `Direction` is `.direction`'s output verbatim. It is simply not in `fbp()`'s 43 columns — the only direction-like one is `RAZ`, the head azimuth — so it is unreachable from this repo's fixture. It is also **not an oracle for `AngleBetweenDeg`**: `.direction(10,40,15)` and `.direction(40,10,15)` both return 25 where `AngleBetweenDeg(10,40)` is 30. 🟢 stands on the narrow, now-tested ground that nothing upstream computes what `AngleBetweenDeg` computes. If `pros`/`lros` are ever brought into scope, `Direction` needs the `TROS` treatment first: its wrap tail adds ±10 rather than ±360 (`.direction(170,160,-30)` → 190, where the correct wrap is -160) and equal bearings fall through every branch to `NA`. Reclassifying this row is a scope call — see the open question above. |
| `foliar_moisture_content.r`, `foliar_moisture_content_minimum.r` | FMC from lat/long/elev/date | 🔴 | — Caller must supply `Crown.FMC`. The `fmc` oracle column **already exists**, carried as a Go-side input exactly as `sfc` was before its port, and the LAT/Dj values were chosen to reach all three date branches (eq. 6 quadratic, eq. 7 linear, eq. 8 plateau). What the sweep does *not* reach: `ELV`, `LONG` and `D0` are fixed at 0, 15 and 0 in every one of the 23,532 cases, so the elevation branch (`LATN = 43 + 33.7·exp(-0.0351·(150-LONG))`, `D0 = 142.1·LAT/LATN + 0.0172·ELV`) and the caller-supplied-`D0` branch have no coverage at all. A port that adds no sweep would assert half the function and look complete. See the drift note above for which cffdrs to assert against. **Queried 2026-09-18, not read:** the second file of this key has no counterpart at the pin — 1.9.2 exposes a single `foliar_moisture_content(LAT, LONG, ELV, DJ, D0)` and `foliar_moisture_content_minimum` does not exist in the namespace at all, so half this row is unportable against 1.9.2 by construction rather than merely unswept. That *is* the provenance question, not a consequence of it. Both branch gates are `<= 0`, so the fixture's `ELV = 0` and `D0 = 0` select the non-elevation and computed-`D0` arms on every row — the two formulas quoted above are exactly the arms never taken. Two details a port must not miss: `D0` is `round(D0, 0)` before `ND`, and `LONG` is constant at 15, so the non-elevation `LATN` coefficient is pinned at a single longitude. |
| `surface_fuel_consumption.r` | SFC per fuel from FFMC/BUI | ✅ | `consumption.go` `SurfaceFuelConsumption` — all eleven equations, asserted over all 23,532 cases. `Crown.SFC` is still the caller's field: this gives them something to fill it with, it does not fill it for them. |
| `crown_base_height.r` | per-fuel CBH defaults | 🔴 | — Caller must supply `Crown.CBH`. **There is no direct column but there is an oracle** (queried 2026-09-18): `fbp(output = "All")`'s 43 columns include neither CBH nor CFL, but sending `CBH = -1` and inverting `CSI` (eq. 56) recovers the published table verbatim — `2, 3, 8, 4, 18, 7, 10, 0, 6, 6, 6, 6, 0, 0, 0, 0, 0`. What blocks this row is the exposure decision below, not the absence of a reference. |
| `crown_fuel_load.r` | per-fuel CFL defaults | 🔴 | — Caller must supply `Crown.CFL`. This table is what keeps D1/S1–S3/O1A/O1B at zero crown; without it that behaviour is the caller's to get right. Same as CBH above: recoverable as `CFC/CFB` once the mixedwood weighting is divided out (`crown_fuel_consumption` applies `PC/100` to M1/M2 and `PDF/100` to M3/M4), giving `0.75, 0.8, 1.15, 1.2, 1.2, 1.8, 0.5, 0, 0.8, 0.8, 0.8, 0.8, 0, 0, 0, 0, 0`. The six zero entries are not recoverable by division but are pinned by the `CFL > 0` gate — those fuels come back `CFB = 0`. |
| `C6calc.r` | C6 crown ROS (RSC), CFB-blended final ROS | 🔴 | — C6 is the one fuel whose ROS depends on CFB, and gofbp's C6 is surface-only. **Corrected 2026-09-18: "every oracle test excludes C6 by name" was false.** Six tests exclude it via `crownChangesROS`; `TestCFFDRSCrownThreshold` deliberately includes it and says so, because CSI and RSO are the same equations for every fuel. **Its oracle is already in the fixture on disk**: of 948 C6 cases, 768 are crown-block rows with explicit CBH and `CFL = 1.0`, and their `ros`/`cfb` columns are the blended quantity. Upstream's `crown_rate_of_spread_c6(ISI, FMC)`, `crown_fraction_burned_c6(RSC, RSS, RSO)` and `rate_of_spread_c6(RSC, RSS, CFB)` consume ISI, FMC, RSS, RSO and CFB only — **no CBH or CFL table**. So this row needs no generator change, no regeneration and no digest move. |
| `total_fuel_consumption.r` | CFC, TFC | 🔴 | — **both blockers are clear, and the second was never real.** SFC landed 2026-09-18. The CFL half was never a dependency on the row below: upstream's `total_fuel_consumption(FUELTYPE, CFL, CFB, SFC, PC, PDF)` takes CFL as a *parameter* — `fbp()` substitutes the table before calling it — so under this package's caller-supplies convention TFC is SFC (✅) + CFB (✅) · `Crown.CFL`, which is already a field. Note CFC carries `PC/100` for M1/M2 and `PDF/100` for M3/M4, not merely CFB·CFL. What remains is generator work rather than a port dependency: `TFC` and `CFC` are in `fbp()`'s contract but are not emitted, so adding them moves the digest, and any test must restrict to `cfl > 0` rows because the surface blocks send `CFL = -1` and their TFC embeds the table value. |
| `fire_intensity.r` | HFI | 🔴 | — needs TFC first. Oracle-confirmed: `fire_intensity(FC, ROS) = 300·FC·ROS`, called as `HFI <- fire_intensity(TFC, ROS)`. This is the only real dependency edge among the 🔴 rows. `HFI` is in `fbp()`'s contract but is not emitted. For C6 the ROS factor is the crown-blended rate, so a C6-inclusive HFI also wants `C6calc.r`. |
| `rate_of_spread_at_time.r`, `distance_at_time.r`, `length_to_breadth_at_time.r` | acceleration model | 🔴 | — the whole time-dependent branch. Nothing here is time-aware. **Not blocked by any 🔴 above it** (queried 2026-09-18): `alpha` depends on fuel type and CFB (✅) alone, and the inputs are ROS/BROS/LB (✅). It is blocked by the *sweep*: `fbp()` gates every t-column on `ACCEL`, and the generator pins `Accel = 0` and `hr = 1` on all 23,532 rows, so `HROSt`/`FROSt`/`BROSt`/`LBt`/`DH`/`DB`/`DF`/`TI` currently all carry their equilibrium values. Reaching it needs an `Accel = 1` block and an `hr` sweep — the largest generator change of any 🔴 row, and the digest moves. Watch `HR <- HR*60`: hours in, minutes in the model. |
| `fbp.r`, `fire_behaviour_prediction.r` | the umbrella driver | 🟡 | — gofbp exposes the pieces, not one `fbp()`-shaped call. Deliberate for now; revisit only once the 🔴 rows above close, since a driver that silently defaults FMC/CBH/CFL is exactly the local judgement this package refuses to make. (SFC was in that list until 2026-09-18 and is now ✅, so it is the one of the four a driver could be handed rather than invent.) Upstream's own driver shows what is being refused: `fbp()` called with **no input at all** returns a complete prediction, defaulting everything from hard-coded per-fuel `CBHs`/`CFLs` vectors. |
| `buildup_index.r`, `drought_code.r`, `duff_moisture_code.r`, `fine_fuel_moisture_code.r`, `fire_weather_index.r`, `fwi.r` | the FWI System | ⚪ | Inputs to FBP, not part of it. gofbp takes FFMC/BUI/ISI as given. |
| `gfmc.r`, `grass_fuel_moisture*.r`, `hffmc.r`, `hourly_fine_fuel_moisture_code.r`, `sdmc.r` | hourly/grass/duff moisture codes | ⚪ | Same boundary as above. |
| `fire_season.r`, `overwinter_drought_code.r` | seasonal bookkeeping | ⚪ | Not FBP. |
| `fbpRaster.r`, `fwiRaster.r`, `gfmcRaster.R`, `hffmcRaster.r` | raster wrappers | ⚪ | Gridding is the caller's; the Go package stays dependency-free and scalar. |
| `lros.r`, `pros.r` | line/point ROS from observed arrival times | ⚪ | Inference from observations, not the FBP System's forward equations. |
| `cffdrs-package.R` | roxygen package docs | ⚪ | Not code. |

## Non-`R/` directories

| Upstream | Status | Note |
|---|---|---|
| `data/` | 🟡 | **Both halves of this row were wrong until 2026-09-18, and the oracle says so.** `data(package="cffdrs")` lists exactly nine `test_*` sample input sets — `test_fbp`, `test_fwi`, `test_gfmc`, `test_hffmc`, `test_lros`, `test_pros`, `test_sdmc`, `test_wDC`, `test_wDC_fs` — and **no fuel-type table**. The per-fuel CBH/CFL defaults do **not** live here either: they are hard-coded vectors inside `R/crown_base_height.r` and `R/crown_fuel_load.r`, which carry their own 🔴 rows above, so this row was also double-counting them. gofbp transcribed the ST-X-3 fuel tables by hand from the publication and checks them against the fixture; there is no upstream data object to port them from. Left 🟡 rather than moved — with nothing here to port, ⚪ is arguable, and that is a status call. |
| `tests/` | 🟡 | Not mirrored. gofbp asserts against a generated 23,532-case sweep instead (`testdata/gen_cffdrs_reference.R`). Upstream's own test cases are still worth reading for edge cases the sweep does not reach — but note R does not install `tests/`, so they are not readable from the pinned container and need the upstream tree. What *is* installed and readable is `inst/extdata` (see below), which pairs with `data/test_fbp` as upstream's own FBP regression case. |
| `man/`, `inst/` | ⚪ | Docs and package metadata — **true of `man/`, and not true of `inst/`.** Queried 2026-09-18: the installed package ships `inst/extdata/test_fbp_out.Rdata`, a 20×9 table of upstream-published FBP reference output (`ID, CFB, CFC, FD, HFI, RAZ, ROS, SFC, TFC`) — the same quantities gofbp asserts, plus the TFC/HFI it still owes — alongside raster fixtures. That is the same content class as the 🟡 `tests/` row, sitting under a reason that says there is nothing here. Splitting `inst/` out of this row moves content out of ⚪, so it is left for a human; see the open question above. |
| `NEWS.md` | — | **Read on every version bump.** It is the cheapest signal that a coefficient moved. |

## Concepts still missing, in dependency order

The 🔴 rows are not independent. Doing them out of order means writing code that
cannot be oracle-tested yet.

> **Measured 2026-09-18: this is not a dependency order.** The call graph was
> queried against the pinned oracle rather than read, and of the edges this list
> asserts, only two exist:
>
> | stated edge | real? | why |
> |---|---|---|
> | 2 → 3 (CBH/CFL before C6) | **no** | C6's three functions take `ISI, FMC, RSS, RSO, CFB` — no table |
> | 2 → 4 (CBH/CFL before a complete TFC) | **no** | `total_fuel_consumption` takes CFL as a *parameter*; `fbp()` substitutes the table before the call |
> | 4 → 4 (TFC before HFI) | yes | `HFI <- fire_intensity(TFC, ROS)` |
> | everything → 6 (driver last) | yes | by definition |
> | 1, 5 | independent | FMC blocked by provenance and sweep; acceleration by the sweep only |
>
> So the sentence above — "doing them out of order means writing code that cannot
> be oracle-tested yet" — is **false for items 3 and 4.** Both can be oracle-tested
> today, and item 3 against the fixture that already exists. What remains is a cost
> and design order, not a correctness one.
>
> **The list is deliberately left in its current order anyway.** Item 4's own note
> says promotion "is a scheduling call for a human, not a correctness one", and
> that reservation outlives the finding that produced it: the audit's job is to say
> the edges are not real, not to decide what to do about it. There is also an
> ordering argument in the other direction that a human should weigh — C6's ROS
> feeds HFI (`300·TFC·ROS`) and the acceleration model, so doing C6 first avoids
> revisiting both later to lift its exclusion. Item 2's "ask before choosing" still
> gates any CBH/CFL work regardless of position.

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
   silent default inside `Crown`. Ask before choosing. Note the exposure decision
   is now the *only* thing blocking this row: both tables were recovered verbatim
   from the pinned oracle on 2026-09-18 (CBH by inverting eq. 56, CFL as
   `CFC/CFB`), so "no column" was true and "no oracle" was not.
3. **C6 RSC** (`C6calc.r`) — removes the exclusion that six oracle tests carry
   (**not** "every oracle test": `TestCFFDRSCrownThreshold` includes C6 on
   purpose). **"Do not attempt before CBH/CFL land" was wrong and is withdrawn** —
   C6's ROS is the CFB blend, but none of `crown_rate_of_spread_c6`,
   `crown_fraction_burned_c6` or `rate_of_spread_c6` touches a CBH or CFL table;
   they take `ISI, FMC, RSS, RSO, CFB`. This is the only 🔴 row whose oracle is
   already in the fixture on disk — 768 of 948 C6 cases are crown-block rows whose
   `ros`/`cfb` are the blended quantity — so it is the only one needing no
   generator change and no digest move. First step: take one of those rows and
   check that `rate_of_spread_c6(crown_rate_of_spread_c6(isi, fmc), RSS, cfb)`
   reproduces its `ros` column. If it does, the exclusion is the single switch. If
   it does not, the row has a finding in it and nothing should be ported until
   that is understood.
4. **TFC, HFI** (`total_fuel_consumption.r`, `fire_intensity.r`) — SFC landed on
   2026-09-18, so TFC's surface half (eq. 60) is unblocked. **The crown half is
   not blocked on item 2 either, and the claim that it was is withdrawn:**
   `total_fuel_consumption` takes CFL as a parameter, so under this package's
   caller-supplies convention TFC needs only SFC (✅), CFB (✅) and `Crown.CFL`.
   (CFC is CFB·CFL weighted by `PC/100` for M1/M2 and `PDF/100` for M3/M4 — not
   CFB·CFL flat.) HFI (eq. 66) genuinely waits on TFC; that is the one real edge
   left in this list. What both still need is a generator change: neither `TFC`,
   `CFC` nor `HFI` is emitted, so adding them moves the digest. Left here rather
   than promoted — moving it up is a scheduling call for a human, not a
   correctness one, and that is still true now that the correctness argument for
   its position has gone.
5. **Acceleration model** (`rate_of_spread_at_time.r`, `distance_at_time.r`,
   `length_to_breadth_at_time.r`) — the largest remaining block, and the only one
   that changes the package's shape (it introduces time).
6. **An `fbp()`-shaped driver** (`fbp.r`) — last, or never. See the row above.

## Log

Newest first. One line per audit; a day with no change still gets a line, so a
gap in the dates is visible as a gap.

| Date | Upstream commit | What changed |
|---|---|---|
| 2026-09-18 | `4d20a30` | Upstream unchanged (`upstream-drift` exit 0); gate green (`precheck` exit 0, 0 of 14 skipping, digest `148a5a9e…24cf8f`); no port, no generator change, no regeneration. **First `/migration-sweep` — the judgement half fanned out across the five status groups, every verdict backed by a query against the pinned container rather than a reading.** It found more wrong than the daily pass had, and the pattern is one thing: *claims about upstream that nobody had ever asked upstream about.* **The dependency order is not a dependency order** — of its stated edges only TFC→HFI and "driver last" exist. C6 does **not** depend on CBH/CFL (`crown_rate_of_spread_c6`/`crown_fraction_burned_c6`/`rate_of_spread_c6` take `ISI, FMC, RSS, RSO, CFB` and no table), and TFC does not either (`total_fuel_consumption` takes CFL as a *parameter*; `fbp()` substitutes the table before the call). Both rows' blockers are withdrawn. The list is left in its order anyway — item 4 reserves promotion for a human, and that survives the finding. **C6 is the top unblocked 🔴 and needs no regeneration**: 768 of its 948 fixture cases are crown-block rows whose `ros`/`cfb` already are the blended quantity. "Every oracle test excludes C6 by name" was false — six do, and `TestCFFDRSCrownThreshold` includes it deliberately. **CBH/CFL have no column but do have an oracle**: both published tables were recovered verbatim (CBH by inverting eq. 56, CFL as `CFC/CFB` net of the M1/M2 `PC/100` and M3/M4 `PDF/100` weightings), so only the exposure decision blocks them. **`foliar_moisture_content_minimum` does not exist at the pin at all**, which makes half that row unportable against 1.9.2 by construction rather than unswept. **`data/` was wrong twice over** — nine `test_*` sample sets, no fuel-type table, and the CBH/CFL defaults are vectors in the R sources, not data objects, so the row was double-counting two 🔴 rows. **`inst/` is not "docs and metadata"**: `inst/extdata/test_fbp_out.Rdata` is a 20×9 table of upstream-published FBP reference output. The 🟢 `direction.r` row survived on narrower ground than it claimed — `pros()`/`lros()` *are* exported and *do* return a `Direction` column, so "no oracle column exists" is false as stated, but it is not an oracle for `AngleBetweenDeg`, which is a different function (`.direction(10,40,15)` = `.direction(40,10,15)` = 25 where `AngleBetweenDeg(10,40)` = 30). Among the exclusions: `crownChangesROS` is a sound mechanism and has only ever narrowed, but `TestCFFDRSCrownFractionBurned`'s C6 reason was **false about upstream** — C6's CFB is eq. 58 on the surface rate like every other fuel, RSC only gates — and its pass two excludes 768 rows that match exactly, so that exclusion hides agreement; the comment is corrected here, the exclusion is left alone as a decision. Three exclusions are dead (`c.ROS == 0` cannot fire — upstream floors ROS at 1e-6 and the fixture's minimum is 3.26e-10 but never 0; `nearBoundary` skips 0; and a silent `angleName` lookup miss with no comment at all), and one has widened to remove 4,992 rows that all pass the contract it also silences. Yesterday's `rate_of_spread_at_theta.r` note was re-verified adversarially and every sub-claim and all four numbers held; corrected only in that it understated the propagation (`TTFC` too) and quoted outputs without naming the inputs, which cost the auditor a 136,000-row grid search — the case is now named in the row. Four status/scope calls are parked above rather than taken. |
| 2026-09-18 | `4d20a30` | Upstream unchanged (`tools/upstream-drift` exit 0: HEAD still `4d20a30`, so there are no `NEWS.md` versions between the pin and now); no port. Gate green (`precheck` exit 0: READY, `go test ./...` green, 0 of 14 `TestCFFDRS*` skipping, fixture `148a5a9e…24cf8f`). **Targeted sweep on the two rows the previous audit left open — and the first overturns that audit's own correction.** `rate_of_spread_at_theta.r` was measured against the pinned container this time rather than read, and the row had conflated two different upstream quantities: `fbp()`'s `TROS` (`ROS·(1-E)/(1-E·cos(THETA-RAZ))`, built from ROS and LB only) is **not** eq. 94's `rate_of_spread_at_theta(ROS, FROS, BROS, THETA)`, which is unexported and never called by `fbp()`. **Both are numerically defective**, so the predicted "emit `TROS` and the row becomes ✅" was never on the table. `TROS` subtracts `RAZ` in degrees from `THETA` in radians inside one `cos()` (1.9.2 lines 134 / 232 / 249, and the same three lines at `4d20a30`), so it misses even the head rate — 2.835 where `ROS` is 16.149 at `RAZ` = 180° — and carries that into `TCFB`, `TFI`, `TTI` and `TROSt`. Eq. 94's two leading terms share a denominator and collapse to `ROS/cosθ`: an uncaught pole at 90° (-2.34e17, because the `ifelse(c1 == 0, …)` guard never fires — `cos(π/2)` is 6.12e-17), negative rates from 30° to 89.9°, and `ROS` returned at *both* 0° and 180° so `BROS` never comes back. `ROSAtAngle` on the same case gives `ROS` at 0° and `BROS` to seven figures at 180°. Row left 🟡 with the evidence written into it; on that evidence the honest status is 🟢, which is now an open question above — as is reading the *published* eq. 94, which was **not** checked and decides whether this is cffdrs' transcription error or the paper's. The disproved "no rate at a bearing" claim was still live in four places the previous fix missed (`ellipse.go` header and `ROSAtAngle` doc, `ellipse_test.go`, `README.md`) and in `/migration-port`, where it was the **worked example of oracle case (c)** — all five corrected, and the command now carries the lesson ("no column exists" is a claim about upstream, so check upstream for it) rather than the claim. Corrected in passing: `ROSAtAngle`'s doc credited "monotone decrease from head to back", which its own preceding paragraph and `TestROSAtAngleIsUnimodalWithTheHeadAsMaximum` both contradict. `direction.r` re-confirmed against the oracle and unchanged: `.direction` has three formals, is unexported, and is called by exactly `lros` and `pros` (both ⚪) — not by `fbp` or `fire_behaviour_prediction`. No generator change, no regeneration, digest untouched. |
| 2026-09-18 | `4d20a30` | Upstream unchanged (`tools/upstream-drift`: HEAD still `4d20a30`); no port. **First check in this worktree actually backed by the oracle.** The fixture was generated from the pinned container and reproduced `148a5a9e…24cf8f` byte for byte, so the recorded digest is now confirmed by regeneration rather than carried forward, and all 14 `TestCFFDRS*` assert instead of skipping (`precheck`: READY, 0 of 14 skipping). Three claims in this file were wrong and are corrected above. (a) **1.10.0 is not a CRAN release** — `remotes::install_version` rejects it and the archive stops at the current 1.9.2, which is exactly what the oracle pins, so the standing "open drift" was never a bump somebody had not got to; it is a question about whether reference numbers may come from git HEAD, and it is now written down as one. (b) **`rate_of_spread_at_theta.r` does have an oracle column** — `fbp(output = "ALL")` returns `TROS`; the row was 🟢 on the claim that no column exists, and is now 🟡 with the column named. (c) 1.9.2 already returns `D0`. Also fixed `testdata/regen-cffdrs.sh`: under Git Bash it handed `docker build` an MSYS-form context path (`/c/...`) with `MSYS_NO_PATHCONV` already exported for the `docker run` mount, so **every first-time image build failed on Windows** while re-running an already-built image worked — which is why it stayed hidden. Found by trying to build a 1.10.0 oracle; with the path fixed, that build ran far enough to establish (a). |
| 2026-09-18 | `4d20a30` | Upstream unchanged (`tools/upstream-drift`: HEAD still `4d20a30`). **Ported SFC** — `surface_fuel_consumption.r` → `consumption.go`, all eleven equations, 🔴 → ✅. Oracle case (a): `sfc` was already a fixture column carried as an input, so the generator did not change and the fixture is byte-identical — `tools/fixture-diff` over the pre-port copy reports *No column shared by both fixtures moved*, digest still `148a5a9e…24cf8f`. All 23,532 cases assert, worst relative error 4.01e-16. Two places cffdrs departs from FCFDG 1992 are now written down and both are oracle-confirmed: C1 uses GLC-X-10's eqs. 9a/9b, not eq. 9 (documented); C7's eq. 13 term is clamped at FFMC 70 (**not** documented upstream). The `SFC <= 0 → 1e-6` floor is reproduced from the reference implementations but the sweep never reaches it — `TestCFFDRSSurfaceFuelConsumptionFloorIsUnreached` says so out loud. No default for SFC or GFL was added; `Crown.SFC` is still the caller's field. |
| 2026-09-06 | `4d20a30` | No upstream change; no port. Turned the mechanical half of this procedure into code: `ledger_test.go` for the step-4 sweep, and `tools/precheck`, `tools/upstream-drift`, `tools/fixture-diff` for the gate, the upstream join and the reference-number diff. The digest re-baseline below was re-derived independently here and reached the same `148a5a9e…`; `tools/fixture-diff` against a rebuilt pre-M3/M4 fixture then showed no reference number moved, across the 18804 cases the two sweeps share. Sweep is 23,532 cases exactly. |
| 2026-09-06 | `4d20a30` | Upstream unchanged (HEAD still `4d20a30`); no port. Audit found three stale claims and fixed them: the fixture digest still named the pre-M3/M4 sweep (regenerated from the pinned container, byte-identical, `148a5a9e…24cf8f`), the `rate_of_spread.r` row still said 15 fuel types, and the sweep is ~23500 cases, not ~18400. Checked the `direction.r` equivalence the row asked about: it does not hold. |
| 2026-09-06 | `4d20a30` | Ledger created. Full `R/` inventory taken against upstream 1.10.0; oracle-vs-upstream version drift recorded as an open item. |
