package entc

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	entload "entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	goldie "github.com/sebdah/goldie/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// generateFixture loads schemaDir's graph and runs entconnect.Generate
// (against descriptorSetPath, or the committed default when empty) into a
// fresh temp output dir, returning every emitted file's bytes keyed by
// filename (not full path) so callers can golden-assert individually and
// byte-compare across repeated calls.
func generateFixture(t *testing.T, schemaDir, descriptorSetPath string) map[string][]byte {
	t.Helper()
	outDir := t.TempDir()
	graph, err := entload.LoadGraph(schemaDir, &gen.Config{
		Target:  t.TempDir(),
		Package: "github.com/smintz/entconnect/entc/goldentest/ent",
	})
	if err != nil {
		t.Fatalf("entc.LoadGraph(%q): %v", schemaDir, err)
	}
	if descriptorSetPath == "" {
		descriptorSetPath = testDescriptorSetPath
	}
	ext, err := NewExtension(WithDescriptorSet(descriptorSetPath), WithOutputDir(outDir))
	if err != nil {
		t.Fatalf("NewExtension: %v", err)
	}
	if err := Generate(graph, ext); err != nil {
		t.Fatalf("Generate(%q): %v", schemaDir, err)
	}
	return readDir(t, outDir)
}

func readDir(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	out := make(map[string][]byte, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", e.Name(), err)
		}
		out[e.Name()] = b
	}
	return out
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// goldenName maps an emitted filename to its golden fixture name:
// "<fixture>_<service_snake>" for a per-service file, "<fixture>_server"
// for the combined server file, "<fixture>_claims" for the claims report.
func goldenName(fixture, filename string) string {
	base := strings.TrimSuffix(filename, ".entconnect.go")
	base = strings.TrimSuffix(base, ".txt")
	return fixture + "_" + base
}

// writeEmptyDescriptorSet writes a valid, zero-file FileDescriptorSet
// (proto-marshaled, exactly what LoadDescriptorSet reads) to a temp file
// and returns its path — AllProcedures against it enumerates zero
// procedures, the "empty" case's literal header-only claims report
// precondition.
func writeEmptyDescriptorSet(t *testing.T) string {
	t.Helper()
	b, err := proto.Marshal(&descriptorpb.FileDescriptorSet{})
	if err != nil {
		t.Fatalf("marshal empty FileDescriptorSet: %v", err)
	}
	path := filepath.Join(t.TempDir(), "empty.binpb")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
	return path
}

// TestGolden_EmittedFiles golden-asserts every file Generate emits for
// every fixture schema package this phase ships (CRUD-06): every emitted
// handler file for every verb, plus the claims report, each covered by a
// golden fixture. Regenerate with `go test ./entc/... -update`.
func TestGolden_EmittedFiles(t *testing.T) {
	fixtures := []struct {
		name      string
		schemaDir string
	}{
		{"read", "../internal/entconnecttest/read/ent/schema"},
		{"write", "../internal/entconnecttest/write/ent/schema"},
		{"list", "../internal/entconnecttest/list/ent/schema"},
		{"update", "../internal/entconnecttest/update/ent/schema"},
		{"manual", "../internal/entconnecttest/manual/ent/schema"},
	}

	g := goldie.New(t)
	for _, fx := range fixtures {
		t.Run(fx.name, func(t *testing.T) {
			files := generateFixture(t, fx.schemaDir, "")
			if len(files) == 0 {
				t.Fatalf("want at least one emitted file for %q, got none", fx.name)
			}
			for _, name := range keysOf(files) {
				g.Assert(t, goldenName(fx.name, name), files[name])
			}
		})
	}
}

// TestGolden_Adjacency is CRUD-06's adjacency edge: two DIFFERENT ent
// schemas (AdjacencyArchive, AdjacencyPing) both contribute methods to
// ONE proto service (AdminService). Both methods must land in the single
// emitted file, in ascending method order, with neither schema's
// contribution overwriting the other's.
func TestGolden_Adjacency(t *testing.T) {
	files := generateFixture(t, "../internal/entconnecttest/adjacency/ent/schema", "")
	b, ok := files["admin_service.entconnect.go"]
	if !ok {
		t.Fatalf("want admin_service.entconnect.go to be emitted, got files: %v", keysOf(files))
	}

	archiveIdx := bytes.Index(b, []byte("func (s *adminServiceServer) ArchiveAdmin("))
	pingIdx := bytes.Index(b, []byte("func (s *adminServiceServer) Ping("))
	if archiveIdx < 0 {
		t.Fatalf("want an ArchiveAdmin method in the emitted file, got:\n%s", b)
	}
	if pingIdx < 0 {
		t.Fatalf("want a Ping method in the emitted file, got:\n%s", b)
	}
	if archiveIdx > pingIdx {
		t.Fatalf("want ArchiveAdmin (ascending procedure order) before Ping, got ArchiveAdmin at byte %d, Ping at byte %d", archiveIdx, pingIdx)
	}
	if len(files) != 3 { // admin_service.entconnect.go + claims.txt + server.entconnect.go
		t.Fatalf("want exactly 3 emitted files (one per-service file + claims.txt + server.entconnect.go), got %d: %v", len(files), keysOf(files))
	}

	goldie.New(t).Assert(t, "adjacency_admin_service", b)
}

// TestGolden_Empty is CRUD-06's empty edge, in two parts:
//   - a graph with zero entconnect bindings anywhere emits zero
//     .entconnect.go files, while the claims report is still written —
//     and, against a zero-procedure descriptor set, that report is
//     literally header-only.
//   - a service with exactly one claimed method (the read fixture's
//     OrderReadService/GetOrder) emits exactly one file containing
//     exactly one method.
func TestGolden_Empty(t *testing.T) {
	t.Run("zero bindings emits zero handler files, claims report still written", func(t *testing.T) {
		files := generateFixture(t, "../internal/entconnecttest/empty/ent/schema", writeEmptyDescriptorSet(t))
		for name := range files {
			if strings.HasSuffix(name, ".entconnect.go") {
				t.Fatalf("want zero .entconnect.go files for a zero-binding graph, got %q among: %v", name, keysOf(files))
			}
		}
		claims, ok := files["claims.txt"]
		if !ok {
			t.Fatalf("want claims.txt to exist even with zero bindings, got files: %v", keysOf(files))
		}
		want := "# Code generated by entconnect. DO NOT EDIT.\n"
		if string(claims) != want {
			t.Fatalf("want a literally header-only claims report against a zero-procedure descriptor set, got: %q", string(claims))
		}
	})

	t.Run("a service with exactly one claimed method emits exactly one file with one method", func(t *testing.T) {
		files := generateFixture(t, "../internal/entconnecttest/read/ent/schema", "")
		b, ok := files["order_read_service.entconnect.go"]
		if !ok {
			t.Fatalf("want order_read_service.entconnect.go to be emitted, got files: %v", keysOf(files))
		}
		if n := bytes.Count(b, []byte("func (s *orderReadServiceServer)")); n != 1 {
			t.Fatalf("want exactly 1 method in the single-method service's emitted file, got %d", n)
		}
	})
}

// TestGolden_Ordering is CRUD-06's ordering edge: generating the same
// graph repeatedly stresses every map (services, svcNames, schema
// bindings) extension.go builds along the way — Go deliberately
// randomizes map iteration order per process, so byte-identical output
// across repeated, independent Generate calls is real evidence that
// every one of those maps is walked through an explicit sort (D-20), not
// by accident. Separately asserts the emitted file's import block is
// sorted.
func TestGolden_Ordering(t *testing.T) {
	const runs = 5
	var first []byte
	for i := 0; i < runs; i++ {
		files := generateFixture(t, "../internal/entconnecttest/read/ent/schema", "")
		b, ok := files["order_read_service.entconnect.go"]
		if !ok {
			t.Fatalf("run %d: want order_read_service.entconnect.go to be emitted, got files: %v", i, keysOf(files))
		}
		if i == 0 {
			first = b
			continue
		}
		if !bytes.Equal(b, first) {
			t.Fatalf("run %d produced different bytes than run 0", i)
		}
	}

	groups := importGroups(t, first)
	if len(groups) == 0 {
		t.Fatal("want a non-empty import block to check for sortedness")
	}
	// gofmt/goimports groups imports into blank-line-separated blocks
	// (stdlib, then third-party) and sorts WITHIN each block, never merges
	// the blocks into one global sort — so sortedness is checked
	// group-by-group, matching that real, documented behavior.
	for gi, group := range groups {
		sorted := append([]string(nil), group...)
		sort.Strings(sorted)
		for i := range group {
			if group[i] != sorted[i] {
				t.Fatalf("import group %d not sorted: got %v, want %v", gi, group, sorted)
			}
		}
	}
}

// TestGolden_RepeatStability is CRUD-06's concurrency/repeat-stability
// edge: generating the same graph five times in a row yields
// byte-identical output. The cross-process-run case (identical bytes
// across separate `go test` invocations) is covered by CI's -count=5
// determinism job (Makefile's test-determinism target).
func TestGolden_RepeatStability(t *testing.T) {
	const runs = 5
	var first map[string][]byte
	for i := 0; i < runs; i++ {
		files := generateFixture(t, "../internal/entconnecttest/write/ent/schema", "")
		if i == 0 {
			first = files
			continue
		}
		if len(files) != len(first) {
			t.Fatalf("run %d: want %d emitted files, got %d", i, len(first), len(files))
		}
		for name, b := range first {
			got, ok := files[name]
			if !ok {
				t.Fatalf("run %d: want file %q to be emitted, it was not", i, name)
			}
			if !bytes.Equal(got, b) {
				t.Fatalf("run %d: file %q differs from run 0", i, name)
			}
		}
	}
}

// importGroups extracts the import-path string literals inside src's
// single `import (...)` block, split into blank-line-separated groups (in
// file order, one group per contiguous run of non-blank lines) — the
// shape gofmt/goimports itself produces (stdlib group, then third-party
// group), each independently sorted rather than merged into one global
// sort.
func importGroups(t *testing.T, src []byte) [][]string {
	t.Helper()
	s := string(src)
	start := strings.Index(s, "import (")
	if start < 0 {
		t.Fatalf("want an import ( block in the emitted file, got:\n%s", s)
	}
	end := strings.Index(s[start:], ")")
	if end < 0 {
		t.Fatalf("want a closing ) for the import block, got:\n%s", s)
	}
	block := s[start+len("import (") : start+end]

	var groups [][]string
	var current []string
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(current) > 0 {
				groups = append(groups, current)
				current = nil
			}
			continue
		}
		// Drop a leading alias (e.g. `entconnectruntime "path"`), keeping
		// only the quoted import path itself.
		path := line
		if idx := strings.LastIndex(line, "\""); idx >= 0 {
			if first := strings.Index(line, "\""); first >= 0 && first != idx {
				path = line[first : idx+1]
			}
		}
		current = append(current, path)
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups
}
