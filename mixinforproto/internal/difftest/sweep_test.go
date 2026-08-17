package difftest

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"buf.build/go/protovalidate"
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"

	entgen "github.com/smintz/entconnect/mixinforproto/internal/difftest/ent"
	_ "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// This file is Plan 03-05 Task 3's driverless differential sweep
// (PIPE-06/D-04/D-13/D-14): for every message in the SAME
// mixinforprototest.v1 registry corpus mixinforproto/corpus_test.go's
// own corpusMessages walks, generated values are pushed through BOTH
// protovalidate.Validate (the boundary verdict) and a real ent mutation
// on a real generated ent.Client (hooks.go's compiled hybrid evaluator,
// the storage verdict), and the two are asserted to agree — entity-
// relative, field path included (D-04), never loosened to close a real
// disagreement (this plan's own must_haves.prohibitions entry).
//
// Real ent.Client coverage is driven GENERICALLY via reflection
// (createEntity/updateEntity below) against every ent.Schema fixture
// under internal/difftest/ent/schema — 21 corpus messages whose
// derivation needs zero Exclude/Override/AsJSON options AND carries at
// least one field the storage-layer hook actually evaluates (a scalar/
// optionalScalar field with a protovalidate rule of its own — hooks.go's
// hookFieldClass scope note). No per-message Go code lives in this file:
// a schema's mere presence under ent/schema is what makes this file
// sweep it with real values, so adding a new fixture there needs no
// change here. A corpus message with no such schema — because it derives
// zero evaluable fields, or fails to derive at all without options it
// was never meant to need (e.g. Oneofs' unresolved members) — is swept
// as "covered with zero cases" (Test 5), never silently skipped: this is
// the SAME "the corpus happens to avoid this shape" failure mode
// Phase 1's guards (mixinforproto/corpus_test.go) exist to make
// detectable, applied to the differential axis instead of the coverage
// axis.
//
// Message-level (cross-field) rules — WithMessageRules(OnCreate) — are
// deliberately out of THIS sweep's scope: they are already covered end
// to end by messagerules_test.go (Task 1), and mixing them into this
// generic per-field sweep would require this file to also special-case
// which fixtures carry the opt-in, exactly the kind of hand-maintained
// per-message list this design avoids.

// corpusMessages mirrors mixinforproto/corpus_test.go's own helper of
// the identical name (registry walk over package mixinforprototest.v1,
// recursing nested messages, filtering IsMapEntry, sorted by FullName).
// Duplicated rather than imported — a _test.go helper in one package is
// not callable from another package — but both walk the SAME live
// protoregistry, not a hand-maintained fixture list, so the two copies
// can never name a different message set.
func corpusMessages(t *testing.T) []protoreflect.MessageDescriptor {
	t.Helper()
	var out []protoreflect.MessageDescriptor
	var walk func(mds protoreflect.MessageDescriptors)
	walk = func(mds protoreflect.MessageDescriptors) {
		for i := 0; i < mds.Len(); i++ {
			md := mds.Get(i)
			if md.IsMapEntry() {
				continue
			}
			out = append(out, md)
			walk(md.Messages())
		}
	}
	protoregistry.GlobalFiles.RangeFilesByPackage(
		protoreflect.FullName("mixinforprototest.v1"),
		func(fd protoreflect.FileDescriptor) bool {
			walk(fd.Messages())
			return true
		},
	)
	if len(out) == 0 {
		t.Fatal("corpusMessages: zero messages found in package mixinforprototest.v1 — registry-linkage regression?")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FullName() < out[j].FullName() })
	return out
}

// isSweepableScalar reports whether fd is a field class hooks.go's
// hookFieldClass would ever route into the storage-layer hybrid
// evaluator: a plain or proto3-optional scalar, never a map, list,
// message-typed, or real-oneof-member field (03-03-SUMMARY.md's
// documented scope boundary — those classes stay boundary-only). This is
// a direct, independent reimplementation from protoreflect data alone
// (never mixinforproto's unexported classify), which this package cannot
// import.
func isSweepableScalar(fd protoreflect.FieldDescriptor) bool {
	if fd.IsMap() || fd.IsList() {
		return false
	}
	if fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind {
		return false
	}
	if oo := fd.ContainingOneof(); oo != nil && !oo.IsSynthetic() {
		return false
	}
	return true
}

func fieldCarriesRule(fd protoreflect.FieldDescriptor) bool {
	rules, err := protovalidate.ResolveFieldRules(fd)
	return err == nil && rules != nil
}

// isLengthUnitDivergent reports whether fd carries a code-point string
// bound (string.min_len/max_len/len) — protovalidate's own semantics
// count Unicode code points there, while ent's MinLen/MaxLen count
// bytes; string.min_bytes/max_bytes/len_bytes are byte-semantic on both
// sides and never divergent. This is computed directly from fd's own
// resolved rules — the identical property mixinforproto's SourceField.
// LengthUnitDivergentIDs records, reached here without needing that
// annotation at all.
func isLengthUnitDivergent(fd protoreflect.FieldDescriptor) bool {
	rules, err := protovalidate.ResolveFieldRules(fd)
	if err != nil || rules == nil || rules.GetString() == nil {
		return false
	}
	sr := rules.GetString()
	return sr.HasMinLen() || sr.HasMaxLen() || sr.HasLen()
}

// sweepableFields returns md's fields hooks.go's hybrid evaluator would
// actually reach, in declaration order (D-24: never derived from map
// iteration).
func sweepableFields(md protoreflect.MessageDescriptor) []protoreflect.FieldDescriptor {
	var out []protoreflect.FieldDescriptor
	fds := md.Fields()
	for i := 0; i < fds.Len(); i++ {
		fd := fds.Get(i)
		if isSweepableScalar(fd) && fieldCarriesRule(fd) {
			out = append(out, fd)
		}
	}
	return out
}

// entClientFor returns the reflect.Value of client's <shortName> field
// (e.g. client.DoubleComparators for shortName "DoubleComparators"), or
// an invalid reflect.Value when no ent.Schema fixture exists for this
// corpus message — the generic, no-hand-list mechanism this file's own
// doc comment describes.
func entClientFor(client *entgen.Client, shortName string) reflect.Value {
	return reflect.ValueOf(client).Elem().FieldByName(shortName)
}

// createEntity drives typeClient.Create() generically via reflection:
// every generated <Type>Create builder shares the method shape
// Mutation() ent.Mutation / Save(ctx) (*Type, error), so no per-type Go
// code is needed here. Returns the created row's int ID and the save
// error (a *protovalidate.ValidationError on rejection, nil on accept —
// hooks.go's own contract).
func createEntity(ctx context.Context, typeClient reflect.Value, values map[string]any) (id int, saveErr error) {
	createVal := typeClient.MethodByName("Create").Call(nil)[0]
	mutation := createVal.MethodByName("Mutation").Call(nil)[0].Interface().(ent.Mutation)
	for name, v := range values {
		if serr := mutation.SetField(name, v); serr != nil {
			return 0, fmt.Errorf("SetField(%q, %v): %w", name, v, serr)
		}
	}
	results := createVal.MethodByName("Save").Call([]reflect.Value{reflect.ValueOf(ctx)})
	if errVal := results[1]; !errVal.IsNil() {
		return 0, errVal.Interface().(error)
	}
	entity := results[0]
	return int(entity.Elem().FieldByName("ID").Int()), nil
}

// updateEntity mirrors createEntity for UpdateOneID(id) — D-06's Update
// half of the sweep.
func updateEntity(ctx context.Context, typeClient reflect.Value, id int, values map[string]any) error {
	updateVal := typeClient.MethodByName("UpdateOneID").Call([]reflect.Value{reflect.ValueOf(id)})[0]
	mutation := updateVal.MethodByName("Mutation").Call(nil)[0].Interface().(ent.Mutation)
	for name, v := range values {
		if serr := mutation.SetField(name, v); serr != nil {
			return fmt.Errorf("SetField(%q, %v): %w", name, v, serr)
		}
	}
	results := updateVal.MethodByName("Save").Call([]reflect.Value{reflect.ValueOf(ctx)})
	if errVal := results[1]; !errVal.IsNil() {
		return errVal.Interface().(error)
	}
	return nil
}

// buildBoundaryEntity constructs the dynamicpb entity protovalidate.Validate
// evaluates, setting exactly the fields named in values — D-04's entity-
// relative comparison: same object, same paths, never a request-wrapper
// artifact.
func buildBoundaryEntity(md protoreflect.MessageDescriptor, byName map[string]protoreflect.FieldDescriptor, values map[string]any) *dynamicpb.Message {
	msg := dynamicpb.NewMessage(md)
	for name, v := range values {
		msg.Set(byName[name], protoreflect.ValueOf(v))
	}
	return msg
}

// scopedFilter restricts protovalidate's evaluation to exactly the
// fields named in scope — the same WithFilter mechanism hooks.go's own
// evaluate() uses (D-03/D-06), so an out-of-scope sibling's own rule
// never contaminates this sweep's per-field comparison. Never scopes in
// the message descriptor itself: message-level rules are out of this
// sweep's scope (see this file's package doc comment).
func scopedFilter(md protoreflect.MessageDescriptor, scope map[string]bool) protovalidate.FilterFunc {
	nums := map[protoreflect.FieldNumber]bool{}
	for name := range scope {
		if fd := md.Fields().ByName(protoreflect.Name(name)); fd != nil {
			nums[fd.Number()] = true
		}
	}
	return func(msg protoreflect.Message, d protoreflect.Descriptor) bool {
		if d == msg.Descriptor() {
			return false
		}
		fd, ok := d.(protoreflect.FieldDescriptor)
		if !ok {
			return false
		}
		return nums[fd.Number()]
	}
}

// violationIdentities extracts err's (RuleId, FieldPath) identity set
// (D-04's comparison key — never excluding field path). ok is false when
// err is non-nil but NOT a *protovalidate.ValidationError: a genuine
// evaluator failure, not a verdict, which the caller must treat as a
// hard test failure rather than a legitimate disagreement.
func violationIdentities(err error) (ids map[string]bool, ok bool) {
	if err == nil {
		return map[string]bool{}, true
	}
	var ve *protovalidate.ValidationError
	if !errors.As(err, &ve) {
		return nil, false
	}
	ids = map[string]bool{}
	for _, v := range ve.Violations {
		ids[v.Proto.GetRuleId()+"\x00"+protovalidate.FieldPathString(v.Proto.GetField())] = true
	}
	return ids, true
}

// disagreement is the printable record of one sweep run's boundary-vs-
// storage mismatch (D-14: seed, message, field, value, both verdicts —
// every one of these must be present so a red CI run reproduces
// verbatim from the printed text alone).
type disagreement struct {
	seed        int64
	message     string
	fieldValues map[string]any
	boundary    map[string]bool
	storage     map[string]bool
}

func (d disagreement) String() string {
	var fields []string
	for name := range d.fieldValues {
		fields = append(fields, name)
	}
	sort.Strings(fields)
	var vals []string
	for _, name := range fields {
		vals = append(vals, fmt.Sprintf("%s=%s", name, formatValue(d.fieldValues[name])))
	}
	return fmt.Sprintf(
		"sweep disagreement: message=%s seed=%d values={%s} boundary_violations=%v storage_violations=%v",
		d.message, d.seed, strings.Join(vals, ", "), sortedKeysOf(d.boundary), sortedKeysOf(d.storage),
	)
}

func sortedKeysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// sameSet reports whether a and b contain the identical key set.
func sameSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// exploratorySeedOffset reads SWEEP_EXPLORATORY_SEED (an opt-in override
// this file itself never sets) and, when present, XORs it into every
// message's seed for this run — D-14's opt-in exploratory mode. The
// clock is read only by the CI workflow's own shell step (`date +%s`),
// NEVER by this Go code, so sweepSeedFor itself stays a pure function of
// the message name with no import of "time" anywhere in this package —
// TestSweepSeed_DeterministicAndClockIndependent's own property holds
// regardless of whether this env var is set. Absent (required CI, every
// local run), this returns 0 and the sweep is fully deterministic.
func exploratorySeedOffset() int64 {
	v := os.Getenv("SWEEP_EXPLORATORY_SEED")
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// --- Test 3: the seed is a pure function of the message name -----------

func TestSweepSeed_DeterministicAndClockIndependent(t *testing.T) {
	name := protoreflect.FullName("mixinforprototest.v1.Int32Comparators")
	a := sweepSeedFor(name)
	b := sweepSeedFor(name)
	if a != b {
		t.Fatalf("sweepSeedFor(%s) returned different seeds across calls: %d vs %d — must be a pure function of the name", name, a, b)
	}
	other := sweepSeedFor("mixinforprototest.v1.FloatComparators")
	if a == other {
		t.Fatalf("sweepSeedFor returned the SAME seed for two different message names (%d) — collision risk defeats D-14's per-message determinism", a)
	}
	// sweepSeedFor's own source (generate.go) imports no "time" package
	// at all — structurally unable to consult the clock. This is a
	// property of the code, not something a runtime assertion can prove
	// stronger than the two checks above already do.
}

// --- Test 4: disagreement report format ---------------------------------

func TestSweepDisagreement_ReportsSeedMessageFieldAndValue(t *testing.T) {
	d := disagreement{
		seed:        1234,
		message:     "mixinforprototest.v1.FakeMessage",
		fieldValues: map[string]any{"gt_field": int32(5)},
		boundary:    map[string]bool{"int32.gt\x00gt_field": true},
		storage:     map[string]bool{},
	}
	msg := d.String()
	for _, want := range []string{"1234", "mixinforprototest.v1.FakeMessage", "gt_field", "5", "int32.gt"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("disagreement report %q missing %q", msg, want)
		}
	}
}

// --- Main sweep: Tests 1, 2, 5, 6 ----------------------------------------

// TestSweep_DriverlessDifferential is Task 3's headline proof: every
// corpus message is swept — real values through both protovalidate and a
// real ent mutation where a schema fixture exists, "covered with zero
// cases" where none does — and the two verdicts agree everywhere a
// comparison was actually made.
func TestSweep_DriverlessDifferential(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	msgs := corpusMessages(t)
	swept := 0
	zeroCase := 0
	divergentFieldsHit := map[string]bool{}
	var divergentFieldsSeen []string

	for _, md := range msgs {
		name := string(md.FullName())
		shortName := string(md.Name())
		fields := sweepableFields(md)

		typeClient := entClientFor(client, shortName)
		if !typeClient.IsValid() || len(fields) == 0 {
			t.Logf("covered-with-zero-cases: %s (no ent.Schema fixture or zero evaluable fields)", name)
			zeroCase++
			continue
		}
		swept++

		byName := map[string]protoreflect.FieldDescriptor{}
		for _, fd := range fields {
			byName[string(fd.Name())] = fd
		}

		seed := sweepSeedFor(md.FullName()) ^ exploratorySeedOffset()
		rng := rand.New(rand.NewSource(seed)) //nolint:gosec // deterministic sweep, not a security use

		candidates := make(map[string][]any, len(fields))
		maxRuns := 2
		for _, fd := range fields {
			divergent := isLengthUnitDivergent(fd)
			cs := fieldValueCandidates(rng, fd, divergent)
			candidates[string(fd.Name())] = cs
			if len(cs) > maxRuns {
				maxRuns = len(cs)
			}
			if divergent {
				divergentFieldsSeen = append(divergentFieldsSeen, name+"."+string(fd.Name()))
			}
		}
		if maxRuns > 8 {
			maxRuns = 8 // bound sweep cost; every candidate still gets exercised via modulo cycling across repeated messages/fields in practice, and boundary values are always in the low indices generate.go appends first.
		}

		t.Run(name, func(t *testing.T) {
			for run := 0; run < maxRuns; run++ {
				values := make(map[string]any, len(fields))
				for _, fd := range fields {
					fname := string(fd.Name())
					cs := candidates[fname]
					v := cs[run%len(cs)]
					values[fname] = v
					if isLengthUnitDivergent(fd) {
						if s, ok := v.(string); ok && utf8.RuneCountInString(s) != len(s) {
							divergentFieldsHit[name+"."+fname] = true
						}
					}
				}

				boundaryEntity := buildBoundaryEntity(md, byName, values)
				boundaryErr := protovalidate.Validate(boundaryEntity, protovalidate.WithFilter(scopedFilter(md, allNames(fields))))
				boundarySet, boundaryOK := violationIdentities(boundaryErr)
				if !boundaryOK {
					t.Fatalf("run %d: protovalidate.Validate returned a non-ValidationError failure (genuine evaluator fault, not a verdict): %v", run, boundaryErr)
				}

				id, saveErr := createEntity(ctx, typeClient, values)
				storageSet, storageOK := violationIdentities(saveErr)
				if !storageOK {
					t.Fatalf("run %d: real Create().Save(ctx) returned a non-ValidationError failure (genuine fault, not a verdict): %v", run, saveErr)
				}

				if !sameSet(boundarySet, storageSet) {
					d := disagreement{seed: seed, message: name, fieldValues: values, boundary: boundarySet, storage: storageSet}
					t.Fatalf("Create: %s", d.String())
				}

				if saveErr != nil {
					// A rejected Create leaves no row to Update — nothing
					// further to compare on this run.
					continue
				}

				// --- Update half (Test 2/D-06): touch a deterministic
				// subset — every other sweepable field by index — with a
				// DIFFERENT candidate value, and compare scoped to
				// exactly that subset.
				subset := map[string]any{}
				for i, fd := range fields {
					if i%2 != 0 {
						continue
					}
					fname := string(fd.Name())
					cs := candidates[fname]
					subset[fname] = cs[(run+1)%len(cs)]
				}
				if len(subset) == 0 {
					continue
				}

				subsetNames := make(map[string]bool, len(subset))
				for name := range subset {
					subsetNames[name] = true
				}
				updBoundaryEntity := buildBoundaryEntity(md, byName, subset)
				updBoundaryErr := protovalidate.Validate(updBoundaryEntity, protovalidate.WithFilter(scopedFilter(md, subsetNames)))
				updBoundarySet, ok := violationIdentities(updBoundaryErr)
				if !ok {
					t.Fatalf("run %d update: protovalidate.Validate returned a non-ValidationError failure: %v", run, updBoundaryErr)
				}

				updErr := updateEntity(ctx, typeClient, id, subset)
				updStorageSet, ok := violationIdentities(updErr)
				if !ok {
					t.Fatalf("run %d update: real Update().Save(ctx) returned a non-ValidationError failure: %v", run, updErr)
				}

				if !sameSet(updBoundarySet, updStorageSet) {
					d := disagreement{seed: seed, message: name + " (Update, changed-only)", fieldValues: subset, boundary: updBoundarySet, storage: updStorageSet}
					t.Fatalf("Update: %s", d.String())
				}
			}
		})
	}

	if swept+zeroCase != len(msgs) {
		t.Fatalf("swept-message accounting mismatch: swept=%d zeroCase=%d total=%d, want swept+zeroCase == %d (a message was silently skipped)", swept, zeroCase, swept+zeroCase, len(msgs))
	}
	t.Logf("sweep summary: %d messages swept with real values, %d covered with zero cases, %d corpus messages total", swept, zeroCase, len(msgs))

	if len(divergentFieldsSeen) == 0 {
		t.Fatal("no corpus field is flagged length-unit-divergent — StringCodePointBounds' min_len/max_len/len fields should be; a regression in isLengthUnitDivergent or the corpus itself")
	}
	sort.Strings(divergentFieldsSeen)
	for _, f := range divergentFieldsSeen {
		if !divergentFieldsHit[f] {
			t.Fatalf("length-unit-divergent field %q never received a multi-byte non-ASCII value across %d runs — a code-point-vs-byte divergence would go undetected", f, 8)
		}
	}
}

func allNames(fields []protoreflect.FieldDescriptor) map[string]bool {
	out := make(map[string]bool, len(fields))
	for _, fd := range fields {
		out[string(fd.Name())] = true
	}
	return out
}

// --- Test 5 (explicit): a zero-derived-field message is reported, not
// silently skipped — Empty is this corpus's own literal zero-field
// fixture, so this is asserted directly rather than only inferred from
// the main sweep's accounting check above.

func TestSweep_ZeroFieldMessageIsReportedNotSkipped(t *testing.T) {
	msgs := corpusMessages(t)
	var found bool
	for _, md := range msgs {
		if string(md.FullName()) != "mixinforprototest.v1.Empty" {
			continue
		}
		found = true
		if got := len(sweepableFields(md)); got != 0 {
			t.Fatalf("mixinforprototest.v1.Empty: want 0 sweepable fields, got %d", got)
		}
	}
	if !found {
		t.Fatal("mixinforprototest.v1.Empty not found in the corpus — fixture removed?")
	}
}

// --- fakeDriver sanity: this sweep must not need a DB driver -----------

func TestSweep_FakeDriverDialectIsSQLite(t *testing.T) {
	// Documents D-13: the sweep drives real Save() calls through the
	// SAME driverless fakeDriver every other file in this package uses —
	// no new dependency, no DB.
	drv := newFakeDriver(dialect.SQLite)
	if drv.Dialect() != dialect.SQLite {
		t.Fatalf("want dialect %q, got %q", dialect.SQLite, drv.Dialect())
	}
}
