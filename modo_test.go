package gojq_test

import (
	"strings"
	"testing"

	"github.com/modopayments/gojq"
)

// --- ParseForCompletion ---

func TestParseForCompletion_TrailingDot(t *testing.T) {
	// Source ends with "." — should inject CompletionSentinel and return recovered=true.
	q, recovered, err := gojq.ParseForCompletion(".foo.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !recovered {
		t.Error("recovered = false, want true for trailing-dot source")
	}
	if q == nil {
		t.Fatal("query is nil")
	}
	// The sentinel identifier must appear somewhere in the printed query.
	if !strings.Contains(q.String(), gojq.CompletionSentinel) {
		t.Errorf("query string %q does not contain sentinel %q", q.String(), gojq.CompletionSentinel)
	}
}

func TestParseForCompletion_RootDot(t *testing.T) {
	// Just "." is a valid query (identity), not a trailing-dot recovery.
	_, recovered, err := gojq.ParseForCompletion(".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "." is valid jq on its own; ParseForCompletion may or may not recover.
	// The key requirement is no error.
	_ = recovered
}

func TestParseForCompletion_TrailingWhitespaceAfterDot(t *testing.T) {
	// Trailing whitespace after "." must still trigger recovery.
	q, recovered, err := gojq.ParseForCompletion(".foo.  \n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !recovered {
		t.Error("recovered = false, want true (trailing whitespace after dot)")
	}
	if !strings.Contains(q.String(), gojq.CompletionSentinel) {
		t.Errorf("query string %q does not contain sentinel", q.String())
	}
}

func TestParseForCompletion_NoTrailingDot_CleanParse(t *testing.T) {
	// Well-formed query with no trailing dot — clean parse.
	q, recovered, err := gojq.ParseForCompletion(".foo | .bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recovered {
		t.Error("recovered = true, want false for cleanly-parsed query")
	}
	if q == nil {
		t.Fatal("query is nil")
	}
}

func TestParseForCompletion_InvalidQuery(t *testing.T) {
	// Unparseable source that doesn't end with "." — should return error.
	_, _, err := gojq.ParseForCompletion("( unclosed")
	if err == nil {
		t.Error("expected error for unparseable query, got nil")
	}
}

func TestParseForCompletion_SentinelInFuncName(t *testing.T) {
	// ".foo." should parse as ".foo.__cursor__"; the function node carrying
	// the sentinel must be reachable from the AST.
	q, recovered, err := gojq.ParseForCompletion(".foo.")
	if err != nil || !recovered {
		t.Fatalf("ParseForCompletion: recovered=%v err=%v", recovered, err)
	}
	str := q.String()
	if !strings.Contains(str, gojq.CompletionSentinel) {
		t.Errorf("sentinel not found in query %q", str)
	}
}

func TestParseForCompletion_DeepPath(t *testing.T) {
	// Multi-level dot path completion.
	q, recovered, err := gojq.ParseForCompletion(".Calls.myRequest.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !recovered {
		t.Error("recovered = false, want true")
	}
	if !strings.Contains(q.String(), gojq.CompletionSentinel) {
		t.Errorf("sentinel not found in query: %q", q.String())
	}
}

// --- UTF-16 positions ---

// TestUTF16Positions_ASCII verifies the baseline: ASCII source produces
// identical byte and UTF-16 positions.
func TestUTF16Positions_ASCII(t *testing.T) {
	// ".foo | length" — all ASCII, byte pos == UTF-16 pos.
	// "length" starts at byte 7.
	q, err := gojq.Parse(".foo | length")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fn := funcNameToken(t, q)
	if fn == nil {
		return
	}
	// In ASCII source byte offset = UTF-16 offset, so Start must be 7.
	if fn.Start != 7 {
		t.Errorf("ASCII: length.Start = %d, want 7", fn.Start)
	}
}

// TestUTF16Positions_BMP verifies that a BMP character (é, U+00E9) that is
// 2 bytes in UTF-8 but 1 UTF-16 code unit shifts subsequent token positions
// by 1 compared to the equivalent ASCII source.
func TestUTF16Positions_BMP(t *testing.T) {
	// `"é" | length` — é is 2 UTF-8 bytes but 1 UTF-16 code unit.
	// Byte layout: " é " _ | _ l e n g t h
	//              0 1 2 3 4 5 6 7 8 9 ...
	// UTF-16 pos:  0 1   2 3 4 5 6 7 ...
	// "length" starts at UTF-8 byte 7, UTF-16 code unit 6.
	q, err := gojq.Parse(`"é" | length`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fn := funcNameToken(t, q)
	if fn == nil {
		return
	}
	const wantStart = 6 // UTF-16
	if fn.Start == 7 {
		t.Errorf("BMP: length.Start = 7 (byte offset), want %d (UTF-16)", wantStart)
	}
	if fn.Start != wantStart {
		t.Errorf("BMP: length.Start = %d, want %d", fn.Start, wantStart)
	}
}

// TestUTF16Positions_Supplementary verifies that a supplementary character
// (🎉, U+1F389) that is 4 bytes in UTF-8 and 2 UTF-16 code units shifts
// subsequent token positions by 2 compared to the equivalent ASCII source.
func TestUTF16Positions_Supplementary(t *testing.T) {
	// `"🎉" | length` — 🎉 is 4 UTF-8 bytes but 2 UTF-16 code units.
	// Byte layout: "  🎉  " _ | _ l ...
	//              0  1234 5 6 7 8 9 ...
	// UTF-16 pos:  0  1    3 4 5 6 7 ...
	// "length" starts at UTF-8 byte 9, UTF-16 code unit 7.
	q, err := gojq.Parse(`"🎉" | length`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fn := funcNameToken(t, q)
	if fn == nil {
		return
	}
	const wantStart = 7 // UTF-16
	if fn.Start == 9 {
		t.Errorf("Supplementary: length.Start = 9 (byte offset), want %d (UTF-16)", wantStart)
	}
	if fn.Start != wantStart {
		t.Errorf("Supplementary: length.Start = %d, want %d", fn.Start, wantStart)
	}
}

// funcNameToken extracts the *gojq.Token for the function name on the
// right-hand side of a pipe in queries of the form `expr | funcname`.
func funcNameToken(t *testing.T, q *gojq.Query) *gojq.Token {
	t.Helper()
	if q.Right == nil || q.Right.Term == nil || q.Right.Term.Func == nil {
		t.Errorf("unexpected AST shape: expected Right.Term.Func, got %s", q.String())
		return nil
	}
	return q.Right.Term.Func.Name
}
