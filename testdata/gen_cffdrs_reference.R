# Emit the FBP reference fixture from the cffdrs R package.
#
#   backend/internal/fbp/testdata/regen-cffdrs.sh
#
# That wrapper pins R and cffdrs in a container, so regenerating needs Docker and
# nothing else. To drive this file directly instead, run it from the REPO ROOT --
# the output path below is relative to it:
#
#   Rscript backend/internal/fbp/testdata/gen_cffdrs_reference.R
#
# Writes backend/internal/fbp/testdata/cffdrs.json: a grid of FBP inputs and what
# the *authoritative* implementation produces for each. Go's TestCFFDRS* assert
# internal/fbp reproduces them.
#
# Why this exists: every coefficient in internal/fbp was typed in by hand from
# ST-X-3, and a transcription error looks exactly like correct code. cffdrs is
# maintained by the Canadian Forest Service authors of the system itself, so it
# is the only oracle in this repo that can say the tables are actually right --
# and since the Python mirror was removed it is the only cross-implementation
# check the package has at all. One such error was found this way (the grass
# curing branch, 2026-08-18, 2.2x too fast at full curing).
#
# R is needed to REGENERATE the fixture, never to run the tests -- the JSON is
# committed and the Go side reads it with no R involved. Re-run this after
# touching a coefficient, and on a cffdrs upgrade.
#
# Requires: R, and install.packages("cffdrs") -- or just Docker, via
# regen-cffdrs.sh. cffdrs Imports sf and terra, so a local install also needs
# GDAL, GEOS and PROJ; the container is usually the shorter path.

suppressMessages(library(cffdrs))

OUT <- file.path("backend", "internal", "fbp", "testdata", "cffdrs.json")

# --- the sweeps ------------------------------------------------------------
#
# ISI is not an fbp() input: it is derived from FFMC and wind. Sweeping both is
# how this reaches a wide ISI range, and on FLAT ground that costs nothing --
# cffdrs' WSV reduces to WS and ROS is still RSI(ISI) x BE, exactly the quantity
# internal/fbp computes. Slope is what pulls the two apart (see below), so it is
# swept separately.
FFMC_VALUES <- c(60, 75, 85, 90, 92, 95)
WS_VALUES <- c(0, 5, 15, 30, 50)
BUI_VALUES <- c(1, 20, 40, 64, 100, 150)
# Slope in PERCENT rise, bracketing the 70% saturation from both sides.
GS_VALUES <- c(0, 5, 15, 30, 45, 60, 69.9, 70, 100, 200)
PC_VALUES <- c(0, 25, 50, 75, 100)   # mixedwood conifer share
PDF_VALUES <- c(0, 35, 100)          # M2 dead fir share
CC_VALUES <- c(20, 50, 80, 100)      # grass curing

CONIFER <- c("C1", "C2", "C3", "C4", "C5", "C6", "C7")
SIMPLE <- c(CONIFER, "D1", "S1", "S2", "S3")
MIXED <- c("M1", "M2")
GRASS <- c("O1a", "O1b")

# fbp() wants every column present for every row, so unused ones get harmless
# in-range defaults: they do not enter the fuels that ignore them.
base_row <- function(fuel, ffmc, bui, ws, gs, pc = 50, pdf = 35, cc = 80) {
  data.frame(
    FuelType = fuel, LAT = 60, LONG = 15, ELV = 0, Dj = 200, D0 = 0,
    FFMC = ffmc, BUI = bui, WS = ws, WD = 0, GS = gs, Aspect = 0,
    PC = pc, PDF = pdf, cc = cc, GFL = 0.35, CBH = -1, CFL = -1,
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
for (fuel in MIXED) {
  for (ffmc in FFMC_VALUES) for (ws in WS_VALUES) for (bui in BUI_VALUES) {
    for (pc in PC_VALUES) for (pdf in if (fuel == "M2") PDF_VALUES else 35) {
      add(base_row(fuel, ffmc, bui, ws, 0, pc = pc, pdf = pdf))
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
# alignment the per-pixel overlay expresses through that back-solve
# (docs/analysis/spread-severity-plan.md section 5).
#
# WS/FFMC/BUI values are drawn from the flat sweeps above so every sloped row has
# a flat counterpart to take its wind-only ISI from.
#
# The fuel list includes M1/M2 to cover eq. 42's mixedwood ISF blend. Go's
# slopeEquivalentISF blends the ISF of C2 and D1 rather than blending RSF and
# inverting once, and drops M2's 0.2 dead-fir weighting in the slope path -- both
# read off cffdrs' source rather than measured, and fuelmap.FuelType emits M1 in
# production. Without these rows TestCFFDRSSlopeBackSolve logs that the blend is
# unoracled and moves on.
#
# FFMC is swept rather than fixed at 85 because 85 is too wet to reach the
# interesting branches: the largest equivalent wind anywhere in an 85-only sweep
# is 38.7 km/h, just under fbp.HighWindKmh. FFMC 95 crosses it, and also reaches
# both fbp.EquivalentWindCapKmh and the isfClampMin guard -- three branches that
# are otherwise transcribed but never checked against the oracle.
for (fuel in c("C2", "C3", "D1", "S1", "O1b", "M1", "M2")) {
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
# alone cannot answer "how fast towards MY parcel" -- see fbp/ellipse.go.
needed <- c("ISI", "BE", "SF", "WSV", "CFB", "FD", "ROS", "LB", "BROS", "FROS")
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
    ', "isi": ', num(out$ISI[i]),
    ', "be": ', num(out$BE[i]),
    ', "sf": ', num(out$SF[i]),
    ', "wsv": ', num(out$WSV[i]),
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
  ' "note": "generated by backend/internal/fbp/testdata/gen_cffdrs_reference.R; do not edit by hand",\n',
  ' "oracle": "cffdrs R package (Canadian Forest Service), the authoritative FBP implementation",\n',
  ' "cffdrs_version": ', q(as.character(packageVersion("cffdrs"))), ',\n',
  ' "r_version": ', q(R.version.string), ',\n',
  ' "cases": [\n', body, '\n ]\n}\n'
)

dir.create(dirname(OUT), recursive = TRUE, showWarnings = FALSE)
writeLines(json, OUT, useBytes = TRUE)
cat(sprintf("wrote %d cases -> %s\n", nrow(inp), OUT))
cat(sprintf("  flat/surface rows usable for ROS parity: %d\n",
            sum(inp$GS == 0 & out$CFB == 0)))
