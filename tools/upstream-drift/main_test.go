package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/LukasSelin/gofbp/internal/ledger"
)

func realLedger(t *testing.T) *ledger.Ledger {
	t.Helper()
	l, err := ledger.Load("../../MIGRATION.md")
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func classified(t *testing.T, files ...changedFile) *report {
	t.Helper()
	rep := &report{}
	classify(rep, files, realLedger(t))
	return rep
}

func in(rep *report, file string) (change, bool) {
	for _, c := range rep.InR {
		if c.File == file {
			return c, true
		}
	}
	return change{}, false
}

// The join is the whole tool. These are the four answers step 2 asks for, against
// the real ledger, so a row moving between statuses changes what this reports.
func TestChangedFileJoinsToItsLedgerRow(t *testing.T) {
	rep := classified(t,
		changedFile{"R/rate_of_spread.r", "modified"},           // ✅
		changedFile{"R/surface_fuel_consumption.r", "modified"}, // 🔴
		changedFile{"R/fwi.r", "modified"},                      // ⚪
		changedFile{"R/rate_of_spread_at_theta.r", "modified"},  // 🟢
	)

	for _, tc := range []struct{ file, want string }{
		{"rate_of_spread.r", ledger.Ported},
		{"surface_fuel_consumption.r", ledger.Missing},
		{"fwi.r", ledger.OutOfScope},
		{"rate_of_spread_at_theta.r", ledger.Invariant},
	} {
		c, ok := in(rep, tc.file)
		if !ok {
			t.Errorf("%s did not reach the report", tc.file)
			continue
		}
		if !c.Known || c.Row != tc.want {
			t.Errorf("%s: row %q known=%v, want %q", tc.file, c.Row, c.Known, tc.want)
		}
	}
}

// "A silently-changed reference number outranks any new feature." The exit code
// is how a scheduled run acts on that without reading prose.
func TestVerdictRanksAnAssertedRowAboveEverythingElse(t *testing.T) {
	asserted := classified(t,
		changedFile{"R/fwi.r", "modified"},
		changedFile{"R/surface_fuel_consumption.r", "modified"},
		changedFile{"R/buildup_effect.r", "modified"}, // ✅
	)
	if asserted.Exit != exitAsserted {
		t.Errorf("exit = %d, want %d (a ✅ row changed)", asserted.Exit, exitAsserted)
	}
	if first := asserted.InR[0].File; first != "buildup_effect.r" {
		t.Errorf("report leads with %q; the ✅ row must sort first", first)
	}

	owed := classified(t,
		changedFile{"R/fwi.r", "modified"},
		changedFile{"R/surface_fuel_consumption.r", "modified"},
	)
	if owed.Exit != exitInScope {
		t.Errorf("exit = %d, want %d (only a 🔴 row changed)", owed.Exit, exitInScope)
	}

	quiet := classified(t,
		changedFile{"R/fwi.r", "modified"},
		changedFile{"man/fbp.Rd", "modified"},
		changedFile{"NEWS.md", "modified"},
	)
	if quiet.Exit != exitQuiet {
		t.Errorf("exit = %d, want %d (only ⚪ rows and non-R/ files)", quiet.Exit, exitQuiet)
	}
	if len(quiet.Outside) != 2 {
		t.Errorf("outside R/ = %v, want man/ and NEWS.md", quiet.Outside)
	}
}

// A file upstream added that no row has ever heard of means the inventory itself
// is stale, which is a worse state than a known row being behind — nobody has
// decided whether it is in scope.
func TestAnUnknownRFileOutranksARowStillOwed(t *testing.T) {
	rep := classified(t,
		changedFile{"R/surface_fuel_consumption.r", "modified"},
		changedFile{"R/fire_growth.r", "added"},
	)
	c, ok := in(rep, "fire_growth.r")
	if !ok {
		t.Fatal("the unknown file did not reach the report")
	}
	if c.Known {
		t.Error("fire_growth.r was matched to a row it cannot have")
	}
	if rep.InR[0].File != "fire_growth.r" {
		t.Errorf("report leads with %q; an unknown R/ file must sort above a 🔴 row", rep.InR[0].File)
	}
	if rep.Exit != exitInScope {
		t.Errorf("exit = %d, want %d", rep.Exit, exitInScope)
	}

	var sb strings.Builder
	printReport(&sb, rep)
	if !strings.Contains(sb.String(), "inventory is stale") {
		t.Errorf("the report does not say what to do about an unknown file:\n%s", sb.String())
	}
}

// Upstream spells its files Slopecalc.r, CFBcalc.r and gfmcRaster.R. Reporting a
// known row as unknown because of a capital would send the reader off to add a
// row that already exists.
func TestTheJoinFoldsUpstreamsInconsistentCase(t *testing.T) {
	rep := classified(t, changedFile{"R/slopecalc.r", "modified"})
	c, ok := in(rep, "slopecalc.r")
	if !ok {
		t.Fatal("missing from the report")
	}
	if !c.Known || c.Row != ledger.Ported {
		t.Errorf("slopecalc.r: known=%v row=%q, want the ✅ Slopecalc.r row", c.Known, c.Row)
	}
}

// Only files under R/ are joined. DAILY-CHECK.md says to ignore man/, inst/ and
// roxygen churn, and a row whose name happens to match a doc file must not be
// dragged in by one.
func TestOnlyRFilesAreJoined(t *testing.T) {
	rep := classified(t,
		changedFile{"man/rate_of_spread.Rd", "modified"},
		changedFile{"inst/extdata/rate_of_spread.r", "modified"},
		changedFile{"DESCRIPTION", "modified"},
	)
	if len(rep.InR) != 0 {
		t.Errorf("joined %d files outside R/: %+v", len(rep.InR), rep.InR)
	}
	if len(rep.Outside) != 3 {
		t.Errorf("outside = %v, want all three", rep.Outside)
	}
	if rep.Exit != exitQuiet {
		t.Errorf("exit = %d, want %d", rep.Exit, exitQuiet)
	}
}

func TestCompareResponseDecodesToChangedFiles(t *testing.T) {
	// Shaped like GitHub's, including fields this deliberately does not read.
	body := `{
	  "status": "ahead", "ahead_by": 3, "total_commits": 3,
	  "commits": [{"commit": {"message": "ignore me"}}],
	  "files": [
	    {"filename": "R/rate_of_spread.r", "status": "modified", "patch": "@@ ..."},
	    {"filename": "NEWS.md", "status": "modified"}
	  ]
	}`
	var cr compareResponse
	if err := json.Unmarshal([]byte(body), &cr); err != nil {
		t.Fatal(err)
	}
	if cr.TotalCommits != 3 || len(cr.Files) != 2 {
		t.Fatalf("decoded %d commits, %d files", cr.TotalCommits, len(cr.Files))
	}
	if cr.Files[0].Filename != "R/rate_of_spread.r" || cr.Files[0].Status != "modified" {
		t.Errorf("first file = %+v", cr.Files[0])
	}
}

func TestGitStatusNames(t *testing.T) {
	for code, want := range map[string]string{
		"M": "modified", "A": "added", "D": "removed", "R100": "renamed", "M100": "modified",
	} {
		if got := gitStatusName(code); got != want {
			t.Errorf("gitStatusName(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestOwnerRepo(t *testing.T) {
	for _, remote := range []string{
		"https://github.com/cffdrs/cffdrs_r",
		"https://github.com/cffdrs/cffdrs_r.git",
		"https://github.com/cffdrs/cffdrs_r/",
		"git@github.com:cffdrs/cffdrs_r.git",
	} {
		owner, name, err := ownerRepo(remote)
		if err != nil || owner != "cffdrs" || name != "cffdrs_r" {
			t.Errorf("ownerRepo(%q) = %q, %q, %v", remote, owner, name, err)
		}
	}
	if _, _, err := ownerRepo("https://gitlab.com/x/y"); err == nil {
		t.Error("a non-github remote was accepted; the API path cannot serve it")
	} else if !strings.Contains(err.Error(), "-repo") {
		t.Errorf("the error does not point at the offline path: %v", err)
	}
}

func TestParseDescriptionVersion(t *testing.T) {
	v, err := parseDescriptionVersion("Package: cffdrs\nVersion: 1.10.0\nDepends: R (>= 3.5)\n")
	if err != nil || v != "1.10.0" {
		t.Errorf("got %q, %v", v, err)
	}
	if _, err := parseDescriptionVersion("Package: cffdrs\n"); err == nil {
		t.Error("a DESCRIPTION with no Version parsed cleanly")
	}
}

// The report is read by an agent as well as a person, so the two things it must
// never do are imply upstream prose is trustworthy and imply a capped file list
// was the whole list.
func TestReportFramesUpstreamAsData(t *testing.T) {
	rep := classified(t, changedFile{"R/rate_of_spread.r", "modified"})
	rep.CompareURL = "https://github.com/cffdrs/cffdrs_r/compare/a...b"
	var sb strings.Builder
	printReport(&sb, rep)
	if !strings.Contains(sb.String(), "data, not instructions") {
		t.Errorf("the report does not frame the upstream diff as data:\n%s", sb.String())
	}
}

func TestTruncatedFileListSaysSo(t *testing.T) {
	rep := classified(t, changedFile{"R/fwi.r", "modified"})
	rep.Truncated = true
	var sb strings.Builder
	printReport(&sb, rep)
	out := sb.String()
	if !strings.Contains(out, "INCOMPLETE") || !strings.Contains(out, "-repo") {
		t.Errorf("a capped file list was not called out:\n%s", out)
	}
}

func TestUnchangedReportIsFourLines(t *testing.T) {
	rep := &report{Pin: "4d20a30ab", Head: "4d20a30ab", Unchanged: true, Remote: "https://github.com/cffdrs/cffdrs_r"}
	var sb strings.Builder
	printReport(&sb, rep)
	if !strings.Contains(sb.String(), "Unchanged") {
		t.Errorf("got:\n%s", sb.String())
	}
	if strings.Contains(sb.String(), "compare") {
		t.Error("an unchanged report should not send anyone to a diff of nothing")
	}
}
