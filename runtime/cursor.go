package runtime

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// Cursor is the decoded form of an opaque page_token (D-08): the
// ordering-tuple values (as strings, one per tuple column, in tuple
// order) the next page's keyset predicate resumes from, plus the
// fingerprint of the ordering fields the token was minted against — a
// page_token whose fingerprint disagrees with the current request fails
// loudly (ErrCursorMismatch) rather than returning a silently
// inconsistent page.
//
// Deliberate scope decision, carried from RESEARCH.md Assumption A1 /
// Open Question 1 (a conscious choice, not an oversight): this cursor is
// NOT cryptographically signed in this phase. AIP-158 itself requires
// only opacity, not integrity, and every paged query still runs through
// ent's own privacy.Filter rules — a forged cursor (a client hand-
// constructing a base64 JSON payload with a valid fingerprint) cannot
// bypass authorization, because the resulting Where predicate is
// evaluated inside the exact same query privacy rules already apply to.
// The residual exposure is narrow: a forged cursor can only probe for
// the *existence* of rows at arbitrary ordering-tuple positions the
// privacy policy would otherwise allow the caller to see anyway (it
// cannot reveal rows the caller isn't authorized to query at all, and it
// cannot mutate data). HMAC-signing the cursor is a recorded, deferrable
// hardening option — not required by AIP-158, and not implemented here.
type Cursor struct {
	Fingerprint string   `json:"f"`
	Keys        []string `json:"k"`
}

// ErrCursorMismatch is returned by DecodeCursor when a token's embedded
// fingerprint disagrees with the fingerprint computed from the current
// request's ordering fields (D-08) — the token was minted for a
// different ordering/filter shape than the request now asks for, and
// resuming from it would silently skip or repeat rows.
var ErrCursorMismatch = errors.New("entconnect: page_token fingerprint does not match this request's ordering fields")

// ErrCursorMalformed is returned by DecodeCursor when token cannot be
// decoded at all (not valid base64, or the decoded bytes are not the
// expected JSON shape) — a truncated, corrupted, or hand-crafted-wrong
// token, not a staleness condition.
var ErrCursorMalformed = errors.New("entconnect: page_token is malformed")

// Fingerprint returns a deterministic hex-encoded SHA-256 digest over
// parts, each length-prefixed before hashing so that no concatenation of
// adjacent parts can collide across different part boundaries — e.g.
// Fingerprint("ab", "c") and Fingerprint("a", "bc") are guaranteed
// distinct even though "ab"+"c" == "a"+"bc" as plain concatenation.
func Fingerprint(parts ...string) string {
	h := sha256.New()
	var lenBuf [8]byte
	for _, p := range parts {
		binary.BigEndian.PutUint64(lenBuf[:], uint64(len(p)))
		h.Write(lenBuf[:])
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// EncodeCursor builds an opaque page_token embedding fingerprint (D-08)
// and the ordering-tuple values keys, in tuple order.
func EncodeCursor(fingerprint string, keys ...string) string {
	c := Cursor{Fingerprint: fingerprint, Keys: keys}
	// Cursor is a plain struct of strings — json.Marshal never fails for
	// this shape.
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor decodes token and verifies its embedded fingerprint
// matches wantFingerprint (the fingerprint computed from the current
// request's ordering fields). Returns ErrCursorMalformed for anything
// that is not decodable (truncated, non-base64, or non-JSON-shaped
// input) and ErrCursorMismatch when the embedded fingerprint differs
// from wantFingerprint. Both are package-level sentinel errors — callers
// match with errors.Is, never a type assertion.
func DecodeCursor(token, wantFingerprint string) (Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Cursor{}, fmt.Errorf("%w: %v", ErrCursorMalformed, err)
	}
	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return Cursor{}, fmt.Errorf("%w: %v", ErrCursorMalformed, err)
	}
	if c.Fingerprint != wantFingerprint {
		return Cursor{}, ErrCursorMismatch
	}
	return c, nil
}
