package fbp

// MIGRATION.md is a set of claims about this repository, and DAILY-CHECK.md's
// step 4 is a person re-reading those claims once a day and deciding whether
// they are still true. Most of that step is not judgement — it is a table join
// between the ledger, the test files and the pinned toolchain, and a join done
// by eye is a join that passes on the day it stops holding.
//
// This file does the mechanical half. What is left over for step 4 — whether a
// changed upstream coefficient matters, whether an exclusion's reason is still a
// mechanism rather than a symptom — is the half that actually needs reading.
//
// These tests run on a fresh clone with no fixture and no Docker. They assert
// nothing about whether a number is right; that is what the TestCFFDRS* tests
// are for, and one of the things checked here is that those still exist.
//
// The ledger is parsed by internal/ledger, which tools/upstream-drift also uses,
// so there is one definition of the ledger's shape rather than two that can
// disagree about which row owns a file.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/LukasSelin/gofbp/internal/ledger"
)

const ledgerPath = "MIGRATION.md"

func load(t *testing.T) *ledger.Ledger {
	t.Helper()
	l, err := ledger.Load(ledgerPath)
	if err != nil {
		t.Fatalf("read the ledger: %v", err)
	}
	return l
}

// TestLedgerParses is the precondition for every other test here: if the ledger
// stops having the shape these read, they would all pass vacuously.
func TestLedgerParses(t *testing.T) {
	l := load(t)

	if len(l.StatusKey) < 5 {
		t.Fatalf("status key has %d entries, expected the five documented statuses", len(l.StatusKey))
	}
	if len(l.Rows) < 15 {
		t.Fatalf("only %d rows in the R/ table; the upstream inventory cannot have shrunk that far", len(l.Rows))
	}

	seen := map[string]int{}
	for _, r := range l.Rows {
		if _, ok := l.StatusKey[r.Status]; !ok {
			t.Errorf("%s: status %q is not in the status key", r, r.Status)
		}
		if r.Concept == "" {
			t.Errorf("%s: no concept", r)
		}
		for _, f := range r.Files {
			if prev, dup := seen[f]; dup {
				t.Errorf("%s: %s is also claimed by the row at line %d — a file cannot have two statuses", r, f, prev)
			}
			seen[f] = r.Line
		}

		// "⚪ — the reason is the row's whole content." A ⚪ row with an empty note
		// is not out of scope, it is unported, and DAILY-CHECK.md says so in as many
		// words.
		if r.Status == ledger.OutOfScope && strings.TrimSpace(r.Note) == "" {
			t.Errorf("%s: out-of-scope row with no reason. The reason is the row's whole "+
				"content — if you cannot restate it in a sentence, it is not out of scope, "+
				"it is unported. Whether the reason is still a good one is step 4's job, "+
				"not this test's.", r)
		}
	}
}

// oracleMarker reads the `ledger:` line out of a test's doc comment.
var oracleMarker = regexp.MustCompile(`(?m)^ledger:\s*(.+)$`)

// TestLedgerOracleClaimsAreBackedByTests is the check DAILY-CHECK.md step 4 opens
// with: "Every ✅ row: does a TestCFFDRS* still assert it?"
//
// There is no way to derive that link — the ledger says `fbp.go` `RSI` and the
// test is called TestCFFDRSSurfaceROS — so it is declared, once, as a `ledger:`
// line in each oracle test's doc comment. The join is then checked in both
// directions, which is what makes it impossible for a ✅ row to quietly lose its
// oracle coverage: deleting the test breaks this, and so does renaming the row.
func TestLedgerOracleClaimsAreBackedByTests(t *testing.T) {
	l := load(t)
	byFile := l.ByFile()

	assertedBy := map[string][]string{} // upstream file -> test names
	fset := token.NewFileSet()
	matches, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range matches {
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !strings.HasPrefix(fn.Name.Name, "Test") {
				continue
			}
			var marker string
			if fn.Doc != nil {
				if m := oracleMarker.FindStringSubmatch(fn.Doc.Text()); m != nil {
					marker = strings.TrimSpace(m[1])
				}
			}
			isOracle := strings.HasPrefix(fn.Name.Name, "TestCFFDRS")

			if marker == "" {
				if isOracle {
					t.Errorf("%s: %s has no `ledger:` line in its doc comment, so no ledger row "+
						"can claim it as its oracle. Add one naming the upstream R file it asserts.",
						fset.Position(fn.Pos()), fn.Name.Name)
				}
				continue
			}
			if !isOracle {
				t.Errorf("%s: %s carries a `ledger:` marker but is not a TestCFFDRS* test. "+
					"Only fixture-backed tests can support a ✅ claim.", fset.Position(fn.Pos()), fn.Name.Name)
				continue
			}
			for _, name := range strings.Split(marker, ",") {
				name = strings.TrimSpace(name)
				row, known := byFile[name]
				if !known {
					t.Errorf("%s: %s claims to assert %q, which is not a row in %s",
						fset.Position(fn.Pos()), fn.Name.Name, name, ledgerPath)
					continue
				}
				if row.Status != ledger.Ported {
					t.Errorf("%s: %s asserts %q against the fixture, but its row is %q. "+
						"A row with a live TestCFFDRS* is ✅ by the ledger's own definition.",
						fset.Position(fn.Pos()), fn.Name.Name, name, row.Status)
				}
				assertedBy[name] = append(assertedBy[name], fn.Name.Name)
			}
		}
	}

	for _, r := range l.Rows {
		if r.Status != ledger.Ported {
			continue
		}
		covered := false
		for _, f := range r.Files {
			if len(assertedBy[f]) > 0 {
				covered = true
			}
		}
		if !covered {
			t.Errorf("%s: is ✅ — ported AND asserted against the fixture — but no "+
				"TestCFFDRS* names it. A row that quietly lost its oracle coverage is worse "+
				"than one that never had it: it is a false claim.", r)
		}
	}

	names := make([]string, 0, len(assertedBy))
	for f := range assertedBy {
		names = append(names, f)
	}
	sort.Strings(names)
	for _, f := range names {
		t.Logf("%-28s %s", f, strings.Join(assertedBy[f], ", "))
	}
}

// TestLedgerDependencyOrderIsComplete keeps "Concepts still missing, in
// dependency order" honest against the table above it. /migration-port takes the
// top unblocked row from that list, so a 🔴 row missing from it is a row that
// will never be picked up.
func TestLedgerDependencyOrderIsComplete(t *testing.T) {
	l := load(t)
	byFile := l.ByFile()

	listed := map[string]bool{}
	for _, name := range l.DependencyOrder {
		listed[name] = true
		row, known := byFile[name]
		if !known {
			t.Errorf("the dependency order names %q, which is not a row in the R/ table", name)
			continue
		}
		if row.Status != ledger.Missing && row.Status != ledger.Partial {
			t.Errorf("the dependency order still lists %q, but its row is %q — "+
				"work that is done should come off the list", name, row.Status)
		}
	}

	for _, r := range l.Rows {
		if r.Status != ledger.Missing {
			continue
		}
		found := false
		for _, f := range r.Files {
			if listed[f] {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: is 🔴 and in scope, but no item in the dependency order names it. "+
				"/migration-port picks its work from that list, so this row is unreachable.", r)
		}
	}
}

// TestLedgerCoversEveryGoFile is step 4's last line: "Any Go file added or
// changed since yesterday: is it in the ledger at all?"
//
// A file with nothing exported is exempt — it adds no claim a caller can see —
// which keeps doc.go and future internal helpers out of the ledger without an
// exemption list that has to be maintained.
func TestLedgerCoversEveryGoFile(t *testing.T) {
	raw, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)

	fset := token.NewFileSet()
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		exported := 0
		for name, obj := range f.Scope.Objects {
			if ast.IsExported(name) && obj != nil {
				exported++
			}
		}
		if exported == 0 {
			continue
		}
		if !strings.Contains(text, "`"+path+"`") {
			t.Errorf("%s exports %d identifiers but is not named anywhere in %s. "+
				"Every file a caller can see needs a row that says what it is and whether "+
				"it is asserted.", path, exported, ledgerPath)
		}
	}
}

// TestLedgerPinsMatchTheToolchain checks the four places the oracle's pinned
// versions are written down against each other. They are copied by hand today,
// which is exactly the kind of drift nobody notices until a regeneration quietly
// runs against a different cffdrs than the ledger claims.
func TestLedgerPinsMatchTheToolchain(t *testing.T) {
	pin, ok := load(t).FindPin("Oracle pins")
	if !ok {
		t.Fatal("the Pins table has no oracle-pins row")
	}
	m := regexp.MustCompile(`cffdrs ([\d.]+), R ([\d.]+)`).FindStringSubmatch(pin.Value)
	if m == nil {
		t.Fatalf("could not read the oracle pins out of %q", pin.Value)
	}
	ledgerCFFDRS, ledgerR := m[1], m[2]

	// Each of these writes the same two numbers down again, in its own syntax.
	sources := []struct {
		path          string
		cffdrsRe, rRe string
		what          string
	}{
		{"testdata/Dockerfile", `ARG CFFDRS_VERSION=([\d.]+)`, `ARG R_VERSION=([\d.]+)`,
			"the image the oracle is built from"},
		{"testdata/regen-cffdrs.sh", `CFFDRS_VERSION="\$\{CFFDRS_VERSION:-([\d.]+)\}"`, `R_VERSION="\$\{CFFDRS_R_VERSION:-([\d.]+)\}"`,
			"the regeneration wrapper's defaults"},
		{".claude/setup-session.sh", `CFFDRS_PIN="\$\{CFFDRS_VERSION:-([\d.]+)\}"`, `R_PIN="\$\{CFFDRS_R_VERSION:-([\d.]+)\}"`,
			"the session bootstrap's defaults"},
	}
	for _, s := range sources {
		raw, err := os.ReadFile(s.path)
		if err != nil {
			t.Errorf("read %s: %v", s.path, err)
			continue
		}
		for _, probe := range []struct{ re, want, label string }{
			{s.cffdrsRe, ledgerCFFDRS, "cffdrs"},
			{s.rRe, ledgerR, "R"},
		} {
			m := regexp.MustCompile(probe.re).FindSubmatch(raw)
			if m == nil {
				t.Errorf("%s (%s): no %s pin matching %s", s.path, s.what, probe.label, probe.re)
				continue
			}
			if got := string(m[1]); got != probe.want {
				t.Errorf("%s (%s) pins %s %s, but %s says %s. The oracle would be "+
					"regenerated against a version the ledger does not claim.",
					s.path, s.what, probe.label, got, ledgerPath, probe.want)
			}
		}
	}
}

// TestLedgerFixtureDigestMatchesTheRecordedOne checks the ledger's abbreviated
// sha256 against the full one in testdata/README.md — the digest a stale local
// fixture is supposed to identify itself by.
func TestLedgerFixtureDigestMatchesTheRecordedOne(t *testing.T) {
	pin, ok := load(t).FindPin("Fixture sha256")
	if !ok {
		t.Fatal("the Pins table has no fixture sha256")
	}
	tokens := ledger.Backticked(pin.Value)
	if len(tokens) == 0 {
		t.Fatalf("the fixture pin names no digest: %q", pin.Value)
	}
	abbrev := tokens[0]
	head, tail, found := strings.Cut(abbrev, "…")
	if !found {
		head, tail = abbrev, ""
	}

	raw, err := os.ReadFile("testdata/README.md")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^sha256\s+([0-9a-f]{64})$`).FindSubmatch(raw)
	if m == nil {
		t.Fatal("testdata/README.md records no full sha256")
	}
	full := string(m[1])
	if !strings.HasPrefix(full, head) || !strings.HasSuffix(full, tail) {
		t.Errorf("%s says the fixture is %s but testdata/README.md records %s. "+
			"One of them was updated after a regeneration and the other was not.",
			ledgerPath, abbrev, full)
	}
}

// TestLedgerLogIsContiguous checks the shape of the Log, not its content. The
// log's whole value is that a gap in the dates means the check was skipped rather
// than that it was quiet — which only holds if the dates are real, unique and in
// order, and if the Pins table was updated on the same day.
func TestLedgerLogIsContiguous(t *testing.T) {
	l := load(t)

	const iso = "2006-01-02"
	var newest time.Time
	seen := map[string]bool{}
	prev := time.Time{}
	for _, e := range l.Log {
		d, err := time.Parse(iso, e.Date)
		if err != nil {
			t.Errorf("%s:%d: %q is not a YYYY-MM-DD date", ledgerPath, e.Line, e.Date)
			continue
		}
		if seen[e.Date] {
			t.Errorf("%s:%d: %s appears twice; one line per audit", ledgerPath, e.Line, e.Date)
		}
		seen[e.Date] = true
		if !prev.IsZero() && !d.Before(prev) {
			t.Errorf("%s:%d: %s is not older than the line above it — the log is newest first",
				ledgerPath, e.Line, e.Date)
		}
		prev = d
		if d.After(newest) {
			newest = d
		}
		if strings.TrimSpace(e.What) == "" {
			t.Errorf("%s:%d: a log line with no description", ledgerPath, e.Line)
		}
	}

	for _, p := range l.Pins {
		if p.Checked == "" {
			continue
		}
		d, err := time.Parse(iso, p.Checked)
		if err != nil {
			t.Errorf("%s:%d: pin checked-date %q is not a YYYY-MM-DD date", ledgerPath, p.Line, p.Checked)
			continue
		}
		if !d.Equal(newest) {
			t.Errorf("%s:%d: this pin was last checked %s but the newest log line is %s. "+
				"Step 6 updates both together, so one of them is stale.",
				ledgerPath, p.Line, p.Checked, newest.Format(iso))
		}
	}
}

// TestPackageHasNoDependencies turns the README's central claim into a check.
// It is described there as load-bearing, and a load-bearing claim that nothing
// tests is a claim about intent rather than about the code.
//
// The go.mod half covers the whole module, internal/ and tools/ included — those
// exist to check this package, and a checking tool that dragged in a dependency
// would put one in the go.sum of everyone who vendors the repo. The `math`-only
// half is about the package a caller imports, which is what the README claims.
func TestPackageHasNoDependencies(t *testing.T) {
	mod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(mod), "\n") {
		if s := strings.TrimSpace(l); strings.HasPrefix(s, "require") {
			t.Errorf("go.mod: %q — the package claims zero dependencies", s)
		}
	}

	fset := token.NewFileSet()
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if p != "math" {
				t.Errorf("%s imports %q. The README's claim is `math` and nothing else; "+
					"widening it is a decision about what this package is, not a refactor.", path, p)
			}
		}
	}
}

// TestFixtureIsNotCommitted guards the licensing boundary. The fixture is GPL-2
// output and this repository is MIT: that is the reason it is generated rather
// than stored, and it is the sort of rule that gets broken by a `git add -A` on a
// machine that happens to have run the generator.
func TestFixtureIsNotCommitted(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git on PATH")
	}
	out, err := exec.Command("git", "ls-files", "testdata/cffdrs.json").Output()
	if err != nil {
		t.Skipf("git ls-files: %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("testdata/cffdrs.json is tracked by git. It is 9.5 MB of GPL-2 output in "+
			"an MIT repository — see testdata/README.md. Untrack it: git rm --cached %s",
			strings.TrimSpace(string(out)))
	}
}

// TestDocsAgreeOnTheNumberOfOracleTests catches the cheapest kind of documentation
// drift: prose that counts something the code can count for itself. Four files
// tell a reader how many fixture-backed tests they are missing without one.
func TestDocsAgreeOnTheNumberOfOracleTests(t *testing.T) {
	fset := token.NewFileSet()
	matches, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	actual := 0
	for _, path := range matches {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && strings.HasPrefix(fn.Name.Name, "TestCFFDRS") {
				actual++
			}
		}
	}

	words := map[int]string{
		1: "one", 2: "two", 3: "three", 4: "four", 5: "five", 6: "six", 7: "seven",
		8: "eight", 9: "nine", 10: "ten", 11: "eleven", 12: "twelve", 13: "thirteen",
		14: "fourteen", 15: "fifteen", 16: "sixteen", 17: "seventeen", 18: "eighteen",
		19: "nineteen", 20: "twenty",
	}
	isCount := map[string]bool{}
	for _, w := range words {
		isCount[w] = true
	}
	for i := 0; i <= 20; i++ {
		isCount[fmt.Sprint(i)] = true
	}

	// Deliberately narrow: README.md also says "the fifteen fuel types", which is a
	// different fifteen and must not be caught here. A word that is not a number at
	// all ("the TestCFFDRS* tests") is prose, not a count.
	counted := regexp.MustCompile(`(\w+) (fixture-backed tests|TestCFFDRS\* tests)`)

	for _, path := range []string{"README.md", "testdata/README.md", ".claude/setup-session.sh", "DAILY-CHECK.md"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		for _, m := range counted.FindAllStringSubmatch(string(raw), -1) {
			if !isCount[strings.ToLower(m[1])] {
				continue // "the TestCFFDRS* tests" — not a count, nothing to check
			}
			if !strings.EqualFold(m[1], words[actual]) && m[1] != fmt.Sprint(actual) {
				t.Errorf("%s says %q, but there are %d TestCFFDRS* functions (%s)",
					path, m[0], actual, words[actual])
			}
		}
	}
}
