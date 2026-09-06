# Emit the FBP reference fixture from the cffdrs R package.
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
                     cbh = -1, cfl = -1, lat = 60, dj = 200) {
  data.frame(
    FuelType = fuel, LAT = lat, LONG = 15, ELV = 0, Dj = dj, D0 = 0,
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
# rather than recomputed on the Go side because FMC and SFC are deliberately NOT
# implemented there -- the Go package takes them as caller-supplied inputs, so the
# fixture is where they have to come from. CSI and RSO are the intermediate steps
# the Go side does implement, and having them separately is what lets a failure
# localise to eq. 56 or eq. 57 rather than to "CFB is wrong".
needed <- c("ISI", "BE", "SF", "WSV", "CFB", "FD", "ROS", "LB", "BROS", "FROS",
            "FMC", "SFC", "CSI", "RSO")
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
    ', "isi": ', num(out$ISI[i]),
    ', "be": ', num(out$BE[i]),
    ', "sf": ', num(out$SF[i]),
    ', "wsv": ', num(out$WSV[i]),
    ', "fmc": ', num(out$FMC[i]),
    ', "sfc": ', num(out$SFC[i]),
    ', "csi": ', num(out$CSI[i]),
    ', "rso": ', num(out$RSO[i]),
    ', "cfb": ', num(out$CFB[i]),
    ', "fd": ', q(as.character(out$FD[i])),
    ', "ros": ', num(out$ROS[i]),
    ', "lb": ', num(out$LB[i]),
    ', "bros": ', num(out$BROS[i]),
    ', "fros": ', num(out$FROS[i]), '}'
  )
}

body <- paste(vapply(seq_len(nrow(inp)), case_json, character(1)), collapse = ",\n")
json <- paste0(
  '{\n',
  ' "note": "generated by testdata/gen_cffdrs_reference.R; do not edit by hand",\n',
  ' "oracle": "cffdrs R package (Canadian Forest Service), the authoritative FBP implementation",\n',
  ' "cffdrs_version": ', q(as.character(packageVersion("cffdrs"))), ',\n',
  ' "r_version": ', q(R.version.string), ',\n',
  ' "cases": [\n', body, '\n ]\n}\n'
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
