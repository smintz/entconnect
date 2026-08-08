package mixinforproto

import (
	"fmt"
	"strings"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"

	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// failureCase is one row of TestSchemaLoadFailures: a pair of closures
// exercising the exact same failure through both of mixinforproto's two
// surfaces (D-06) — the panicking ent.Mixin.Fields() adapter and the
// error-returning Validate[M] debug entry point (D-07) — plus the
// substrings the rendered message's first line must contain (D-25).
type failureCase struct {
	name            string
	fields          func() []ent.Field // wraps MixinForProto[M](opts...).Fields(); expected to panic
	validate        func() error       // wraps Validate[M](opts...); expected to return a non-nil error
	wantInFirstLine []string
}

// recoverPanic runs fn and reports whether it panicked, plus the
// panic value rendered as a string via error.Error() when the recovered
// value is an error (which every mixinforproto failure path produces).
func recoverPanic(fn func() []ent.Field) (panicked bool, msg string) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			if err, ok := r.(error); ok {
				msg = err.Error()
			} else {
				msg = fmt.Sprint(r)
			}
		}
	}()
	fn()
	return false, ""
}

// assertFirstLineSelfSufficient is the shared helper every row in
// TestSchemaLoadFailures uses (per this plan's Task 3 action text): it
// splits msg on the first newline and asserts that the first line alone
// — not the message as a whole — contains every required substring.
// This encodes the subprocess-truncation constraint (D-08) as a test,
// not a convention: a future change that moves detail to line two fails
// here, not silently.
func assertFirstLineSelfSufficient(t *testing.T, msg string, want []string) {
	t.Helper()
	first, _, _ := strings.Cut(msg, "\n")
	if first == "" {
		t.Fatal("want a non-empty first line")
	}
	if strings.Contains(first, "\n") {
		t.Fatalf("first line must not itself contain a newline: %q", first)
	}
	for _, w := range want {
		if !strings.Contains(first, w) {
			t.Fatalf("first line %q missing required substring %q (full message: %q)", first, w, msg)
		}
	}
}

// runFailureCase proves all three things D-25 exists to force for a
// single failure path:
//  1. The failure fires at schema load — Fields() panics — not later at
//     first mutation.
//  2. The panic value's message contains the specific expected content
//     (asserted via assertFirstLineSelfSufficient, not merely "it
//     panicked" — a bare Go runtime panic would fail this).
//  3. Validate[M] returns a non-nil error whose message is
//     byte-identical to the panic message, reached without invoking
//     go generate, entc.Generate, or entc.LoadGraph anywhere (MIX-12/
//     D-06/D-07 parity: one core, two surfaces).
func runFailureCase(t *testing.T, c failureCase) {
	t.Run(c.name, func(t *testing.T) {
		panicked, panicMsg := recoverPanic(c.fields)
		if !panicked {
			t.Fatal("want Fields() to panic at schema load")
		}
		assertFirstLineSelfSufficient(t, panicMsg, c.wantInFirstLine)

		err := c.validate()
		if err == nil {
			t.Fatal("want Validate[M] to return a non-nil error")
		}
		if err.Error() != panicMsg {
			t.Fatalf(
				"Validate[M] message is not byte-identical to the panic message:\nValidate[M]: %q\npanic:       %q",
				err.Error(), panicMsg,
			)
		}
	})
}

// TestSchemaLoadFailures is the single table-driven suite proving every
// failure path this phase can produce fires at schema load with
// specific, self-sufficient message content, and reproduces
// byte-identically in-process through Validate[M] (D-25). No row here
// covers "an unsupported proto kind" (mentioned as a possibility in
// this plan's action text) because fieldmap.go's scalar-kind switch is
// exhaustive over everything proto3 syntax can express (Plan 02);
// see 01-02-SUMMARY.md's TestDerive_FormerlyUnsupportedKindNowMaps.
func TestSchemaLoadFailures(t *testing.T) {
	runFailureCase(t, failureCase{
		name: "unknown Exclude name",
		fields: func() []ent.Field {
			return MixinForProto[*mixinforprototestv1.Scalars](Exclude("does_not_exist")).Fields()
		},
		validate: func() error {
			return Validate[*mixinforprototestv1.Scalars](Exclude("does_not_exist"))
		},
		wantInFirstLine: []string{"mixinforprototest.v1.Scalars", "does_not_exist", "Exclude"},
	})

	runFailureCase(t, failureCase{
		name: "unknown Override name",
		fields: func() []ent.Field {
			return MixinForProto[*mixinforprototestv1.Scalars](Override("does_not_exist", field.String("does_not_exist"))).Fields()
		},
		validate: func() error {
			return Validate[*mixinforprototestv1.Scalars](Override("does_not_exist", field.String("does_not_exist")))
		},
		wantInFirstLine: []string{"mixinforprototest.v1.Scalars", "does_not_exist", "Override"},
	})

	runFailureCase(t, failureCase{
		name: "unknown AsJSON name",
		fields: func() []ent.Field {
			return MixinForProto[*mixinforprototestv1.Messages](AsJSON("does_not_exist")).Fields()
		},
		validate: func() error {
			return Validate[*mixinforprototestv1.Messages](AsJSON("does_not_exist"))
		},
		wantInFirstLine: []string{"mixinforprototest.v1.Messages", "does_not_exist", "AsJSON"},
	})

	runFailureCase(t, failureCase{
		name: "empty-string Exclude name",
		fields: func() []ent.Field {
			return MixinForProto[*mixinforprototestv1.Scalars](Exclude("")).Fields()
		},
		validate: func() error {
			return Validate[*mixinforprototestv1.Scalars](Exclude(""))
		},
		wantInFirstLine: []string{"mixinforprototest.v1.Scalars", "Exclude"},
	})

	runFailureCase(t, failureCase{
		name: "nil Override field",
		fields: func() []ent.Field {
			return MixinForProto[*mixinforprototestv1.Tracer](Override("name", nil)).Fields()
		},
		validate: func() error {
			return Validate[*mixinforprototestv1.Tracer](Override("name", nil))
		},
		wantInFirstLine: []string{"mixinforprototest.v1.Tracer", "name", "Override", "nil"},
	})

	runFailureCase(t, failureCase{
		name: "Exclude/Override conflict",
		fields: func() []ent.Field {
			return MixinForProto[*mixinforprototestv1.Scalars](
				Exclude("string_field"),
				Override("string_field", field.String("string_field")),
			).Fields()
		},
		validate: func() error {
			return Validate[*mixinforprototestv1.Scalars](
				Exclude("string_field"),
				Override("string_field", field.String("string_field")),
			)
		},
		wantInFirstLine: []string{"mixinforprototest.v1.Scalars", "string_field", "Exclude", "Override"},
	})

	runFailureCase(t, failureCase{
		name: "AsJSON on a scalar field",
		fields: func() []ent.Field {
			return MixinForProto[*mixinforprototestv1.Presence](AsJSON("plain_string")).Fields()
		},
		validate: func() error {
			return Validate[*mixinforprototestv1.Presence](AsJSON("plain_string"))
		},
		wantInFirstLine: []string{"mixinforprototest.v1.Presence", "plain_string", "AsJSON"},
	})

	runFailureCase(t, failureCase{
		name: "reserved static identifier collision",
		fields: func() []ent.Field {
			return MixinForProto[*mixinforprototestv1.ReservedStatic]().Fields()
		},
		validate: func() error {
			return Validate[*mixinforprototestv1.ReservedStatic]()
		},
		wantInFirstLine: []string{"mixinforprototest.v1.ReservedStatic", "config", "Exclude", "Override"},
	})

	runFailureCase(t, failureCase{
		name: "reserved structural identifier collision",
		fields: func() []ent.Field {
			return MixinForProto[*mixinforprototestv1.ReservedStructural]().Fields()
		},
		validate: func() error {
			return Validate[*mixinforprototestv1.ReservedStructural]()
		},
		wantInFirstLine: []string{"mixinforprototest.v1.ReservedStructural", "id", "Exclude", "Override"},
	})

	runFailureCase(t, failureCase{
		name: "unresolved oneof",
		fields: func() []ent.Field {
			return MixinForProto[*mixinforprototestv1.PartialOneof]().Fields()
		},
		validate: func() error {
			return Validate[*mixinforprototestv1.PartialOneof]()
		},
		wantInFirstLine: []string{"mixinforprototest.v1.PartialOneof", "choice", "alpha", "bravo", "charlie", "Exclude", "Override"},
	})

	runFailureCase(t, failureCase{
		name: "partially resolved oneof",
		fields: func() []ent.Field {
			return MixinForProto[*mixinforprototestv1.PartialOneof](Exclude("alpha")).Fields()
		},
		validate: func() error {
			return Validate[*mixinforprototestv1.PartialOneof](Exclude("alpha"))
		},
		wantInFirstLine: []string{"mixinforprototest.v1.PartialOneof", "choice", "bravo", "charlie"},
	})
}

// TestSchemaLoadFailures_SuccessPathDoesNotPanic is the MIX-11 empty
// edge (see this plan's must_haves): a clean derivation panics not at
// all, and Validate[M] returns exactly nil — the success path is not an
// error path carrying an empty list.
func TestSchemaLoadFailures_SuccessPathDoesNotPanic(t *testing.T) {
	panicked, msg := recoverPanic(func() []ent.Field {
		return MixinForProto[*mixinforprototestv1.Scalars]().Fields()
	})
	if panicked {
		t.Fatalf("want no panic for a clean derivation, got: %s", msg)
	}

	if err := Validate[*mixinforprototestv1.Scalars](); err != nil {
		t.Fatalf("want a nil error from Validate[M] for a clean derivation, got: %v", err)
	}
}
