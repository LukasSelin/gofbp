// Command precheck answers one question before any migration work starts: can
// this session say whether a coefficient is right?
//
// DAILY-CHECK.md and both migration commands already carry the rule — without
// testdata/cffdrs.json the TestCFFDRS* tests skip, and a session that cannot run
// them can read and audit but must not port. Until now that rule was prose asking
// to be obeyed. This makes it an exit code, so a session cannot proceed through it
// by deciding the code looks right.
//
// Usage:
//
//	precheck [options]
//
//	-mode port|audit   what the session intends to do (default port)
//	-json              emit the verdict as JSON
//	-skip-tests        report on the fixture only; do not run go test
//
// Exit status:
//
//	0  the session can do what -mode asked
//	1  degraded: it can audit, but it must NOT port a coefficient
//	2  it cannot do either — go test ./... is red, and today's job is that
//	3  precheck itself could not answer
//
// # Why a digest mismatch blocks
//
// A fixture whose sha256 does not match the one recorded in testdata/README.md is
// one of two things, and they are not the same finding:
//
//   - It was generated against different versions than the ledger pins. The
//     oracle is then not the oracle this repo's claims were measured against, so
//     a port asserted with it means less than it looks like it means.
//   - It was generated against the SAME versions and the numbers still moved.
//     That is the reference implementation disagreeing with the recorded state,
//     which testdata/README.md calls a finding and not noise.
//
// Both block porting; the report says which one it is, because the second is a
// thing to go and investigate rather than to regenerate past.
//
// Stdlib only.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/LukasSelin/gofbp/internal/ledger"
)

const (
	exitReady    = 0
	exitDegraded = 1
	exitRed      = 2
	exitUnknown  = 3
)

const fixturePath = "testdata/cffdrs.json"

type verdict struct {
	Mode string `json:"mode"`

	Branch string `json:"branch,omitempty"`
	Dirty  int    `json:"dirty_files"`

	TestsRun  bool   `json:"tests_run"`
	TestsPass bool   `json:"tests_pass"`
	TestTail  string `json:"test_tail,omitempty"`

	FixturePresent bool   `json:"fixture_present"`
	FixtureBytes   int64  `json:"fixture_bytes,omitempty"`
	FixtureSHA     string `json:"fixture_sha256,omitempty"`
	RecordedSHA    string `json:"recorded_sha256,omitempty"`

	FixtureCFFDRS string `json:"fixture_cffdrs,omitempty"`
	FixtureR      string `json:"fixture_r,omitempty"`
	PinCFFDRS     string `json:"pin_cffdrs,omitempty"`
	PinR          string `json:"pin_r,omitempty"`

	OracleTests   int `json:"oracle_tests"`
	OracleSkipped int `json:"oracle_skipped"`

	CanAudit bool     `json:"can_audit"`
	CanPort  bool     `json:"can_port"`
	Blockers []string `json:"blockers,omitempty"`
	Exit     int      `json:"exit"`
}

func main() {
	mode := flag.String("mode", "port", "what the session intends to do: port or audit")
	asJSON := flag.Bool("json", false, "emit the verdict as JSON")
	skipTests := flag.Bool("skip-tests", false, "report on the fixture only; do not run go test")
	flag.Parse()

	if *mode != "port" && *mode != "audit" {
		fmt.Fprintf(os.Stderr, "precheck: -mode must be port or audit, got %q\n", *mode)
		os.Exit(exitUnknown)
	}

	v := &verdict{Mode: *mode, CanAudit: true, CanPort: true}
	v.Branch, v.Dirty = gitState()

	// Tests first: whether the oracle tests ran and passed is evidence the fixture
	// check needs when a digest does not match.
	if !*skipTests {
		runTests(v)
	}
	checkFixture(v)
	decide(v)

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(v); err != nil {
			fmt.Fprintf(os.Stderr, "precheck: %v\n", err)
			os.Exit(exitUnknown)
		}
	} else {
		printVerdict(os.Stdout, v)
	}
	os.Exit(v.Exit)
}

// block records something that stops a coefficient being ported. Auditing
// survives all of these: reading the ledger needs no oracle.
func (v *verdict) block(reason string) {
	v.CanPort = false
	v.Blockers = append(v.Blockers, reason)
}

func checkFixture(v *verdict) {
	// The pins first: without them there is nothing to compare a fixture against,
	// and that is itself a reason not to trust one.
	if l, err := ledger.Load("MIGRATION.md"); err == nil {
		if p, ok := l.FindPin("Oracle pins"); ok {
			if m := regexp.MustCompile(`cffdrs ([\d.]+), R ([\d.]+)`).FindStringSubmatch(p.Value); m != nil {
				v.PinCFFDRS, v.PinR = m[1], m[2]
			}
		}
	} else {
		v.block(fmt.Sprintf("MIGRATION.md does not parse (%v), so the pins cannot be read", err))
	}
	v.RecordedSHA = recordedDigest()

	info, err := os.Stat(fixturePath)
	if err != nil {
		v.block("there is no " + fixturePath + ", so every TestCFFDRS* skips. " +
			"Generate it with ./testdata/regen-cffdrs.sh (Docker, ~10 min the first time).")
		return
	}
	v.FixturePresent = true
	v.FixtureBytes = info.Size()

	sum, meta, err := digestAndMeta(fixturePath)
	if err != nil {
		v.block(fmt.Sprintf("%s is unreadable: %v", fixturePath, err))
		return
	}
	v.FixtureSHA = sum
	v.FixtureCFFDRS, v.FixtureR = meta.cffdrs, meta.r

	versionsMatch := v.PinCFFDRS != "" && v.FixtureCFFDRS == v.PinCFFDRS &&
		v.PinR != "" && strings.Contains(v.FixtureR, v.PinR)

	switch {
	case v.RecordedSHA == "":
		v.block("testdata/README.md records no sha256, so this fixture cannot be identified")
	case sum == v.RecordedSHA:
		// The good case: the oracle is the one the repo's claims were measured
		// against.
	case !versionsMatch:
		v.block(fmt.Sprintf("the fixture was generated against cffdrs %s / R %s, but the ledger "+
			"pins cffdrs %s / R %s. Its numbers are not the ones this repo's claims were "+
			"measured against. Regenerate at the pins, or treat the bump as the reviewed "+
			"step it is.", v.FixtureCFFDRS, shortR(v.FixtureR), v.PinCFFDRS, v.PinR))
	default:
		// This is the case that must not overclaim. The digest is taken over the
		// WHOLE file, and the file's own metadata — including r_version's build
		// date, "R version 4.6.1 (2026-06-24)" — is inside it. So a different build
		// of the pinned R version changes this digest without moving a single
		// number, and the digest alone cannot tell that apart from numbers that
		// really moved. Say what is known and what is not.
		msg := fmt.Sprintf("the fixture reports the pinned versions (cffdrs %s, R %s) but its digest "+
			"is not the recorded one.\n     recorded  %s\n     this one  %s\n"+
			"     r_version  %q",
			v.FixtureCFFDRS, v.PinR, v.RecordedSHA, sum, v.FixtureR)
		msg += "\n     That is one of two things and the digest cannot separate them: a different\n" +
			"     BUILD of the same R version (its build date is hashed as part of the file), or\n" +
			"     reference numbers that actually moved."
		switch {
		case v.TestsRun && v.OracleTests > 0 && v.OracleSkipped == 0 && v.TestsPass:
			msg += fmt.Sprintf("\n     All %d TestCFFDRS* passed against it, which is evidence the values agree\n"+
				"     with this package — but not that they match the fixture the ledger's claims\n"+
				"     were measured against.", v.OracleTests)
		case v.TestsRun:
			msg += "\n     The oracle tests did not all run against it, so there is no evidence either way."
		}
		msg += "\n     Either way this is a reviewed step, not one to regenerate past: re-baselining\n" +
			"     the recorded digest means saying in the PR what the reference numbers did."
		v.block(msg)
	}
}

type fixtureMeta struct{ cffdrs, r string }

// digestAndMeta hashes the fixture and reads the two versions recorded into it,
// in one pass. The fixture is ~9.5 MB, which is small enough to stream and large
// enough not to hold twice.
func digestAndMeta(path string) (string, fixtureMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fixtureMeta{}, err
	}
	defer f.Close()

	h := sha256.New()
	// The metadata is in the first few hundred bytes — the generator emits it
	// before "cases" — so only the head is kept.
	var head strings.Builder
	tee := io.TeeReader(f, h)
	if _, err := io.Copy(io.Discard, io.TeeReader(io.LimitReader(tee, 4096), &head)); err != nil {
		return "", fixtureMeta{}, err
	}
	if _, err := io.Copy(io.Discard, tee); err != nil {
		return "", fixtureMeta{}, err
	}
	return hex.EncodeToString(h.Sum(nil)), parseMeta(head.String()), nil
}

var (
	metaCFFDRS = regexp.MustCompile(`"cffdrs_version"\s*:\s*"([^"]*)"`)
	metaR      = regexp.MustCompile(`"r_version"\s*:\s*"([^"]*)"`)
)

func parseMeta(head string) fixtureMeta {
	var m fixtureMeta
	if g := metaCFFDRS.FindStringSubmatch(head); g != nil {
		m.cffdrs = g[1]
	}
	if g := metaR.FindStringSubmatch(head); g != nil {
		m.r = g[1]
	}
	return m
}

// shortR turns "R version 4.6.1 (2026-02-28)" into "4.6.1".
func shortR(s string) string {
	if m := regexp.MustCompile(`\d+\.\d+\.\d+`).FindString(s); m != "" {
		return m
	}
	if s == "" {
		return "unknown"
	}
	return s
}

// recordedDigest reads the CURRENT digest from testdata/README.md. That file also
// keeps the previous one, deliberately, so a stale fixture identifies itself —
// which is exactly why this takes the first and not any match.
func recordedDigest() string {
	raw, err := os.ReadFile("testdata/README.md")
	if err != nil {
		return ""
	}
	m := regexp.MustCompile(`(?m)^sha256\s+([0-9a-f]{64})$`).FindSubmatch(raw)
	if m == nil {
		return ""
	}
	return string(m[1])
}

func runTests(v *verdict) {
	v.TestsRun = true
	out, err := exec.Command("go", "test", "./...").CombinedOutput()
	v.TestsPass = err == nil
	if !v.TestsPass {
		v.TestTail = tail(string(out), 20)
		v.CanAudit = false
		v.block("go test ./... is red. Today's job is that, not the ledger.")
	}

	// Counted rather than hardcoded: the number of oracle tests grows with every
	// ported row.
	vo, _ := exec.Command("go", "test", ".", "-run", "TestCFFDRS", "-v").CombinedOutput()
	v.OracleTests, v.OracleSkipped = countOracle(string(vo))
	if v.TestsPass && v.OracleTests > 0 && v.OracleSkipped > 0 {
		v.block(fmt.Sprintf("%d of %d TestCFFDRS* tests skipped, so nothing concluded today about "+
			"coefficients is backed by anything.", v.OracleSkipped, v.OracleTests))
	}
}

var (
	oracleRun  = regexp.MustCompile(`(?m)^=== RUN\s+TestCFFDRS[^/\r\n]*$`)
	oracleSkip = regexp.MustCompile(`(?m)^--- SKIP:\s+TestCFFDRS[^/\r\n]*\(`)
)

// countOracle reads `go test -v` output.
//
// Two things separate a top-level test from a subtest, and both are needed. Go
// indents subtest RESULT lines ("    --- SKIP: TestX/sub"), so the column-zero
// anchor handles those — but it does NOT indent their "=== RUN TestX/sub" lines,
// so the name must also be rejected if it contains a slash. Without that second
// half a table-driven oracle test reports as several, and the skip fraction the
// gate blocks on comes out wrong.
func countOracle(out string) (total, skipped int) {
	return len(oracleRun.FindAllString(out, -1)), len(oracleSkip.FindAllString(out, -1))
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func gitState() (branch string, dirty int) {
	if out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command("git", "status", "--porcelain").Output(); err == nil {
		for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if strings.TrimSpace(l) != "" {
				dirty++
			}
		}
	}
	return branch, dirty
}

func decide(v *verdict) {
	switch {
	case !v.CanAudit:
		v.Exit = exitRed
	case v.Mode == "port" && !v.CanPort:
		v.Exit = exitDegraded
	default:
		v.Exit = exitReady
	}
}

func printVerdict(w io.Writer, v *verdict) {
	p := func(f string, a ...any) { fmt.Fprintf(w, f, a...) }

	if v.Branch != "" {
		p("branch   %s", v.Branch)
		if v.Dirty > 0 {
			p(", %d uncommitted file%s", v.Dirty, plural(v.Dirty))
		}
		p("\n")
	}
	if v.TestsRun {
		if v.TestsPass {
			p("tests    go test ./... green")
		} else {
			p("tests    go test ./... RED")
		}
		if v.OracleTests > 0 {
			p(", %d of %d TestCFFDRS* skipping", v.OracleSkipped, v.OracleTests)
		}
		p("\n")
	}
	if v.FixturePresent {
		p("fixture  %d bytes, cffdrs %s / R %s\n", v.FixtureBytes, v.FixtureCFFDRS, shortR(v.FixtureR))
		p("         %s\n", v.FixtureSHA)
	} else {
		p("fixture  absent\n")
	}

	if !v.TestsPass && v.TestsRun {
		p("\n%s\n", v.TestTail)
	}

	p("\n")
	switch {
	case !v.CanAudit:
		p("CANNOT PROCEED. go test ./... is red; today's job is that.\n")
	case v.CanPort:
		p("READY. The oracle is present and is the one the ledger pins, so a coefficient\n" +
			"ported in this session can actually be checked.\n")
	default:
		p("AUDIT ONLY — do NOT port a coefficient in this session.\n\n")
		for _, b := range v.Blockers {
			p("  - %s\n", b)
		}
		p("\nReading the ledger, the upstream diff and the Go is all still fine. What is\n" +
			"not fine is concluding a transcribed number is right: a wrong one looks\n" +
			"exactly like a right one without the fixture.\n")
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
