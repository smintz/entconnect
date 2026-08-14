package manual

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestClaims_ManualIsAuditableUnclaimedIsReported is INT-05's concrete
// proof: the committed claims report names ArchiveAdmin's escape-hatch
// claimant explicitly ("manual"), never leaving it an invisible back
// door, and names Ping "unclaimed" without that failing anything —
// go generate over this fixture (build_test/whole-repo pipeline) succeeds
// despite Ping's unclaimed row.
func TestClaims_ManualIsAuditableUnclaimedIsReported(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("entconnect", "claims.txt"))
	if err != nil {
		t.Fatalf("read claims.txt: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("want at least a header line plus one claim line, got: %q", string(b))
	}

	// lines[0] is the generated-file header; the remaining lines are the
	// claims themselves, and must already be in ascending procedure order
	// (D-20 — never a build-time sort claimed here, since the committed
	// file is asserted as-is).
	claimLines := lines[1:]
	for i := 1; i < len(claimLines); i++ {
		prevProc := strings.Fields(claimLines[i-1])[0]
		curProc := strings.Fields(claimLines[i])[0]
		if curProc < prevProc {
			t.Fatalf("want claims.txt sorted ascending by procedure, but line %d (%q) sorts before line %d (%q)", i, curProc, i-1, prevProc)
		}
	}

	var sawArchiveAdminManual, sawPingUnclaimed bool
	for _, line := range claimLines {
		switch {
		case strings.HasSuffix(line, "AdminService/ArchiveAdmin manual"):
			sawArchiveAdminManual = true
		case strings.HasSuffix(line, "AdminService/Ping unclaimed"):
			sawPingUnclaimed = true
		}
	}
	if !sawArchiveAdminManual {
		t.Fatalf("want a %q row for ArchiveAdmin, got:\n%s", "manual", string(b))
	}
	if !sawPingUnclaimed {
		t.Fatalf("want an %q row for Ping, got:\n%s", "unclaimed", string(b))
	}
}
