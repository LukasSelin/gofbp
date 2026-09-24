# Emit the FBP and FWI System reference fixture from the cffdrs R package.
#
# FBP is the whole of this file up to "=== The FWI System ===", and lands in the
# fixture's "cases"; the FWI System is the block after it, and lands in
# "fwi_cases". The two share nothing but the file and the pins.
#
#   testdata/regen-cffdrs.sh
#
# That wrapper pins R and cffdrs in a container, so regenerating needs Docker and
# nothing else. To drive this file directly instead, run it from the MODULE ROOT --
# the output path below is relative to it:
#
#   Rscript testdata/gen_cffdrs_reference.R
#
# Writes testdata/cffdrs.json: a grid of FBP inputs and what the *authoritative*
# implementation produces for each. Go's TestCFFDRS* assert this package
# reproduces them.
#
# Why this exists: every coefficient in this package was typed in by hand from
# ST-X-3, and a transcription error looks exactly like correct code. cffdrs is
# maintained by the Canadian Forest Service authors of the system itself, so it
# is the only oracle available that can say the tables are actually right, and
# the only cross-implementation check the package has at all. One such error was
# found this way (the grass curing branch, 2026-08-18, 2.2x too fast at full
# curing).
#
# R is needed to REGENERATE the fixture, never to run the tests. The JSON is NOT
# committed: generate it once and the Go side reads it with no R involved, and
# the TestCFFDRS* tests skip until you do. Re-run this after touching a
# coefficient, and on a cffdrs upgrade.
#
# Requires: R, and install.packages("cffdrs") -- or just Docker, via
# regen-cffdrs.sh. cffdrs Imports sf and terra, so a local install also needs
# GDAL, GEOS and PROJ; the container is usually the shorter path.

suppressMessages(library(cffdrs))

OUT <- file.path("testdata", "cffdrs.json")

# --- the sweeps ------------------------------------------------------------
#
# ISI is not an fbp() input: it is derived from FFMC and wind. Sweeping both is
# how this reaches a wide ISI range, and on FLAT ground that costs nothing --
# cffdrs' WSV reduces to WS and ROS is still RSI(ISI) x BE, exactly the quantity
# this package computes. Slope is what pulls the two apart (see below), so it is
# swept separately.
FFMC_VALUES <- c(60, 75, 85, 90, 92, 95)
WS_VALUES <- c(0, 5, 15, 30, 50)
BUI_VALUES <- c(1, 20, 40, 64, 100, 150)
# Slope in PERCENT rise, bracketing the 70% saturation from both sides.
GS_VALUES <- c(0, 5, 15, 30, 45, 60, 69.9, 70, 100, 200)
PC_VALUES <- c(0, 25, 50, 75, 100)   # M1/M2 conifer share
# M3/M4 dead balsam fir share. This is a DIFFERENT input from PC, weighting a
# different pair of curves (eqs. 29/33 against eq. 27), and it reaches only M3
# and M4 -- cffdrs' M2 is weighted by PC, not PDF, despite its name. The
# endpoints are both here because they are where the blend collapses to
# something nameable: at 100 to the fuel's own eq. 30 curve, at 0 to D1 alone
# (times 0.2 for M4).
PDF_VALUES <- c(0, 25, 35, 60, 100)
CC_VALUES <- c(20, 50, 80, 100)      # grass curing
# Crown-fire threshold sweeps. CBH is metres to the base of the crown and LAT/Dj
# are how foliar moisture content is reached -- FMC is a function of latitude,
# longitude, elevation and how far the day of year is from the annual minimum, so
# sweeping Dj is the only way to move it. See the crown block below.
CBH_VALUES <- c(2, 3, 7, 20)
LAT_VALUES <- c(45, 60)
# Chosen against where the FMC minimum actually falls, not spread evenly over the
# year. FMC is 120 flat once the day of year is 50 days or more from that
# minimum (eq. 8), and the minimum sits near day 147 at LAT 45 and near day 196 at
# LAT 60 for this sweep's longitude -- so an even spread puts most rows on the
# plateau and the 25.9 coefficient in eq. 56 is then pinned at one value. These
# four straddle both minima and reach the quadratic branch (eq. 6) and the linear
# one (eq. 7) as well as the plateau. Check the count TestCFFDRSCrownThreshold
# logs if you change them.
DJ_VALUES <- c(150, 175, 200, 240)

CONIFER <- c("C1", "C2", "C3", "C4", "C5", "C6", "C7")
SIMPLE <- c(CONIFER, "D1", "S1", "S2", "S3")
# The two mixedwood families are swept separately because they take different
# blend inputs. Sweeping both inputs over both families would quadruple the block
# to buy nothing: PDF does not reach M1/M2 and PC does not reach M3/M4, so the
# extra rows would be exact duplicates of rows already here.
MIXED_PC <- c("M1", "M2")
MIXED_PDF <- c("M3", "M4")
GRASS <- c("O1a", "O1b")

# fbp() wants every column present for every row, so unused ones get harmless
# in-range defaults: they do not enter the fuels that ignore them.
#
# CBH and CFL default to -1, the sentinel that makes fbp() substitute its own
# per-fuel table. That is deliberate for the surface sweeps -- they assert
# quantities crown fire does not enter -- but it makes those rows USELESS for
# asserting the crown threshold, because the fixture would record the -1 we sent
# rather than the value cffdrs actually used, and fbp() does not return either
# one. The crown block below therefore passes both explicitly, inside the ranges
# fbp() honours verbatim: CBH in (0, 50] and CFL in (0, 2]. Send a value outside
# those and it is silently replaced by the table, which is the same trap again.
base_row <- function(fuel, ffmc, bui, ws, gs, pc = 50, pdf = 35, cc = 80,
                     cbh = -1, cfl = -1, lat = 60, dj = 200,
                     long = 15, elv = 0, d0 = 0) {
  data.frame(
    FuelType = fuel, LAT = lat, LONG = long, ELV = elv, Dj = dj, D0 = d0,
    FFMC = ffmc, BUI = bui, WS = ws, WD = 0, GS = gs, Aspect = 0,
    PC = pc, PDF = pdf, cc = cc, GFL = 0.35, CBH = cbh, CFL = cfl,
    hr = 1, theta = 0, Accel = 0, montane = 0,
    stringsAsFactors = FALSE
  )
}

rows <- list()
add <- function(df) rows[[length(rows) + 1L]] <<- df

# Flat ground, wide ISI: the coefficient check.
for (fuel in SIMPLE) {
  for (ffmc in FFMC_VALUES) for (ws in WS_VALUES) for (bui in BUI_VALUES) {
    add(base_row(fuel, ffmc, bui, ws, 0))
  }
}
for (fuel in MIXED_PC) {
  for (ffmc in FFMC_VALUES) for (ws in WS_VALUES) for (bui in BUI_VALUES) {
    for (pc in PC_VALUES) {
      add(base_row(fuel, ffmc, bui, ws, 0, pc = pc))
    }
  }
}
for (fuel in MIXED_PDF) {
  for (ffmc in FFMC_VALUES) for (ws in WS_VALUES) for (bui in BUI_VALUES) {
    for (pdf in PDF_VALUES) {
      add(base_row(fuel, ffmc, bui, ws, 0, pdf = pdf))
    }
  }
}
for (fuel in GRASS) {
  for (ffmc in FFMC_VALUES) for (ws in WS_VALUES) for (bui in BUI_VALUES) {
    for (cc in CC_VALUES) add(base_row(fuel, ffmc, bui, ws, 0, cc = cc))
  }
}
# Sloped ground. These rows now carry two jobs. They assert SF, WSV and the full
# back-solve path (TestCFFDRSSlopeBackSolve), and they measure what the simplified
# RSI x BE x SF product costs relative to it (TestCFFDRSSlopeDivergence), which is
# the specification of fbp.ROS's upper-bound contract.
# Wind must be swept here, not just slope. At WS = 0 the back-solve is a no-op
# round trip -- it recovers exactly the ISI that gives RSZ x SF, so the simplified
# product is not an approximation at all and measuring there says nothing. The gap
# opens once there is real wind to vector-add to the slope-equivalent wind, and it
# depends on the ANGLE between them: WD 0 against Aspect 0 is wind driving straight
# upslope, WD 180 is wind fighting it. That angle is exactly the wind-slope
# alignment a per-pixel overlay expresses through that back-solve.
#
# WS/FFMC/BUI values are drawn from the flat sweeps above so every sloped row has
# a flat counterpart to take its wind-only ISI from.
#
# The fuel list includes M1/M2 to cover eq. 42's mixedwood ISF blend. Go's
# slopeEquivalentISF blends the ISF of C2 and D1 rather than blending RSF and
# inverting once, and drops M2's 0.2 dead-fir weighting in the slope path -- both
# read off cffdrs' source rather than measured, and M1 is a common real-world
# mapping. Without these rows TestCFFDRSSlopeBackSolve logs that the blend is
# unoracled and moves on.
#
# FFMC is swept rather than fixed at 85 because 85 is too wet to reach the
# interesting branches: the largest equivalent wind anywhere in an 85-only sweep
# is 38.7 km/h, just under fbp.HighWindKmh. FFMC 95 crosses it, and also reaches
# both fbp.EquivalentWindCapKmh and the isfClampMin guard -- three branches that
# are otherwise transcribed but never checked against the oracle.
#
# M3/M4 are here for eqs. 42b/42c, which are eq. 42's construction with the
# fuel's own eq. 30 curve in C2's place. cffdrs reaches that pure component by
# calling its own rate_of_spread with PDF forced to 100, and it drops M4's 0.2
# deciduous weighting in the slope path exactly as it drops M2's -- two readings
# taken off the R source rather than measured, and TestCFFDRSSlopeBackSolve is
# what turns them into assertions. PDF stays at base_row's 35 here: the weight
# only has to be non-degenerate to pin the blend, and sweeping it would multiply
# this block, which is already the largest one.
for (fuel in c("C2", "C3", "D1", "S1", "O1b", "M1", "M2", "M3", "M4")) {
  for (ffmc in c(85, 95)) {
    for (gs in GS_VALUES) for (ws in c(0, 5, 15, 30)) for (wd in c(0, 90, 180, 270)) {
      for (bui in c(40, 100)) {
        r <- base_row(fuel, ffmc, bui, ws, gs)
        r$WD <- wd
        add(r)
      }
    }
  }
}

# Crown-fire threshold. A block of its own rather than more columns on the sweeps
# above, for two reasons: it keeps the growth bounded (this is ~9200 rows against
# the ~11500 that were here before), and it left every then-existing test's case
# count untouched, so a regeneration that changed one of them was a real signal
# rather than a side effect of that addition.
#
# NOTE: adding M3/M4 DID move those counts, deliberately and everywhere -- two
# new fuels in the flat, sloped and crown blocks, PDF_VALUES widened from 3
# values to 5, and M2's pointless PDF sweep dropped (PDF does not reach M2). A
# regeneration across that change is expected to renumber every block. It is the
# one commit where a changed count is not a signal; after it, the rule above
# applies again.
#
# What has to VARY, and why each one is here:
#
#   CBH  drives CSI directly (eq. 56) and is the input a caller is most likely to
#        get from stand inventory rather than a table. 2 m to 20 m spans the
#        published per-fuel values.
#   LAT  and Dj are the only handles on FMC. FMC is not an fbp() input the way
#        ISI is not -- it is derived from location and the distance in days from
#        the annual minimum, so a fixture at one latitude and one date pins the
#        crown equations at a single foliar moisture and says nothing about the
#        25.9 coefficient. See DJ_VALUES for how the dates were picked; LAT 45/60
#        moves where the minimum falls, which is what makes the same date reach a
#        different moisture.
#   GS   at 0 and 30 % because CFB is computed from the SURFACE rate on the full
#        slope path (RSI(ISI(WSV)) x BE, no SF), so a flat-only sweep would not
#        check that the slope reaches the threshold through the back-solve.
#
# CFL is FIXED at 1.0 and that is not an oversight. It enters none of the
# quantities emitted here: fbp() uses it as a gate on CFB, zeroing it where CFL is
# not positive, and otherwise only in the consumption outputs this fixture does
# not carry. Sweeping it would buy nothing, and the one value worth testing -- 0, the published entry for the
# fuels that have no crown -- cannot be sent, because fbp() reads a non-positive
# CFL as "use the table". The gate is asserted in crown_test.go instead.
#
# C6 is included, and only its CSI and RSO are usable. C6 is the single fuel
# whose ROS depends on CFB, through a separate crown rate of spread this package
# does not implement, so its cfb and ros columns are a different quantity. The Go
# side excludes it by name and says so.
for (fuel in c("C1", "C2", "C3", "C4", "C5", "C6", "C7", "D1",
               "M1", "M2", "M3", "M4", "S1", "O1b")) {
  for (ffmc in c(85, 92, 95)) for (bui in c(40, 100)) {
    for (ws in c(0, 30)) for (gs in c(0, 30)) {
      for (cbh in CBH_VALUES) for (lat in LAT_VALUES) for (dj in DJ_VALUES) {
        add(base_row(fuel, ffmc, bui, ws, gs, cbh = cbh, cfl = 1.0, lat = lat, dj = dj))
      }
    }
  }
}

# Foliar moisture content. A block of its own, for the same reason the crown one
# is: FMC depends on NONE of the inputs the sweeps above vary. It is a function of
# LAT, LONG, ELV, Dj and D0 alone, so crossing its drivers into any existing block
# would multiply thousands of rows to say the same thing the ~200 below say.
#
# Why it had to exist at all. Every one of the rows above sends LONG = 15, ELV = 0
# and D0 = 0, which means they exercise exactly one of the model's three paths:
#
#   eqs. 1, 2   the ELV <= 0 branch, at a SINGLE longitude. LATN is an
#               exponential in (150 - LONG), and one longitude pins a point on it,
#               not its shape -- 46, 23.4 and 0.0360 could each be wrong and the
#               fixture would not notice.
#   eqs. 3, 4   the ELV > 0 branch. UNREACHED. 43, 33.7, 0.0351, 142.1 and 0.0172
#               had no oracle coverage whatsoever.
#   D0 given    the caller-supplied minimum date, which bypasses eqs. 1-4
#               entirely. UNREACHED.
#
# Each site gets its OWN LATITUDE, and that is load-bearing rather than tidy.
# tools/fixture-diff keys a case by its input columns to compare two fixtures, and
# LONG/ELV/D0 do not exist in any fixture generated before this block did. Two
# sites differing only in those three would therefore be indistinguishable to a
# diff against an older fixture, and it would report ambiguous keys and stop being
# believable. Distinct latitudes keep every row identifiable under the old key as
# well as the new one. BUI is 60 for the same reason: it is in none of the sweeps
# above, so no row here can collide with one of theirs either.
#
# The fuel is C2 throughout. FMC does not vary by fuel -- but fbp() ZEROES it for
# D1/S1/S2/S3/O1A/O1B, the fuels with no crown, so the block has to use one that
# keeps it. That zeroing is already oracled thousands of times over by the blocks
# above; see the Go side, which excludes those fuels by name.
FMC_SITES <- list(
  # Eqs. 1, 2 -- ELV <= 0, swept across the longitudes eqs. 1/3 are written for
  # (52 to 140 degrees WEST, positive). Six points on the LATN exponential.
  list(lat = 35, long = 60, elv = 0, d0 = 0),
  list(lat = 40, long = 80, elv = 0, d0 = 0),
  list(lat = 50, long = 100, elv = 0, d0 = 0),
  list(lat = 55, long = 120, elv = 0, d0 = 0),
  list(lat = 65, long = 140, elv = 0, d0 = 0),
  list(lat = 70, long = 52, elv = 0, d0 = 0),
  # fbp() folds the sign of LONG (LONG <- ifelse(LONG < 0, -LONG, LONG)) before
  # calling foliar_moisture_content, so a caller may hand it the conventional
  # signed longitude. This row is the one that says so: -60 must give what +60
  # gives. Without it the Go test's math.Abs would be an assumption. If the fold
  # were absent, (150 - (-60)) is 210 and D0 lands on 118 rather than 116.
  list(lat = 36, long = -60, elv = 0, d0 = 0),
  # Eqs. 3, 4 -- ELV > 0. Elevation and longitude both vary, and 0.0172 per metre
  # needs the spread: 200 m contributes 3.4 days, 3000 m contributes 51.6.
  list(lat = 37, long = 60, elv = 200, d0 = 0),
  list(lat = 42, long = 80, elv = 800, d0 = 0),
  list(lat = 48, long = 100, elv = 1500, d0 = 0),
  list(lat = 52, long = 120, elv = 2500, d0 = 0),
  list(lat = 58, long = 140, elv = 400, d0 = 0),
  list(lat = 68, long = 52, elv = 3000, d0 = 0),
  # D0 supplied -- eqs. 1-4 are bypassed, so LAT/LONG/ELV do nothing here except
  # keep the rows distinct (see above). The first two are ordinary integer dates.
  list(lat = 33, long = 90, elv = 0, d0 = 120),
  list(lat = 63, long = 90, elv = 900, d0 = 200),
  # The last three pin the ROUNDING, which is not in the paper at all: cffdrs
  # 1.9.2 rounds D0 -- including a supplied one -- with R's round(), which goes to
  # the EVEN digit on an exact half. 150.5 is the case that separates the two
  # rules: half-to-even gives 150, half-away-from-zero (Go's math.Round) gives
  # 151, and the two differ in FMC wherever ND is inside eq. 6's window. 151.5 is
  # the control the two rules agree on, and 150.7 is ordinary rounding.
  list(lat = 34, long = 90, elv = 0, d0 = 150.5),
  list(lat = 39, long = 90, elv = 0, d0 = 151.5),
  list(lat = 44, long = 90, elv = 0, d0 = 150.7)
)
# Spread across the year rather than around any one site's minimum: D0 runs from
# 113 to 271 across the sites above, and these twelve dates put every one of them
# on all three of eqs. 6 (ND < 30), 7 (30 <= ND < 50) and 8 (ND >= 50). The Go
# side counts the three and fails if any is empty, so narrowing this list is
# caught rather than quietly halving what the block asserts. 90/150/170 also land
# ND on exactly 30 and exactly 50 for the D0 = 120 site, which is where a
# mistranscribed < for <= would show.
FMC_DJ_VALUES <- c(1, 60, 90, 110, 130, 150, 170, 190, 210, 240, 280, 330)

for (s in FMC_SITES) {
  for (dj in FMC_DJ_VALUES) {
    add(base_row("C2", 90, 60, 10, 0, lat = s$lat, dj = dj,
                 long = s$long, elv = s$elv, d0 = s$d0))
  }
}

# Crown and total fuel consumption. A block of its own, and for a reason the FMC
# block above should make familiar: the column existing is not the same claim as
# the column exercising the function.
#
# CFC is CFL x CFB, weighted by PC/100 for M1/M2 and by PDF/100 for M3/M4
# (eqs. 66a/66b/66c), and TFC is SFC + CFC (eq. 67). Every crown-block row above
# sends CFL = 1.0 -- fixed, deliberately, because CFL entered none of the columns
# this fixture used to carry. It enters CFC directly, and multiplying by one
# asserts nothing about the multiplication: eq. 66a would agree with the oracle
# across all 10752 crown-block rows if CFL were ignored entirely. So the factor
# needs its own sweep, and that is what this is.
#
# What has to vary:
#
#   CFL  the factor itself, four values across (0, 2]. Outside that range fbp()
#        silently substitutes its own table (crown_fuel_load: CFL <= 0 | CFL > 2
#        | NA), and the fixture would then record the value we SENT rather than
#        the one it used -- the same trap the CBH/CFL note above describes. None
#        of these is 1.0, so no row here collides with a crown-block row under
#        tools/fixture-diff's key, which includes cfl.
#   PC   for M1/M2 and PDF for M3/M4: the two weighting branches. Both include 0,
#        where the branch collapses CFC to zero whatever CFL is -- which is the
#        cheapest way to tell 66b/66c apart from 66a.
#   CBH  and FFMC and WS together decide whether the row crowns at all. CBH 2
#        with FFMC 95 and WS 30 crowns; CBH 20 with FFMC 85 and WS 0 does not, and
#        a CFB of zero is what pins CFC to zero and TFC to SFC alone.
#
# The plain group carries C6 as well as C2/C7, because C6's CFB comes from a
# different function (crown_fraction_burned_c6) and eq. 66a has to be indifferent
# to that. D1 is here for a sharper reason: its PUBLISHED crown fuel load is 0, so
# D1 never crowns through fbp()'s own table -- but CFL is a parameter of eq. 66a,
# not a per-fuel gate inside it, and sending an explicit CFL makes fbp() honour
# it. These rows are what says eq. 66a applies no fuel test beyond the four
# mixedwoods. They are not a claim that D1 stands have crowns.
#
# BUI is 60 and GS is 0 throughout: flat, so the Go side can run the whole chain
# from FFMC and wind, and off the BUI_VALUES grid so the block stays legible in a
# fixture diff.
CFC_CFL_VALUES <- c(0.3, 0.8, 1.6, 2.0)
CFC_PC_VALUES <- c(0, 25, 75)
CFC_PDF_VALUES <- c(0, 35, 60)
CFC_FFMC_VALUES <- c(85, 95)
CFC_WS_VALUES <- c(0, 30)
CFC_CBH_VALUES <- c(2, 20)

for (cfl in CFC_CFL_VALUES) {
  for (ffmc in CFC_FFMC_VALUES) for (ws in CFC_WS_VALUES) for (cbh in CFC_CBH_VALUES) {
    for (fuel in c("C2", "C6", "C7", "D1")) {
      add(base_row(fuel, ffmc, 60, ws, 0, cbh = cbh, cfl = cfl))
    }
    for (fuel in MIXED_PC) {
      for (pc in CFC_PC_VALUES) {
        add(base_row(fuel, ffmc, 60, ws, 0, pc = pc, cbh = cbh, cfl = cfl))
      }
    }
    for (fuel in MIXED_PDF) {
      for (pdf in CFC_PDF_VALUES) {
        add(base_row(fuel, ffmc, 60, ws, 0, pdf = pdf, cbh = cbh, cfl = cfl))
      }
    }
  }
}

inp <- do.call(rbind, rows)
# Carry an explicit ID. Without one, fbp() auto-assigns 1..n and returns its rows
# sorted by ID as a STRING -- for n > 9 that is 1, 10, 100, 1000, 2, ... and reading
# the output positionally pairs each input with some other row's answer. The
# resulting fixture looks entirely plausible (right shape, right magnitudes, wrong
# pairing) and would have turned this harness into a generator of false mismatches.
# Do not "simplify" this away.
inp$ID <- seq_len(nrow(inp))
cat(sprintf("running cffdrs over %d cases...\n", nrow(inp)))
out <- fbp(inp, output = "All")
if (nrow(out) != nrow(inp)) {
  stop("cffdrs returned ", nrow(out), " rows for ", nrow(inp), " inputs")
}
out <- out[order(as.integer(as.character(out$ID))), ]
if (!identical(as.integer(as.character(out$ID)), inp$ID)) {
  stop("could not realign cffdrs output with its input by ID")
}

# LB/BROS/FROS are the fire ELLIPSE: how elongated the fire is at the net
# effective wind, and how fast it runs backwards and sideways. The head rate
# alone cannot answer "how fast towards MY location" -- see ellipse.go.
#
# FMC, SFC, CSI and RSO are the crown-fire threshold's chain. They are carried
# separately rather than folded into CFB because that is what lets a failure
# localise to eq. 56 or eq. 57 rather than to "CFB is wrong". FMC and SFC were
# also, for a time, inputs the Go side read rather than quantities it computed;
# both are now ported (foliar.go, consumption.go) and both columns are assertions.
#
# Note what fbp() does to FMC before returning it: it forces 0 for D1, S1, S2, S3,
# O1A and O1B, the fuels with no crown. That is the DRIVER's decision, not
# foliar_moisture_content()'s, so those rows say nothing about eqs. 1-8 and the Go
# side skips them by name.
needed <- c("ISI", "BE", "SF", "WSV", "CFB", "FD", "ROS", "LB", "BROS", "FROS",
            "FMC", "SFC", "CSI", "RSO", "CFC", "TFC")
missing <- setdiff(needed, names(out))
if (length(missing)) {
  stop("cffdrs ", packageVersion("cffdrs"), " did not return: ",
       paste(missing, collapse = ", "),
       " -- the output contract changed; update this script and the Go test together.")
}

# --- alignment self-check --------------------------------------------------
# BE depends on nothing but fuel type and BUI. If the join above is wrong this is
# violated immediately and loudly, which is the cheapest possible guard against
# shipping a plausible-looking but mispaired fixture.
key <- paste(inp$FuelType, inp$BUI)
inconsistent <- sum(vapply(split(round(out$BE, 12), key),
                           function(v) length(unique(v)) > 1L, logical(1)))
if (inconsistent > 0) {
  stop(inconsistent, " of ", length(unique(key)), " (fuel, BUI) groups disagree on BE",
       " -- output is not aligned with input")
}
cat("alignment check: BE is a function of (fuel, BUI) across all ",
    length(unique(key)), " groups
", sep = "")

# --- emit ------------------------------------------------------------------
# Hand-rolled JSON: jsonlite is not a base package and this fixture must be
# regenerable from a bare R install, same principle as the stdlib-only Python
# generators.
num <- function(x) ifelse(is.finite(x), format(x, digits = 17, scientific = FALSE, trim = TRUE), "null")
q <- function(s) paste0('"', s, '"')

case_json <- function(i) {
  paste0(
    '  {"fuel": ', q(inp$FuelType[i]),
    ', "ffmc": ', num(inp$FFMC[i]),
    ', "bui": ', num(inp$BUI[i]),
    ', "ws": ', num(inp$WS[i]),
    ', "wd": ', num(inp$WD[i]),
    ', "gs": ', num(inp$GS[i]),
    ', "pc": ', num(inp$PC[i]),
    ', "pdf": ', num(inp$PDF[i]),
    ', "cc": ', num(inp$cc[i]),
    ', "cbh": ', num(inp$CBH[i]),
    ', "cfl": ', num(inp$CFL[i]),
    ', "lat": ', num(inp$LAT[i]),
    ', "dj": ', num(inp$Dj[i]),
    # LONG/ELV/D0 are inputs, carried for the same reason LAT and Dj are: they
    # are the rest of what FMC is a function of. They were constants in this
    # script until the FMC block above gave them something to say, which is why
    # a fixture older than that block has no such column.
    ', "long": ', num(inp$LONG[i]),
    ', "elv": ', num(inp$ELV[i]),
    ', "d0": ', num(inp$D0[i]),
    ', "isi": ', num(out$ISI[i]),
    ', "be": ', num(out$BE[i]),
    ', "sf": ', num(out$SF[i]),
    ', "wsv": ', num(out$WSV[i]),
    ', "fmc": ', num(out$FMC[i]),
    ', "sfc": ', num(out$SFC[i]),
    ', "csi": ', num(out$CSI[i]),
    ', "rso": ', num(out$RSO[i]),
    ', "cfb": ', num(out$CFB[i]),
    # CFC and TFC are eqs. 66a/66b/66c and 67. Both are fbp() OUTPUTS, unlike sfc
    # which spent a while here as an input the Go side read -- they were simply
    # not emitted until consumption.go grew the two functions that compute them.
    # Note what CFC embeds on a row that sent the CBH/CFL sentinel: fbp()
    # substituted its own table CFL before multiplying, and this fixture records
    # the -1 we sent, so those rows cannot say what CFL produced the number. The
    # Go side restricts to cfl in (0, 2] for anything that reads CFL.
    ', "cfc": ', num(out$CFC[i]),
    ', "tfc": ', num(out$TFC[i]),
    ', "fd": ', q(as.character(out$FD[i])),
    ', "ros": ', num(out$ROS[i]),
    ', "lb": ', num(out$LB[i]),
    ', "bros": ', num(out$BROS[i]),
    ', "fros": ', num(out$FROS[i]), '}'
  )
}

body <- paste(vapply(seq_len(nrow(inp)), case_json, character(1)), collapse = ",\n")

# === The FWI System ========================================================
#
# Everything above is FBP and lands in "cases". Everything below is the daily
# Fire Weather Index System, which package fwi (fwi/) ports, and lands in a
# SEPARATE top-level array, "fwi_cases". Separate on purpose, three ways over:
#
#   - The "cases" rows above come out byte-identical to a fixture generated
#     before this block existed, so tools/fixture-diff over the FBP section can
#     say "no column moved" and mean it.
#   - No row down here carries a "fuel" key. precheck counts FBP cases by that
#     substring, and the documented 24,260 is an FBP number.
#   - The two systems share no input: FBP takes FFMC/BUI as given, and this is
#     where they come from.
#
# What is reached, and how. cffdrs 1.9.2 does not export the six component
# functions (fine_fuel_moisture_code, duff_moisture_code, drought_code,
# initial_spread_index, buildup_index, fire_weather_index), so they are called
# through ::: -- the same bodies fwi() calls, with no driver in between. That is
# the only way to reach inputs fwi() would refuse or rewrite: fwi() clamps RH to
# 99.9999 before any code sees it, so RH exactly 100 (and above) is reachable
# only here. fwi() itself is then run three more ways, because it is what the
# Go Step claims to match:
#
#   kind      what it asserts                              through
#   ffmc      FFMC, one step                               fine_fuel_moisture_code
#   dmc       DMC, one step, every lat.adjust band         duff_moisture_code
#   dc        DC, one step, every lat.adjust band          drought_code
#   isi       ISI, fbpMod = FALSE                          initial_spread_index
#   bui       BUI                                          buildup_index
#   fwi       FWI                                          fire_weather_index
#   day       one day, all seven outputs incl. DSR         fwi(batch = FALSE)
#   chain     30-120 day sequences, state carried          fwi(batch = TRUE)
#   test_fwi  cffdrs' own 48-day sample dataset            fwi(test_fwi)
#
# Input columns are named so they never collide with an output column of the
# same name: the ISI grid's FFMC is ffmc_in, not ffmc, because ffmc is what the
# FFMC rows OUTPUT. tools/fixture-diff keys on the input set, and a name doing
# both jobs would key some rows on an output.

# The rain thresholds, read off the pinned source rather than off a paper: FFMC
# applies rain only when prec > 0.5, DMC when prec > 1.5 and DC when prec > 2.8,
# each with <= on the no-rain side. Every block that takes rain puts a row ON
# the threshold and one just above it, which is where a < for <= would show.
FWI_COLS_IN <- c("kind", "chain", "day", "lat_adjust", "mon", "lat",
                 "temp", "rh", "ws", "prec",
                 "ffmc_yda", "dmc_yda", "dc_yda",
                 "ffmc_in", "dmc_in", "dc_in", "isi_in", "bui_in")
FWI_COLS_OUT <- c("ffmc", "dmc", "dc", "isi", "bui", "fwi", "dsr")
FWI_COLS <- c(FWI_COLS_IN, FWI_COLS_OUT)

fwi_rows <- list()
fadd <- function(df) {
  for (col in FWI_COLS) if (!col %in% names(df)) df[[col]] <- NA
  fwi_rows[[length(fwi_rows) + 1L]] <<- df[, FWI_COLS]
}
internal <- function(name) get(name, envir = asNamespace("cffdrs"))
grid <- function(...) expand.grid(..., KEEP.OUT.ATTRS = FALSE, stringsAsFactors = FALSE)

# --- FFMC ---
# Every branch of fine_fuel_moisture_code:
#   rain       prec 0.5 (not applied) against 0.51 (applied)
#   eq. 3b     wmo > 150 needs yesterday's FFMC below ~20.0 (wmo = 150 at 19.99);
#              0 and 10 reach it, 20 sits just under it
#   wmo cap    FFMC 0 is wmo = 250 before any rain, so any rain pushes it over
#   wetting    a wet yesterday under dry air: FFMC 97-101 at RH 90-100
#   drying     a moist yesterday under dry air
#   neither    ew <= wmo <= ed, the equilibrium band; the grid lands some rows
#              there and the Go side counts them
#   101 clamp  60 C at low RH drives ed negative, so drying overshoots below
#              zero moisture. Not weather, but it is the only way the clamp is
#              reached, and the clamp is in the code.
# The 0 clamp is NOT reachable and is not attempted: it needs moisture above
# 250, which needs ew above 250, which needs temp below about -1200 C -- where
# exp(0.0365 * temp) has already flattened the rate to nothing. The Go side
# says so rather than leaving the clamp to look covered.
g <- grid(ffmc_yda = c(0, 10, 20, 50, 70, 85, 92, 97, 101),
          temp = c(-10, 0, 15, 30, 60),
          rh = c(0, 5, 20, 45, 70, 90, 100),
          ws = c(0, 15, 100),
          prec = c(0, 0.3, 0.5, 0.51, 1, 5, 20, 80))
g$ffmc <- internal("fine_fuel_moisture_code")(g$ffmc_yda, g$temp, g$rh, g$ws, g$prec)
g$kind <- "ffmc"
fadd(g)

# --- DMC ---
# (a) Weather and rain, at one latitude and month (55 N in July, the Swedish
# consumer's own band). Branches:
#   rain       prec 1.5 (not applied) against 1.51 (applied)
#   eq. 13     b's three pieces: yesterday <= 33, <= 65, > 65, with 33 and 65
#              themselves on the grid
#   temp floor -5 is clamped to -1.1, and -1.1 itself is on the grid
#   pr clamp   heavy rain on a DMC near 0: wmr - 20 exceeds exp(5.6348) and the
#              log goes past 5.6348, so pr comes out negative (50 and 150 mm)
#   RH 100     rk = 0 exactly -- no drying at all
# (b) The final dmc1 < 0 clamp needs rk < 0, which needs RH above 100.
# fwi() never sends one (it clamps RH first), but the component takes RH as
# given, and RH derived from dewpoint does exceed 100 in real data.
g <- grid(dmc_yda = c(0, 5, 20, 33, 40, 65, 90, 200),
          temp = c(-5, -1.1, 0, 20, 35),
          rh = c(0, 40, 100),
          prec = c(0, 1.5, 1.51, 3, 10, 50, 150))
g <- rbind(g, grid(dmc_yda = c(0, 0.5, 5), temp = 20, rh = c(100.5, 120), prec = 0))
g$mon <- 7
g$lat <- 55
g$lat_adjust <- TRUE
g$dmc <- internal("duff_moisture_code")(g$dmc_yda, g$temp, g$rh, g$prec, g$lat, g$mon, TRUE)
g$kind <- "dmc"
fadd(g)

# (c) The day-length factor, every month in every band, both sides of every
# boundary. duff_moisture_code's bands (lat.adjust = TRUE):
#   lat > 30          ell01, the Canadian 46 N table -- Sweden is here
#   10 < lat <= 30    ell02
#   -10 < lat <= 10   9 for every month
#   -30 < lat <= -10  ell03
#   -90 <= lat <= -30 ell04
#   lat < -90         ell01 again: none of the four ifelse arms matches, so it
#                     falls through to the northern default. Invalid input, but
#                     it is what the code does, and -95 pins it.
# Upstream's own comments say "latitude >= 30N" for ell01; the code says > 30.
# 30 is on the grid, so the oracle -- not the comment -- decides.
# lat.adjust = FALSE uses ell01 everywhere, which is the lat > 30 band; the Go
# side has no flag and asserts these rows at latitude 46 instead.
DMC_LATS <- c(-95, -90, -60, -30.5, -30, -29.5, -10.5, -10, -9.5, 0,
              9.5, 10, 10.5, 29.5, 30, 30.5, 46, 55, 62, 69, 90)
for (adj in c(TRUE, FALSE)) {
  g <- grid(lat = DMC_LATS, mon = 1:12)
  g$dmc_yda <- 20
  g$temp <- 20
  g$rh <- 40
  g$prec <- 0
  g$lat_adjust <- adj
  g$dmc <- internal("duff_moisture_code")(g$dmc_yda, g$temp, g$rh, g$prec, g$lat, g$mon, adj)
  g$kind <- "dmc"
  fadd(g)
}

# --- DC ---
# (a) Weather and rain. Branches:
#   rain       prec 2.8 (not applied) against 2.81 (applied)
#   temp floor -10 is clamped to -2.8, which is itself on the grid
#   pe clamp   January's -1.6 day-length factor at temp 0 or 1 makes pe negative
#   dr0 clamp  heavy rain on a low DC: 100 mm on DC 0 or 15
#   RH         DC does not read RH at all. Both extremes are here so a Go port
#              that used it would be caught rather than agreeing by accident.
# The final dc1 < 0 clamp is not reachable from a non-negative yesterday: dr and
# pe are both clamped at zero first.
g <- grid(dc_yda = c(0, 15, 100, 300, 600, 1000),
          temp = c(-10, -2.8, 0, 1, 20, 35),
          rh = c(0, 100),
          prec = c(0, 2.8, 2.81, 5, 20, 100),
          mon = c(1, 7))
g$lat <- 55
g$lat_adjust <- TRUE
g$dc <- internal("drought_code")(g$dc_yda, g$temp, g$rh, g$prec, g$lat, g$mon, TRUE)
g$kind <- "dc"
fadd(g)

# (b) Day length. drought_code's bands (lat.adjust = TRUE):
#   lat > 20          fl01 -- Sweden is here
#   -20 < lat <= 20   1.4 for every month
#   lat <= -20        fl02 (no lower bound, unlike DMC's ell04)
# Note the DC and DMC bands do NOT share boundaries: 15 N is DMC's ell02 band and
# DC's equatorial one. The chains below include a station there.
DC_LATS <- c(-95, -90, -45, -20.5, -20, -19.5, 0, 19.5, 20, 20.5, 46, 55, 62, 69, 90)
for (adj in c(TRUE, FALSE)) {
  g <- grid(lat = DC_LATS, mon = 1:12)
  g$dc_yda <- 100
  g$temp <- 20
  g$rh <- 40
  g$prec <- 0
  g$lat_adjust <- adj
  g$dc <- internal("drought_code")(g$dc_yda, g$temp, g$rh, g$prec, g$lat, g$mon, adj)
  g$kind <- "dc"
  fadd(g)
}

# --- ISI, BUI, FWI ---
# ISI with fbpMod = FALSE, which is what fwi() passes. The wind grid runs well
# past 40 km/h on purpose: that is where FBP's high-wind function would take
# over, and it must NOT here. FFMC runs the full 0-101 range, 0 included --
# FBP's ISI refuses FFMC 0, the FWI System's does not.
g <- grid(ffmc_in = c(0, 10, 30, 50, 70, 80, 85, 88, 90, 92, 94, 96, 98, 99, 100, 101),
          ws = c(0, 5, 10, 20, 30, 39.9, 40, 50, 70, 100, 150))
g$isi <- internal("initial_spread_index")(g$ffmc_in, g$ws, FALSE)
g$kind <- "isi"
fadd(g)

# BUI branches: dmc = dc = 0 (the 0/0 guard); dmc = 0 with dc > 0; dc = 0 with
# dmc > 0; bui1 >= dmc (dmc <= 0.4 dc) against bui1 < dmc; and bui0 < 0, which a
# small DMC with little DC reaches (dmc 0.5, 0.9 against cc near 0.92).
g <- grid(dmc_in = c(0, 0.5, 0.9, 1, 5, 10, 20, 40, 80, 150, 300),
          dc_in = c(0, 1, 10, 50, 100, 250, 500, 800, 1200))
g$bui <- internal("buildup_index")(g$dmc_in, g$dc_in)
g$kind <- "bui"
fadd(g)

# FWI branches: bui <= 80 against > 80 (79.9, 80, 80.1 all present) and the
# bb <= 1 identity against the log-power form, with isi = 0 at the bottom.
g <- grid(isi_in = c(0, 0.5, 1, 2, 5, 10, 20, 50, 100),
          bui_in = c(0, 1, 10, 40, 79.9, 80, 80.1, 120, 200, 400))
g$fwi <- internal("fire_weather_index")(g$isi_in, g$bui_in)
g$kind <- "fwi"
fadd(g)

# --- fwi() one day ---
# batch = FALSE makes every row its own station for one day, each with its own
# yesterday, so this is fwi()'s per-day step -- RH clamp, all six codes and DSR
# -- over a cross of states, weather, latitudes and months. The weather set is
# chosen for the thresholds and extremes, not for realism:
FWI_DAY_WEATHER <- data.frame(
  temp = c(20, 30, 35, 10, 12, 25, 5, 8, 15, 15, 15, 15, 18, 14, -5, -15, 45),
  rh = c(40, 15, 0, 100, 100.5, 20, 95, 90, 80, 80, 80, 80, 70, 85, 60, 70, 5),
  ws = c(15, 25, 0, 5, 5, 150, 10, 10, 10, 10, 10, 10, 20, 30, 10, 5, 40),
  prec = c(0, 0, 0, 0, 0.2, 0, 0.5, 0.51, 1.5, 1.51, 2.8, 2.81, 10, 45, 0, 3, 0)
)
FWI_DAY_STATES <- data.frame(
  ffmc_yda = c(85, 95, 40, 101, 70),
  dmc_yda = c(6, 60, 1, 0, 30),
  dc_yda = c(15, 400, 5, 0, 150)
)
run_days <- function(lats, mons, weather, states, adj) {
  g <- merge(merge(merge(weather, states), data.frame(lat = lats)), data.frame(mon = mons))
  input <- data.frame(long = -100, lat = g$lat, yr = 2000, mon = g$mon, day = 1,
                      temp = g$temp, rh = g$rh, ws = g$ws, prec = g$prec)
  init <- data.frame(ffmc = g$ffmc_yda, dmc = g$dmc_yda, dc = g$dc_yda)
  o <- fwi(input, init = init, batch = FALSE, out = "fwi", lat.adjust = adj)
  if (nrow(o) != nrow(g)) stop("fwi() returned ", nrow(o), " rows for ", nrow(g))
  g$ffmc <- o$FFMC; g$dmc <- o$DMC; g$dc <- o$DC
  g$isi <- o$ISI; g$bui <- o$BUI; g$fwi <- o$FWI; g$dsr <- o$DSR
  g$lat_adjust <- adj
  g$kind <- "day"
  fadd(g)
}
run_days(c(-35, -15, 0, 15, 25, 46, 55, 62, 69), c(1, 4, 7, 10),
         FWI_DAY_WEATHER, FWI_DAY_STATES, TRUE)
# lat.adjust = FALSE through the driver too, every month, at latitudes where it
# differs from TRUE.
run_days(c(-35, -15, 0, 15, 62), 1:12,
         FWI_DAY_WEATHER[c(1, 2, 13), ], FWI_DAY_STATES[1:2, ], FALSE)

# --- fwi() chains ---
# Single steps cannot see state propagation: a Go Step that returned the right
# day but carried the wrong thing into tomorrow passes every row above. These
# are whole sequences through fwi() itself, and the Go side chains its own state
# from the first day's yesterday -- it never reads the oracle's intermediate
# state.
#
# The weather is pseudo-random with a fixed seed and a fixed RNG kind, so it
# regenerates identically at the pinned R. It is rounded to one decimal so the
# fixture reads like weather, and it is recorded into the fixture, so the Go
# side needs no RNG. Rain comes in spells (a two-state Markov chain) so dry
# spells build the codes up and a wet spell knocks them down, and each chain can
# force a dry spell and a storm to make sure that actually happens.
set.seed(20260924, kind = "Mersenne-Twister", normal.kind = "Inversion",
         sample.kind = "Rejection")
FWI_CHAINS <- list(
  # Sweden, both ends of the consumer's 55-69 N range, through a season.
  list(id = "se-south", lat = 55.7, start = "2025-04-15", days = 120,
       init = c(85, 6, 15), tmean = 13, tamp = 7, wet = 0.30),
  list(id = "se-north", lat = 67.9, start = "2025-05-20", days = 110,
       init = c(85, 6, 15), tmean = 9, tamp = 6, wet = 0.35),
  list(id = "se-mid-wet-start", lat = 62.4, start = "2025-06-01", days = 60,
       init = c(60, 3, 20), tmean = 12, tamp = 5, wet = 0.30),
  # A drought: forty rainless days then a storm, at the Canadian reference
  # latitude.
  list(id = "drought-49n", lat = 49, start = "2025-06-01", days = 100,
       init = c(85, 6, 15), tmean = 22, tamp = 6, wet = 0.20,
       dry = 10:50, storm = c(51, 60)),
  # Autumn into winter: temperatures fall through both floors (-1.1, -2.8).
  list(id = "autumn-freeze-60n", lat = 60, start = "2025-10-01", days = 75,
       init = c(80, 20, 250), tmean = -6, tamp = 10, wet = 0.35),
  # One station in each remaining band, DMC's and DC's.
  list(id = "15n", lat = 15, start = "2025-03-01", days = 40,
       init = c(85, 6, 15), tmean = 28, tamp = 3, wet = 0.15),
  list(id = "22n", lat = 22, start = "2025-05-01", days = 40,
       init = c(85, 6, 15), tmean = 27, tamp = 3, wet = 0.20),
  list(id = "equator", lat = 3, start = "2025-01-10", days = 40,
       init = c(85, 6, 15), tmean = 27, tamp = 1, wet = 0.40),
  list(id = "25s", lat = -25, start = "2025-11-15", days = 70,
       init = c(85, 6, 15), tmean = 24, tamp = 4, wet = 0.20),
  list(id = "41s", lat = -41, start = "2026-01-01", days = 60,
       init = c(85, 6, 15), tmean = 16, tamp = 5, wet = 0.30)
)
for (ch in FWI_CHAINS) {
  dates <- seq(as.Date(ch$start), by = "day", length.out = ch$days)
  doy <- as.integer(format(dates, "%j"))
  # Seasonal swing, flipped for the southern hemisphere.
  season <- sin(2 * pi * (doy - 110) / 365) * sign(ch$lat + 1e-9)
  temp <- round(ch$tmean + ch$tamp * season + rnorm(ch$days, 0, 3), 1)
  rh <- round(pmin(100, pmax(5, 60 - 1.5 * (temp - ch$tmean) + rnorm(ch$days, 0, 14))), 1)
  ws <- round(pmax(0, rnorm(ch$days, 12, 8)), 1)
  wet <- logical(ch$days)
  for (d in seq_len(ch$days)) {
    p <- if (d > 1 && wet[d - 1]) 0.6 else ch$wet * 0.6
    wet[d] <- runif(1) < p
  }
  prec <- ifelse(wet, round(rexp(ch$days, 1 / 6), 1), 0)
  if (!is.null(ch$dry)) prec[ch$dry] <- 0
  if (!is.null(ch$storm)) prec[ch$storm] <- c(62, 35)
  input <- data.frame(long = -100, lat = ch$lat,
                      yr = as.integer(format(dates, "%Y")),
                      mon = as.integer(format(dates, "%m")),
                      day = as.integer(format(dates, "%d")),
                      temp = temp, rh = rh, ws = ws, prec = prec)
  init <- data.frame(ffmc = ch$init[1], dmc = ch$init[2], dc = ch$init[3], lat = ch$lat)
  o <- fwi(input, init = init, batch = TRUE, out = "fwi")
  if (nrow(o) != ch$days) stop("chain ", ch$id, ": fwi() returned ", nrow(o), " rows")
  g <- data.frame(kind = "chain", chain = ch$id, day = seq_len(ch$days),
                  lat = ch$lat, mon = input$mon, temp = temp, rh = rh, ws = ws, prec = prec,
                  ffmc = o$FFMC, dmc = o$DMC, dc = o$DC, isi = o$ISI,
                  bui = o$BUI, fwi = o$FWI, dsr = o$DSR, stringsAsFactors = FALSE)
  # Yesterday is recorded on day 1 only: it is the chain's start, and every
  # later day's yesterday is the Go side's own previous output.
  g$ffmc_yda <- c(ch$init[1], rep(NA, ch$days - 1))
  g$dmc_yda <- c(ch$init[2], rep(NA, ch$days - 1))
  g$dc_yda <- c(ch$init[3], rep(NA, ch$days - 1))
  g$lat_adjust <- TRUE
  fadd(g)
}

# --- test_fwi ---
# cffdrs' own sample dataset, through fwi() with its own default start-up codes
# (85, 6, 15). Independent of every choice above: real weather, chosen by
# upstream, not by this script.
data("test_fwi", package = "cffdrs", envir = environment())
o <- fwi(test_fwi, out = "fwi")
if (nrow(o) != nrow(test_fwi)) stop("fwi(test_fwi) returned ", nrow(o), " rows")
n_tf <- nrow(test_fwi)
fadd(data.frame(kind = "test_fwi", chain = "test_fwi", day = seq_len(n_tf),
                lat = test_fwi$lat, mon = test_fwi$mon, temp = test_fwi$temp,
                rh = test_fwi$rh, ws = test_fwi$ws, prec = test_fwi$prec,
                ffmc_yda = c(85, rep(NA, n_tf - 1)),
                dmc_yda = c(6, rep(NA, n_tf - 1)),
                dc_yda = c(15, rep(NA, n_tf - 1)),
                lat_adjust = TRUE,
                ffmc = o$FFMC, dmc = o$DMC, dc = o$DC, isi = o$ISI,
                bui = o$BUI, fwi = o$FWI, dsr = o$DSR, stringsAsFactors = FALSE))

FW <- do.call(rbind, fwi_rows)
# Every row must have produced the output its kind is about. A non-finite one is
# not skipped -- it is emitted as null and the Go side fails on it -- but it is
# worth stopping here first, where the inputs are still in hand.
for (col in FWI_COLS_OUT) {
  v <- FW[[col]]
  bad <- !is.na(v) & !is.finite(v)
  if (any(bad)) stop(sum(bad), " non-finite ", col, " values in the FWI block")
}

fwi_value <- function(v) {
  if (is.character(v)) q(v)
  else if (is.logical(v)) tolower(as.character(v))
  else num(v)
}
fwi_json <- function(i) {
  parts <- character(0)
  for (col in FWI_COLS) {
    v <- FW[[col]][i]
    # A column a row's kind does not use is NA and is left out; NaN is not NA
    # here, and is emitted (as null) rather than hidden.
    if (is.na(v) && !is.nan(v)) next
    parts <- c(parts, paste0('"', col, '": ', fwi_value(v)))
  }
  paste0('  {', paste(parts, collapse = ", "), '}')
}
fwi_body <- paste(vapply(seq_len(nrow(FW)), fwi_json, character(1)), collapse = ",\n")

json <- paste0(
  '{\n',
  ' "note": "generated by testdata/gen_cffdrs_reference.R; do not edit by hand",\n',
  ' "oracle": "cffdrs R package (Canadian Forest Service), the authoritative FBP implementation",\n',
  ' "cffdrs_version": ', q(as.character(packageVersion("cffdrs"))), ',\n',
  ' "r_version": ', q(R.version.string), ',\n',
  ' "cases": [\n', body, '\n ],\n',
  ' "fwi_cases": [\n', fwi_body, '\n ]\n}\n'
)

dir.create(dirname(OUT), recursive = TRUE, showWarnings = FALSE)
writeLines(json, OUT, useBytes = TRUE)
cat(sprintf("wrote %d cases -> %s\n", nrow(inp), OUT))
cat(sprintf("  flat/surface rows usable for ROS parity: %d\n",
            sum(inp$GS == 0 & inp$FuelType != "C6")))
cat(sprintf("  rows usable for the crown threshold (explicit CBH/CFL): %d, of which %d crown\n",
            sum(inp$CBH > 0 & inp$CFL > 0),
            sum(inp$CBH > 0 & inp$CFL > 0 & out$CFB > 0)))
cat(sprintf("  fire descriptions: S %d, I %d, C %d\n",
            sum(out$FD == "S"), sum(out$FD == "I"), sum(out$FD == "C")))
cat(sprintf("wrote %d FWI System cases (fwi_cases):\n", nrow(FW)))
for (k in unique(FW$kind)) cat(sprintf("  %-9s %d\n", k, sum(FW$kind == k)))
