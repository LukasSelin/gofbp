package ledger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The real thing, which is also the fixture every other test here mutates.
const real = "../../MIGRATION.md"

func TestLoadTheRealLedger(t *testing.T) {
	l, err := Load(real)
	if err != nil {
		t.Fatal(err)
	}

	if len(l.Rows) < 15 {
		t.Errorf("%d rows", len(l.Rows))
	}
	if len(l.Pins) < 4 {
		t.Errorf("%d pins", len(l.Pins))
	}
	if len(l.Log) < 1 {
		t.Errorf("%d log entries", len(l.Log))
	}
	if len(l.DependencyOrder) < 5 {
		t.Errorf("dependency order: %v", l.DependencyOrder)
	}

	byKey := l.ByKey()
	// A row that owns several upstream files must be reachable by each of them,
	// and the index must carry the row's real status rather than the first one it
	// saw. Two rows exercise that: FMC, which owns two files and is ported, and
	// the acceleration model, which owns three and is still owed.
	for _, tc := range []struct {
		files  []string
		status string
	}{
		{[]string{"foliar_moisture_content.r", "foliar_moisture_content_minimum.r"}, Ported},
		{[]string{"rate_of_spread_at_time.r", "distance_at_time.r", "length_to_breadth_at_time.r"}, Missing},
	} {
		for _, f := range tc.files {
			r, ok := byKey[f]
			if !ok {
				t.Fatalf("%s is not indexed", f)
			}
			if r.Status != tc.status {
				t.Errorf("%s: status %q, want %q", f, r.Status, tc.status)
			}
		}
	}

	// And one upstream file can be two rows. initial_spread_index.r is both
	// FBP's ISI and the FWI System's, one per value of fbpMod: ByFile reaches
	// both, ByKey tells them apart, and neither swallows the other.
	isi := l.ByFile()["initial_spread_index.r"]
	if len(isi) != 2 {
		t.Fatalf("initial_spread_index.r has %d rows, want 2 (one per fbpMod)", len(isi))
	}
	for _, v := range []string{"fbpMod = TRUE", "fbpMod = FALSE"} {
		r, ok := byKey[Key("initial_spread_index.r", v)]
		if !ok {
			t.Errorf("no row keyed %q", Key("initial_spread_index.r", v))
			continue
		}
		if r.Variant != v || r.Status != Ported {
			t.Errorf("%s: variant %q status %q", r, r.Variant, r.Status)
		}
	}
	if _, bare := byKey["initial_spread_index.r"]; bare {
		t.Error("a file split across variant rows must not also be reachable by its bare name")
	}

	if sha, err := l.UpstreamCommit(); err != nil || sha == "" {
		t.Errorf("UpstreamCommit() = %q, %v", sha, err)
	}
	if v, err := l.UpstreamVersion(); err != nil || v == "" {
		t.Errorf("UpstreamVersion() = %q, %v", v, err)
	}
}

// The dependency order is a list, and its order is the whole point: /migration-port
// takes the top item. Parsing must preserve it and must not deduplicate it into a
// set.
func TestDependencyOrderKeepsItsOrder(t *testing.T) {
	l, err := Load(real)
	if err != nil {
		t.Fatal(err)
	}
	// This tracks the ledger's current top row, so it moves with every port —
	// SFC held it until 2026-09-18, then FMC for the rest of that day. Updating
	// it IS the check: a parser that deduplicated or reordered the list would
	// land on something else.
	if got := l.DependencyOrder[0]; got != "crown_base_height.r" {
		t.Errorf("first item = %q, want crown_base_height.r — the CBH/CFL tables are the "+
			"top unblocked row", got)
	}
	seen := map[string]bool{}
	for _, f := range l.DependencyOrder {
		if seen[f] {
			t.Errorf("%s appears twice", f)
		}
		seen[f] = true
		if !strings.HasSuffix(strings.ToLower(f), ".r") {
			t.Errorf("%q is not an upstream R file; prose backticks must not reach the list", f)
		}
	}
}

// mutate writes a copy of the real ledger with one substitution applied.
func mutate(t *testing.T, old, new string) string {
	t.Helper()
	raw, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if old != "" {
		if !strings.Contains(s, old) {
			t.Fatalf("the ledger no longer contains %q, so this test is not testing anything", old)
		}
		s = strings.Replace(s, old, new, 1)
	}
	path := filepath.Join(t.TempDir(), "MIGRATION.md")
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A parser that recovers from a missing section is worse than one that fails:
// every check built on it would pass by finding nothing to check.
func TestAMissingSectionIsAnError(t *testing.T) {
	for _, tc := range []struct {
		name, old, new, wantIn string
	}{
		{"the R/ table", "## R/ — file by file", "## R/ files (renamed)", "R/ — file by file"},
		{"the status key", "## Status key", "## Key", "Status key"},
		{"the pins", "## Pins", "## Pinned versions", "Pins"},
		{"the log", "## Log", "## History", "Log"},
		{"the dependency order", "## Concepts still missing, in dependency order", "## TODO", "dependency-order"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(mutate(t, tc.old, tc.new))
			if err == nil {
				t.Fatalf("renaming %s parsed cleanly; the checks above this would pass vacuously", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Errorf("error %q does not name what is missing (%q)", err, tc.wantIn)
			}
		})
	}
}

// The dependency order joins to the R/ table on backticked filenames. An item
// written without one is invisible to that join, so a list with none at all is an
// error rather than an empty result.
func TestADependencyOrderWithNoFilenamesIsAnError(t *testing.T) {
	raw, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	head, tail, ok := strings.Cut(s, dependencyHeading)
	if !ok {
		t.Fatal("the dependency-order heading is gone")
	}
	_, after, ok := strings.Cut(tail, "\n## ")
	if !ok {
		t.Fatal("no section follows the dependency order")
	}
	stripped := head + dependencyHeading + "\n\n1. **SFC** — no file named.\n\n## " + after

	path := filepath.Join(t.TempDir(), "MIGRATION.md")
	if err := os.WriteFile(path, []byte(stripped), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("a dependency order naming no files parsed cleanly")
	}
}

func TestAMalformedRowIsAnError(t *testing.T) {
	// A row whose first cell has no backticked filename cannot be joined to
	// anything — not to a test marker, not to an upstream diff.
	_, err := Load(mutate(t,
		"| `buildup_effect.r` | BE |",
		"| buildup_effect.r | BE |"))
	if err == nil {
		t.Fatal("a row with no backticked filename parsed cleanly")
	}
	if !strings.Contains(err.Error(), "names no upstream file") {
		t.Errorf("unhelpful error: %v", err)
	}
}

func TestBackticked(t *testing.T) {
	got := Backticked("`foliar_moisture_content.r`, `foliar_moisture_content_minimum.r` and `D0`")
	want := []string{"foliar_moisture_content.r", "foliar_moisture_content_minimum.r", "D0"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
			break
		}
	}
	if n := len(Backticked("no code spans here")); n != 0 {
		t.Errorf("got %d tokens from prose", n)
	}
}
