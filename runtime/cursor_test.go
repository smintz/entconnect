package runtime

import (
	"errors"
	"testing"
)

func TestCursorFingerprint_LengthPrefixCollisionResistance(t *testing.T) {
	// "ab"+"c" == "a"+"bc" as plain concatenation — the length-prefix
	// discipline must keep these distinct.
	a := Fingerprint("ab", "c")
	b := Fingerprint("a", "bc")
	if a == b {
		t.Fatalf("Fingerprint(\"ab\",\"c\") == Fingerprint(\"a\",\"bc\") == %q — length-prefix collision", a)
	}
}

func TestCursorFingerprint_Deterministic(t *testing.T) {
	a := Fingerprint("created_at", "id")
	b := Fingerprint("created_at", "id")
	if a != b {
		t.Fatalf("Fingerprint is not deterministic: %q != %q", a, b)
	}
	if Fingerprint("created_at", "id") == Fingerprint("id", "created_at") {
		t.Fatal("Fingerprint must be order-sensitive: swapping part order produced the same digest")
	}
}

func TestCursor_RoundTrip(t *testing.T) {
	fp := Fingerprint("created_at", "id")
	token := EncodeCursor(fp, "2026-08-08T00:00:00Z", "42")

	got, err := DecodeCursor(token, fp)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if got.Fingerprint != fp {
		t.Fatalf("want fingerprint %q, got %q", fp, got.Fingerprint)
	}
	if len(got.Keys) != 2 || got.Keys[0] != "2026-08-08T00:00:00Z" || got.Keys[1] != "42" {
		t.Fatalf("want keys [2026-08-08T00:00:00Z 42], got %v", got.Keys)
	}
}

func TestCursor_FingerprintMismatch(t *testing.T) {
	token := EncodeCursor(Fingerprint("created_at", "id"), "2026-08-08T00:00:00Z", "42")

	_, err := DecodeCursor(token, Fingerprint("status", "id"))
	if err == nil {
		t.Fatal("want an error for a mismatched fingerprint, got nil")
	}
	if !errors.Is(err, ErrCursorMismatch) {
		t.Fatalf("want errors.Is(err, ErrCursorMismatch), got %v", err)
	}
}

func TestCursor_MalformedTruncated(t *testing.T) {
	token := EncodeCursor(Fingerprint("created_at"), "2026-08-08T00:00:00Z")
	truncated := token[:len(token)/2]

	_, err := DecodeCursor(truncated, Fingerprint("created_at"))
	if err == nil {
		t.Fatal("want an error for a truncated token, got nil")
	}
	if !errors.Is(err, ErrCursorMalformed) {
		t.Fatalf("want errors.Is(err, ErrCursorMalformed), got %v", err)
	}
}

func TestCursor_MalformedNonBase64(t *testing.T) {
	_, err := DecodeCursor("not-valid-base64!!!###", Fingerprint("created_at"))
	if err == nil {
		t.Fatal("want an error for non-base64 input, got nil")
	}
	if !errors.Is(err, ErrCursorMalformed) {
		t.Fatalf("want errors.Is(err, ErrCursorMalformed), got %v", err)
	}
}

func TestCursor_MalformedValidBase64NotJSON(t *testing.T) {
	// Valid RawURLEncoding base64 whose decoded bytes are not the expected
	// JSON shape at all.
	const notJSON = "bm90LWpzb24" // base64.RawURLEncoding("not-json")

	_, err := DecodeCursor(notJSON, Fingerprint("created_at"))
	if err == nil {
		t.Fatal("want an error for non-JSON decoded content, got nil")
	}
	if !errors.Is(err, ErrCursorMalformed) {
		t.Fatalf("want errors.Is(err, ErrCursorMalformed), got %v", err)
	}
}
