#!/usr/bin/env bash
# Session setup for gofbp. Runs before Claude Code launches, on a fresh
# environment, and its one real job is to make the oracle available.
#
# Without testdata/cffdrs.json the twelve TestCFFDRS* tests skip, and a session
# that cannot run them cannot say whether a coefficient is right -- only whether
# it is self-consistent. Both /migration-check and /migration-port treat that as
# a precondition and refuse to port through it, so an environment without the
# fixture can read and audit but cannot do the work.
#
# Deliberately NOT `set -e`. A session that starts with no fixture is degraded;
# a session that fails to start is useless. Everything here reports and
# continues, and the summary at the end is the thing to read.
#
# Slow the first time -- an R toolchain plus GDAL is several minutes. If the
# environment caches its image between sessions, this is a one-off.

set -uo pipefail

R_PIN="${CFFDRS_R_VERSION:-4.6.1}"       # what the container pins; see testdata/Dockerfile
CFFDRS_PIN="${CFFDRS_VERSION:-1.9.2}"    # the version the recorded digest came from

cd "$(dirname "${BASH_SOURCE[0]}")/.." || exit 0
REPO="$PWD"
FIXTURE="$REPO/testdata/cffdrs.json"

say() { printf '[setup] %s\n' "$*"; }
warn() { printf '[setup] WARNING: %s\n' "$*" >&2; }

# --- 1. the toolchain -------------------------------------------------------

if command -v go >/dev/null 2>&1; then
	say "go: $(go version)"
else
	warn "go not found -- nothing in this repository can be built or tested"
fi

# --- 2. the fixture ---------------------------------------------------------
#
# Three paths, in order of how closely they reproduce the pinned oracle.

if [ -f "$FIXTURE" ]; then
	say "fixture already present, leaving it alone"

elif command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
	# The exact pinned path: R and cffdrs both fixed by testdata/Dockerfile.
	say "generating fixture via Docker (R $R_PIN, cffdrs $CFFDRS_PIN) -- several minutes"
	"$REPO/testdata/regen-cffdrs.sh" || warn "docker regeneration failed"

elif command -v apt-get >/dev/null 2>&1; then
	# No Docker, which is the normal case in a sandboxed session. Install R on
	# the host instead and use regen-cffdrs.sh --local.
	#
	# cffdrs Imports sf and terra, which link against GDAL/GEOS/PROJ. This
	# package uses none of that -- but R resolves a package's imports at load
	# time, so the libraries have to be here for library(cffdrs) to return at
	# all. Same reasoning as testdata/Dockerfile; see its comments.
	say "no Docker; installing R and the GDAL stack -- several minutes"
	export DEBIAN_FRONTEND=noninteractive
	sudo apt-get update -qq
	sudo apt-get install -y -qq --no-install-recommends \
		r-base-core r-base-dev \
		libgdal-dev libgeos-dev libproj-dev libudunits2-dev \
		libcurl4-openssl-dev libssl-dev libxml2-dev \
		|| warn "apt install failed -- the fixture will be missing"

	if command -v Rscript >/dev/null 2>&1; then
		# Pin cffdrs to the version the recorded digest came from. The R version
		# is whatever apt has, which is the compromise this path makes; see the
		# digest note in the summary below.
		say "installing cffdrs $CFFDRS_PIN"
		Rscript -e "install.packages('remotes', repos='https://cloud.r-project.org', quiet=TRUE)" \
			-e "remotes::install_version('cffdrs', version='${CFFDRS_PIN}', repos='https://cloud.r-project.org', upgrade='never', quiet=TRUE)" \
			-e "stopifnot(packageVersion('cffdrs') == '${CFFDRS_PIN}')" \
			|| warn "cffdrs install failed"

		"$REPO/testdata/regen-cffdrs.sh" --local || warn "local regeneration failed"
	fi

else
	warn "no fixture, no Docker, no apt -- the TestCFFDRS* tests will skip"
fi

# --- 3. say plainly what this session can and cannot claim ------------------

echo
say "--------------------------------------------------------------"

if [ -f "$FIXTURE" ]; then
	got="$(sha256sum "$FIXTURE" 2>/dev/null | cut -d' ' -f1)"
	want="$(grep -oE '^sha256  [0-9a-f]{64}$' "$REPO/testdata/README.md" 2>/dev/null | head -1 | awk '{print $2}')"
	say "fixture: $(wc -c <"$FIXTURE" | tr -d ' ') bytes"
	say "  sha256   $got"
	if [ -n "$want" ] && [ "$got" = "$want" ]; then
		say "  matches the digest recorded in testdata/README.md"
	elif [ -n "$want" ]; then
		warn "digest differs from testdata/README.md ($want)."
		warn "  Expected if R here is not $R_PIN -- the R version is recorded INTO"
		warn "  the fixture. NOT expected if cffdrs is $CFFDRS_PIN and R matches:"
		warn "  then the reference numbers moved, and that is a finding, not noise."
	fi
else
	warn "no fixture. The TestCFFDRS* tests will skip, so this session can read"
	warn "  and audit the ledger but must NOT port a coefficient -- see"
	warn "  DAILY-CHECK.md. Both migration commands check this before starting."
fi

if command -v go >/dev/null 2>&1; then
	# Count the total rather than hardcoding it: the number of oracle tests grows
	# with every ported row, and a stale count here would understate the gap.
	run="$(cd "$REPO" && go test . -run TestCFFDRS -v 2>/dev/null)"
	skipped="$(printf '%s\n' "$run" | grep -c '^--- SKIP')"
	total="$(printf '%s\n' "$run" | grep -c '^=== RUN   TestCFFDRS')"
	say "TestCFFDRS* skipping: ${skipped:-unknown} of ${total:-unknown}"
fi

say "--------------------------------------------------------------"
exit 0
