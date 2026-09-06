// Package ledger reads MIGRATION.md.
//
// The ledger is the repository's set of claims about itself — what is ported,
// what is asserted against the oracle, what is deliberately out of scope — and
// two things now read it: ledger_test.go, which checks the claims are internally
// consistent, and tools/upstream-drift, which joins them to what moved upstream.
// Both need the same answer to "which row owns rate_of_spread.r", so there is one
// parser rather than two that can disagree.
//
// It is a markdown parser only to the extent it has to be: the ledger's tables
// are the interface, and a heading rename is an error rather than something to
// recover from. That is deliberate — a parser that quietly finds nothing would
// make every check above it pass vacuously.
//
// Stdlib only.
package ledger

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Statuses, as defined by the ledger's own status key.
const (
	Ported     = "✅" // ported, and a TestCFFDRS* asserts it against the fixture
	Invariant  = "🟢" // ported, asserted only by identity/invariant tests
	Partial    = "🟡" // partially ported — the note is the gap
	Missing    = "🔴" // not ported, and in scope
	OutOfScope = "⚪" // deliberately out of scope — the reason is the row's content
)

// Row is one line of the "R/ — file by file" table.
type Row struct {
	// Files are the upstream R filenames named in the first cell. A row can own
	// several — the FMC row owns both foliar_moisture_content.r and its _minimum
	// companion — because they are one concept upstream splits across files.
	Files   []string
	Concept string
	Status  string
	Note    string
	Line    int
}

func (r Row) String() string {
	return fmt.Sprintf("%s (%s, line %d)", strings.Join(r.Files, ", "), r.Status, r.Line)
}

// Pin is one row of the Pins table: what was read, and when it was last checked.
type Pin struct {
	Name    string
	Value   string
	Checked string
	Line    int
}

// LogEntry is one row of the Log. A day with no change still gets one, so a gap
// in the dates is visible as a gap.
type LogEntry struct {
	Date   string
	Commit string
	What   string
	Line   int
}

type Ledger struct {
	Path string

	Rows      []Row
	Pins      []Pin
	Log       []LogEntry
	StatusKey map[string]string

	// DependencyOrder is the upstream filenames named in "Concepts still missing,
	// in dependency order", in the order they appear. /migration-port takes its
	// work from the head of this list.
	DependencyOrder []string

	Lines []string
}

// backticked pulls `foo.r` tokens out of a markdown cell. The ledger's first
// column is a comma-separated list of them and its last column mixes filenames
// with Go identifiers, so this is the only reliable way to read either.
var backticked = regexp.MustCompile("`([^`]+)`")

// Backticked returns every `…` token in s.
func Backticked(s string) []string {
	var out []string
	for _, m := range backticked.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}

// Load reads and parses a ledger. Every table it needs must be present: a
// missing one is an error, not an empty result, because the checks built on this
// would otherwise pass by finding nothing.
func Load(path string) (*Ledger, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	l := &Ledger{
		Path:  path,
		Lines: strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n"),
	}

	if err := l.parseStatusKey(); err != nil {
		return nil, err
	}
	if err := l.parseRows(); err != nil {
		return nil, err
	}
	if err := l.parsePins(); err != nil {
		return nil, err
	}
	if err := l.parseLog(); err != nil {
		return nil, err
	}
	if err := l.parseDependencyOrder(); err != nil {
		return nil, err
	}
	return l, nil
}

// ByFile indexes rows by every upstream filename they claim.
func (l *Ledger) ByFile() map[string]Row {
	idx := make(map[string]Row, len(l.Rows))
	for _, r := range l.Rows {
		for _, f := range r.Files {
			idx[f] = r
		}
	}
	return idx
}

// FindPin returns the first pin whose name contains substr.
func (l *Ledger) FindPin(substr string) (Pin, bool) {
	for _, p := range l.Pins {
		if strings.Contains(p.Name, substr) {
			return p, true
		}
	}
	return Pin{}, false
}

// UpstreamCommit returns the sha from the "Upstream commit last read" pin. That
// sha is the base of every drift comparison, so a ledger that has lost it cannot
// be audited at all.
func (l *Ledger) UpstreamCommit() (string, error) {
	p, ok := l.FindPin("Upstream commit last read")
	if !ok {
		return "", fmt.Errorf("%s: no \"Upstream commit last read\" row in the Pins table", l.Path)
	}
	tokens := Backticked(p.Value)
	if len(tokens) == 0 {
		return "", fmt.Errorf("%s:%d: the upstream commit pin names no sha", l.Path, p.Line)
	}
	return tokens[0], nil
}

// UpstreamVersion returns the package version from the "Upstream package version"
// pin.
func (l *Ledger) UpstreamVersion() (string, error) {
	p, ok := l.FindPin("Upstream package version")
	if !ok {
		return "", fmt.Errorf("%s: no \"Upstream package version\" row in the Pins table", l.Path)
	}
	return strings.TrimSpace(p.Value), nil
}

// --- table reading ----------------------------------------------------------

// tableUnder returns the cells of the first markdown table following heading,
// with the header row and the |---|---| separator dropped, plus each row's
// 1-based line number.
func (l *Ledger) tableUnder(heading string) ([][]string, []int, error) {
	start := -1
	for i, line := range l.Lines {
		if strings.TrimSpace(line) == heading {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, nil, fmt.Errorf("%s: heading %q is gone. The ledger's shape is part of "+
			"its contract — if the section was renamed, rename it here too rather than "+
			"letting the checks above pass by finding nothing", l.Path, heading)
	}

	var rows [][]string
	var lineNos []int
	inTable := false
	for i := start + 1; i < len(l.Lines); i++ {
		line := strings.TrimSpace(l.Lines[i])
		if !strings.HasPrefix(line, "|") {
			if inTable || strings.HasPrefix(line, "## ") {
				break
			}
			continue
		}
		inTable = true
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for j := range cells {
			cells[j] = strings.TrimSpace(cells[j])
		}
		if isSeparator(cells) {
			continue
		}
		rows = append(rows, cells)
		lineNos = append(lineNos, i+1)
	}
	if len(rows) < 2 {
		return nil, nil, fmt.Errorf("%s: no table under %q", l.Path, heading)
	}
	return rows[1:], lineNos[1:], nil // drop the header row
}

func isSeparator(cells []string) bool {
	for _, c := range cells {
		if strings.Trim(c, "-: ") != "" {
			return false
		}
	}
	return len(cells) > 0
}

func (l *Ledger) parseStatusKey() error {
	cells, _, err := l.tableUnder("## Status key")
	if err != nil {
		return err
	}
	l.StatusKey = map[string]string{}
	for _, c := range cells {
		if len(c) >= 2 && c[0] != "" {
			l.StatusKey[c[0]] = c[1]
		}
	}
	return nil
}

func (l *Ledger) parseRows() error {
	cells, lineNos, err := l.tableUnder("## R/ — file by file")
	if err != nil {
		return err
	}
	for i, c := range cells {
		if len(c) < 4 {
			return fmt.Errorf("%s:%d: expected 4 columns in the R/ table, got %d", l.Path, lineNos[i], len(c))
		}
		files := Backticked(c[0])
		if len(files) == 0 {
			return fmt.Errorf("%s:%d: first column names no upstream file", l.Path, lineNos[i])
		}
		l.Rows = append(l.Rows, Row{
			Files: files, Concept: c[1], Status: c[2], Note: c[3], Line: lineNos[i],
		})
	}
	return nil
}

func (l *Ledger) parsePins() error {
	cells, lineNos, err := l.tableUnder("## Pins")
	if err != nil {
		return err
	}
	for i, c := range cells {
		if len(c) < 3 {
			return fmt.Errorf("%s:%d: expected 3 columns in the Pins table, got %d", l.Path, lineNos[i], len(c))
		}
		l.Pins = append(l.Pins, Pin{Name: c[0], Value: c[1], Checked: c[2], Line: lineNos[i]})
	}
	return nil
}

func (l *Ledger) parseLog() error {
	cells, lineNos, err := l.tableUnder("## Log")
	if err != nil {
		return err
	}
	for i, c := range cells {
		if len(c) < 3 {
			return fmt.Errorf("%s:%d: expected 3 columns in the Log table, got %d", l.Path, lineNos[i], len(c))
		}
		l.Log = append(l.Log, LogEntry{Date: c[0], Commit: c[1], What: c[2], Line: lineNos[i]})
	}
	return nil
}

const dependencyHeading = "## Concepts still missing, in dependency order"

func (l *Ledger) parseDependencyOrder() error {
	start := -1
	for i, line := range l.Lines {
		if strings.TrimSpace(line) == dependencyHeading {
			start = i
			break
		}
	}
	if start < 0 {
		return fmt.Errorf("%s: the dependency-order section is gone", l.Path)
	}
	seen := map[string]bool{}
	for i := start + 1; i < len(l.Lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(l.Lines[i]), "## ") {
			break
		}
		for _, name := range Backticked(l.Lines[i]) {
			// The prose in that section also backticks `D0` and `Crown`. Only the
			// filenames are the join key.
			if !strings.HasSuffix(strings.ToLower(name), ".r") || seen[name] {
				continue
			}
			seen[name] = true
			l.DependencyOrder = append(l.DependencyOrder, name)
		}
	}
	if len(l.DependencyOrder) == 0 {
		return fmt.Errorf("%s: the dependency order names no upstream files. Each item has to "+
			"name the file its row is keyed on, or nothing can join this list to the table", l.Path)
	}
	return nil
}
