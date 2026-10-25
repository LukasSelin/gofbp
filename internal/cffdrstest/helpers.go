// Package cffdrstest is the test support the FBP packages share: the cffdrs
// fixture and its case type, the divergence register, and the tolerances every
// oracle test compares with. It imports none of the packages it serves, so each
// of them can use it from its own internal tests.
package cffdrstest

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// MaxF is the larger of a and b.
func MaxF(a, b float64) float64 {
	if b > a {
		return b
	}
	return a
}

// AngleDiff is the unsigned separation of two azimuths, so 359.9 and 0.1 are
// 0.2 apart rather than 359.8.
func AngleDiff(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), 360)
	return math.Min(d, 360-d)
}

// RepoPath is rel, a slash-separated path from the repository root, resolved
// from wherever go test is running. Each package's tests run in that package's
// directory, so a bare "testdata/cffdrs.json" would be absent from all but the
// root — and absent is a skip, not a failure.
func RepoPath(t *testing.T, rel string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, filepath.FromSlash(rel))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above the working directory; cannot find %s", rel)
		}
		dir = parent
	}
}
