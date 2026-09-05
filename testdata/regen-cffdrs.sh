#!/usr/bin/env bash
# Regenerate testdata/cffdrs.json, the FBP reference fixture that every Go
# TestCFFDRS* asserts against.
#
# The fixture is NOT committed -- see testdata/README.md -- so those tests skip
# until you generate it. Generate it once and they read it with no R involved.
# Re-run this when you change a coefficient in this package, when you widen a
# sweep in gen_cffdrs_reference.R, or on a cffdrs upgrade -- then read the diff,
# because a changed reference number is the oracle telling you something, not
# noise to be committed past.
#
# R comes from PATH if it has cffdrs, otherwise from Docker: the Dockerfile beside
# this script pins R 4.6.1 and cffdrs 1.9.2, so a regeneration needs Docker and
# nothing else. Both versions land in the fixture's own metadata and are logged by
# the Go tests, so a version drift shows up in the diff rather than silently
# moving several thousand numbers.
#
# Runs anywhere with bash: Linux, macOS, or Windows via Git Bash.
#
# Usage:
#   regen-cffdrs.sh [options]
#
#   --docker            force the container even if a usable local R exists
#   --local             force local Rscript; fail rather than falling back
#   --rebuild           rebuild the image even if it is already present
#   --r-version V       R version to pin (default 4.6.1, or CFFDRS_R_VERSION)
#   --cffdrs-version V  cffdrs version to pin (default 1.9.2, or CFFDRS_VERSION)
#   --dry-run           print what would run; touch nothing
#
# The first Docker run builds the image and takes a while -- an R toolchain plus
# GDAL. After that it is cached and a regeneration is seconds. The sweep itself is
# ~11500 cases through cffdrs::fbp() and is not the slow part.
set -euo pipefail

R_VERSION="${CFFDRS_R_VERSION:-4.6.1}"
CFFDRS_VERSION="${CFFDRS_VERSION:-1.9.2}"
MODE=auto
REBUILD=0
DRY_RUN=0

die() { printf 'error: %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
	case "$1" in
	--docker) MODE=docker ;;
	--local) MODE=local ;;
	--rebuild) REBUILD=1 ;;
	--dry-run) DRY_RUN=1 ;;
	--r-version) R_VERSION="${2:?--r-version needs a value}"; shift ;;
	--cffdrs-version) CFFDRS_VERSION="${2:?--cffdrs-version needs a value}"; shift ;;
	-h | --help) sed -n '2,32p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
	*) die "unknown option: $1 (try --help)" ;;
	esac
	shift
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# testdata -> module root.
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
FIXTURE="$SCRIPT_DIR/cffdrs.json"
IMAGE="gofbp-cffdrs:${R_VERSION}-${CFFDRS_VERSION}"

[ -f "$SCRIPT_DIR/gen_cffdrs_reference.R" ] ||
	die "cannot find the generator from $SCRIPT_DIR — is this still in testdata/?"

# Report the fixture's recorded provenance. The whole point of pinning is that a
# mismatch is visible, so print it rather than making someone grep the JSON.
fixture_versions() {
	[ -f "$FIXTURE" ] || { echo "  (no fixture yet)"; return; }
	sed -n 's/.*"cffdrs_version": *"\([^"]*\)".*/  cffdrs: \1/p;s/.*"r_version": *"\([^"]*\)".*/  R:      \1/p' "$FIXTURE" | head -2
	printf '  cases:  %s\n' "$(grep -o '"fuel"' "$FIXTURE" | wc -l | tr -d ' ')"
}

have_local_r() {
	command -v Rscript >/dev/null 2>&1 &&
		Rscript -e 'quit(status = !requireNamespace("cffdrs", quietly = TRUE))' >/dev/null 2>&1
}

echo "Fixture before:"
fixture_versions

if [ "$MODE" = auto ]; then
	if have_local_r; then MODE=local; else MODE=docker; fi
fi

case "$MODE" in
local)
	have_local_r || die "no local Rscript with cffdrs installed (drop --local to use Docker)"
	echo "Using local Rscript."
	if [ "$DRY_RUN" = 1 ]; then
		echo "would run: (cd $REPO_ROOT && Rscript testdata/gen_cffdrs_reference.R)"
		exit 0
	fi
	(cd "$REPO_ROOT" && Rscript testdata/gen_cffdrs_reference.R)
	;;
docker)
	command -v docker >/dev/null 2>&1 || die "docker not found, and no local R with cffdrs"
	if [ "$DRY_RUN" = 1 ]; then
		echo "would build: $IMAGE (R $R_VERSION, cffdrs $CFFDRS_VERSION)"
		echo "would run:   docker run --rm -v $REPO_ROOT:/repo $IMAGE"
		exit 0
	fi
	if [ "$REBUILD" = 1 ] || ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
		echo "Building $IMAGE (first build pulls an R toolchain and GDAL; not quick)..."
		docker build \
			--build-arg "R_VERSION=$R_VERSION" \
			--build-arg "CFFDRS_VERSION=$CFFDRS_VERSION" \
			-t "$IMAGE" "$SCRIPT_DIR"
	fi
	echo "Running the sweep in $IMAGE..."
	# The container writes the fixture as root; on Linux that would leave a
	# root-owned file in the tree, so run as the caller where the concept exists.
	USER_ARGS=()
	if [ "$(uname -s)" != "Darwin" ] && command -v id >/dev/null 2>&1 && [ "$(id -u)" != "0" ]; then
		case "$(uname -s)" in MINGW* | MSYS* | CYGWIN*) ;; *) USER_ARGS=(--user "$(id -u):$(id -g)") ;; esac
	fi
	docker run --rm "${USER_ARGS[@]}" -v "$REPO_ROOT:/repo" "$IMAGE"
	;;
esac

echo
echo "Fixture after:"
fixture_versions
echo
echo "Now read the diff, then:  go test . -v -run TestCFFDRS"
echo "A changed reference number is the oracle disagreeing with this package."
