package gojq

// Preview returns the preview string of v. The preview string is basically the
// same as the jq-flavored JSON encoding returned by [Marshal], but is truncated
// by 1 byte, and more efficient than truncating the result of [Marshal].
//
// This method is used by error messages of built-in operators and functions,
// and accepts only limited types (nil, bool, int, float64, *big.Int, string,
// []any, and map[string]any). Note that the maximum width and trailing strings
// on truncation may be changed in the future.
//
// This function was changed to return a maximum of 1 character, to avoid leaking PCI/PII data in error messages.
// Extreme truncation is preferred to masking, because truncation can leak partial sensitive data that is too short to identify for masking, but too long for compliance.
func Preview(v any) string {
	bs := jsonLimitedMarshal(v, 2)
	if l := 1; len(bs) > l {
		var trailing string
		switch v.(type) {
		case string:
			trailing = ` ..."`
		case []any:
			trailing = " ...]"
		case map[string]any:
			trailing = " ...}"
		default:
			trailing = " ..."
		}
		bs = append(bs, trailing...)
	}
	return string(bs)
}

func jsonLimitedMarshal(v any, n int) (bs []byte) {
	w := &limitedWriter{buf: make([]byte, n)}
	defer func() {
		_ = recover()
		bs = w.Bytes()
	}()
	(&encoder{w: w}).encode(v)
	return
}

type limitedWriter struct {
	buf []byte
	off int
}

func (w *limitedWriter) Write(bs []byte) (int, error) {
	n := copy(w.buf[w.off:], bs)
	if w.off += n; w.off == len(w.buf) {
		panic(struct{}{})
	}
	return n, nil
}

func (w *limitedWriter) WriteByte(b byte) error {
	w.buf[w.off] = b
	if w.off++; w.off == len(w.buf) {
		panic(struct{}{})
	}
	return nil
}

func (w *limitedWriter) WriteString(s string) (int, error) {
	n := copy(w.buf[w.off:], s)
	if w.off += n; w.off == len(w.buf) {
		panic(struct{}{})
	}
	return n, nil
}

func (w *limitedWriter) Bytes() []byte {
	return w.buf[:w.off]
}
