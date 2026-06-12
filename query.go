package gojq

import (
	"context"
	"strings"
)

// Comment is a single-line jq comment (# to end of line).
type Comment struct {
	Text   string // includes the leading #, no trailing newline
	Pos    int    // byte offset of the # in the source
	Col    int    // column of the # (bytes since the last newline)
	Inline bool   // true when non-whitespace code precedes # on the same line
}

// printer holds state for position-aware printing (gofmt pattern).
// It walks the AST and flushes any comment whose Pos falls before the
// current node's Pos before writing the node itself.
type printer struct {
	buf      strings.Builder
	comments []Comment
	ci       int // index of next comment to emit
}

func newPrinter(comments []Comment) *printer {
	return &printer{comments: comments}
}

// writeComment emits a single comment. When the buffer is empty or ends with a
// newline the comment is on its own line and its original column is preserved
// (Col leading spaces). Otherwise the comment is inline and no spaces are
// added — the caller has already written the preceding token.
func (p *printer) writeComment(c Comment) {
	if s := p.buf.String(); len(s) == 0 || s[len(s)-1] == '\n' {
		if c.Col > 0 {
			p.buf.WriteString(strings.Repeat(" ", c.Col))
		}
	}
	p.buf.WriteString(c.Text)
	p.buf.WriteByte('\n')
}

// flush emits all comments whose Pos is strictly less than beforePos.
func (p *printer) flush(beforePos int) {
	for p.ci < len(p.comments) && p.comments[p.ci].Pos < beforePos {
		p.writeComment(p.comments[p.ci])
		p.ci++
	}
}

// flushAll emits all remaining comments. If the buffer doesn't end with a
// newline a newline is inserted first so the comment lands on its own line.
func (p *printer) flushAll() {
	for p.ci < len(p.comments) {
		if s := p.buf.String(); len(s) > 0 && s[len(s)-1] != '\n' {
			p.buf.WriteByte('\n')
		}
		p.writeComment(p.comments[p.ci])
		p.ci++
	}
}

// Parse a query string, and returns the query struct.
//
// If parsing failed, it returns an error of type [*ParseError], which has
// the byte offset and the invalid token. The byte offset is the scanned bytes
// when the error occurred. The token is empty if the error occurred after
// scanning the entire query string.
func Parse(src string) (*Query, error) {
	l := newLexer(src)
	if yyParse(l) > 0 {
		return nil, l.err
	}
	return l.result, nil
}

// Query represents the abstract syntax tree of a jq query.
type Query struct {
	Meta     *ConstObject
	Imports  []*Import
	FuncDefs []*FuncDef
	Term     *Term
	Left     *Query
	Right    *Query
	Patterns []*Pattern
	Op       Operator
	Pos      int       // byte offset of the first token of this query in the source
	OpPos    int       // byte offset of the binary operator token (|, ,, //, +, etc.); 0 for non-binary queries
	Comments []Comment // all comments in the program; populated only on the root Query
}

// Run the query.
//
// It is safe to call this method in goroutines, to reuse a parsed [*Query].
func (e *Query) Run(v any) Iter {
	return e.RunWithContext(context.Background(), v)
}

// RunWithContext runs the query with context.
func (e *Query) RunWithContext(ctx context.Context, v any) Iter {
	code, err := Compile(e)
	if err != nil {
		return NewIter(err)
	}
	return code.RunWithContext(ctx, v)
}

func (e *Query) String() string {
	p := newPrinter(e.Comments)
	e.writeTo(p)
	p.flushAll()
	return p.buf.String()
}

func (e *Query) writeTo(p *printer) {
	p.flush(e.Pos)
	if e.Meta != nil {
		p.buf.WriteString("module ")
		e.Meta.writeTo(p)
		p.buf.WriteString(";\n")
	}
	for _, im := range e.Imports {
		im.writeTo(p)
	}
	for _, fd := range e.FuncDefs {
		fd.writeTo(p)
		p.buf.WriteByte(' ')
	}
	if e.Term != nil {
		e.Term.writeTo(p)
	} else if e.Right != nil {
		e.Left.writeTo(p)
		if e.Op != OpComma {
			p.buf.WriteByte(' ')
		}
		for i, pat := range e.Patterns {
			if i == 0 {
				p.buf.WriteString("as ")
			} else {
				p.buf.WriteString("?// ")
			}
			pat.writeTo(p)
			p.buf.WriteByte(' ')
		}
		p.buf.WriteString(e.Op.String())
		p.buf.WriteByte(' ')
		e.Right.writeTo(p)
	}
}

func (e *Query) toIndexKey() any {
	if e.Term == nil {
		return nil
	}
	return e.Term.toIndexKey()
}

func (e *Query) toIndices(xs []any) []any {
	if e.Term == nil {
		return nil
	}
	return e.Term.toIndices(xs)
}

// Import ...
type Import struct {
	ImportPath  string
	ImportAlias string
	IncludePath string
	Meta        *ConstObject
}

func (e *Import) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Import) writeTo(p *printer) {
	if e.ImportPath != "" {
		p.buf.WriteString("import ")
		jsonEncodeString(&p.buf, e.ImportPath)
		p.buf.WriteString(" as ")
		p.buf.WriteString(e.ImportAlias)
	} else {
		p.buf.WriteString("include ")
		jsonEncodeString(&p.buf, e.IncludePath)
	}
	if e.Meta != nil {
		p.buf.WriteByte(' ')
		e.Meta.writeTo(p)
	}
	p.buf.WriteString(";\n")
}

// FuncDef ...
type FuncDef struct {
	Name string
	Args []string
	Body *Query
	Pos  int // byte offset of the "def" keyword
}

func (e *FuncDef) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *FuncDef) writeTo(p *printer) {
	p.buf.WriteString("def ")
	p.buf.WriteString(e.Name)
	if len(e.Args) > 0 {
		p.buf.WriteByte('(')
		for i, e := range e.Args {
			if i > 0 {
				p.buf.WriteString("; ")
			}
			p.buf.WriteString(e)
		}
		p.buf.WriteByte(')')
	}
	p.buf.WriteString(": ")
	e.Body.writeTo(p)
	p.buf.WriteByte(';')
}

// Term ...
type Term struct {
	Type       TermType
	Index      *Index
	Func       *Func
	Object     *Object
	Array      *Array
	Number     string
	Unary      *Unary
	Format     string
	Str        *String
	If         *If
	Try        *Try
	Reduce     *Reduce
	Foreach    *Foreach
	Label      *Label
	Break      string
	Query      *Query
	SuffixList []*Suffix
	Pos        int // byte offset of the first token of this term in the source
	ClosePos   int // byte offset of the closing ')' for TermTypeQuery terms (0 otherwise)
}

func (e *Term) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Term) writeTo(p *printer) {
	switch e.Type {
	case TermTypeIdentity:
		p.buf.WriteByte('.')
	case TermTypeRecurse:
		p.buf.WriteString("..")
	case TermTypeNull:
		p.buf.WriteString("null")
	case TermTypeTrue:
		p.buf.WriteString("true")
	case TermTypeFalse:
		p.buf.WriteString("false")
	case TermTypeIndex:
		e.Index.writeTo(p)
	case TermTypeFunc:
		e.Func.writeTo(p)
	case TermTypeObject:
		e.Object.writeTo(p)
	case TermTypeArray:
		e.Array.writeTo(p)
	case TermTypeNumber:
		p.buf.WriteString(e.Number)
	case TermTypeUnary:
		e.Unary.writeTo(p)
	case TermTypeFormat:
		p.buf.WriteString(e.Format)
		if e.Str != nil {
			p.buf.WriteByte(' ')
			e.Str.writeTo(p)
		}
	case TermTypeString:
		e.Str.writeTo(p)
	case TermTypeIf:
		e.If.writeTo(p)
	case TermTypeTry:
		e.Try.writeTo(p)
	case TermTypeReduce:
		e.Reduce.writeTo(p)
	case TermTypeForeach:
		e.Foreach.writeTo(p)
	case TermTypeLabel:
		e.Label.writeTo(p)
	case TermTypeBreak:
		p.buf.WriteString("break ")
		p.buf.WriteString(e.Break)
	case TermTypeQuery:
		p.buf.WriteByte('(')
		e.Query.writeTo(p)
		p.buf.WriteByte(')')
	}
	for _, e := range e.SuffixList {
		e.writeTo(p)
	}
}

func (e *Term) toIndexKey() any {
	switch e.Type {
	case TermTypeNumber:
		return toNumber(e.Number)
	case TermTypeUnary:
		return e.Unary.toNumber()
	case TermTypeString:
		if e.Str.Queries == nil {
			return e.Str.Str
		}
		return nil
	default:
		return nil
	}
}

func (e *Term) toIndices(xs []any) []any {
	switch e.Type {
	case TermTypeIndex:
		if xs = e.Index.toIndices(xs); xs == nil {
			return nil
		}
	case TermTypeQuery:
		if xs = e.Query.toIndices(xs); xs == nil {
			return nil
		}
	default:
		return nil
	}
	for _, s := range e.SuffixList {
		if xs = s.toIndices(xs); xs == nil {
			return nil
		}
	}
	return xs
}

func (e *Term) toNumber() any {
	if e.Type == TermTypeNumber {
		return toNumber(e.Number)
	}
	return nil
}

// Unary ...
type Unary struct {
	Op   Operator
	Term *Term
}

func (e *Unary) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Unary) writeTo(p *printer) {
	p.buf.WriteString(e.Op.String())
	e.Term.writeTo(p)
}

func (e *Unary) toNumber() any {
	v := e.Term.toNumber()
	if v != nil && e.Op == OpSub {
		v = funcOpNegate(v)
	}
	return v
}

// Pattern ...
type Pattern struct {
	Name   string
	Array  []*Pattern
	Object []*PatternObject
}

func (e *Pattern) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Pattern) writeTo(p *printer) {
	if e.Name != "" {
		p.buf.WriteString(e.Name)
	} else if len(e.Array) > 0 {
		p.buf.WriteByte('[')
		for i, e := range e.Array {
			if i > 0 {
				p.buf.WriteString(", ")
			}
			e.writeTo(p)
		}
		p.buf.WriteByte(']')
	} else if len(e.Object) > 0 {
		p.buf.WriteByte('{')
		for i, e := range e.Object {
			if i > 0 {
				p.buf.WriteString(", ")
			}
			e.writeTo(p)
		}
		p.buf.WriteByte('}')
	}
}

// PatternObject ...
type PatternObject struct {
	Key       string
	KeyString *String
	KeyQuery  *Query
	Val       *Pattern
}

func (e *PatternObject) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *PatternObject) writeTo(p *printer) {
	if e.Key != "" {
		p.buf.WriteString(e.Key)
	} else if e.KeyString != nil {
		e.KeyString.writeTo(p)
	} else if e.KeyQuery != nil {
		p.buf.WriteByte('(')
		e.KeyQuery.writeTo(p)
		p.buf.WriteByte(')')
	}
	if e.Val != nil {
		p.buf.WriteString(": ")
		e.Val.writeTo(p)
	}
}

// Index ...
type Index struct {
	Name    string
	Str     *String
	Start   *Query
	End     *Query
	IsSlice bool
}

func (e *Index) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Index) writeTo(p *printer) {
	if l := p.buf.Len(); l > 0 {
		// ". .x" != "..x" and "0 .x" != "0.x"
		if c := p.buf.String()[l-1]; c == '.' || '0' <= c && c <= '9' {
			p.buf.WriteByte(' ')
		}
	}
	p.buf.WriteByte('.')
	e.writeSuffixTo(p)
}

func (e *Index) writeSuffixTo(p *printer) {
	if e.Name != "" {
		p.buf.WriteString(e.Name)
	} else if e.Str != nil {
		e.Str.writeTo(p)
	} else {
		p.buf.WriteByte('[')
		if e.IsSlice {
			if e.Start != nil {
				e.Start.writeTo(p)
			}
			p.buf.WriteByte(':')
			if e.End != nil {
				e.End.writeTo(p)
			}
		} else {
			e.Start.writeTo(p)
		}
		p.buf.WriteByte(']')
	}
}

func (e *Index) toIndexKey() any {
	if e.Name != "" {
		return e.Name
	} else if e.Str != nil {
		if e.Str.Queries == nil {
			return e.Str.Str
		}
	} else if !e.IsSlice {
		return e.Start.toIndexKey()
	} else {
		var start, end any
		ok := true
		if e.Start != nil {
			start = e.Start.toIndexKey()
			ok = start != nil
		}
		if e.End != nil && ok {
			end = e.End.toIndexKey()
			ok = end != nil
		}
		if ok {
			return map[string]any{"start": start, "end": end}
		}
	}
	return nil
}

func (e *Index) toIndices(xs []any) []any {
	if k := e.toIndexKey(); k != nil {
		return append(xs, k)
	}
	return nil
}

// Func ...
type Func struct {
	Name string
	Args []*Query
}

func (e *Func) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Func) writeTo(p *printer) {
	p.buf.WriteString(e.Name)
	if len(e.Args) > 0 {
		p.buf.WriteByte('(')
		for i, e := range e.Args {
			if i > 0 {
				p.buf.WriteString("; ")
			}
			e.writeTo(p)
		}
		p.buf.WriteByte(')')
	}
}

// String ...
type String struct {
	Str     string
	Queries []*Query
}

func (e *String) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *String) writeTo(p *printer) {
	if e.Queries == nil {
		jsonEncodeString(&p.buf, e.Str)
		return
	}
	p.buf.WriteByte('"')
	for _, e := range e.Queries {
		if e.Term.Str == nil {
			p.buf.WriteString(`\`)
			e.writeTo(p)
		} else {
			es := e.String()
			p.buf.WriteString(es[1 : len(es)-1])
		}
	}
	p.buf.WriteByte('"')
}

// Object ...
type Object struct {
	KeyVals  []*ObjectKeyVal
	ClosePos int // byte offset of the closing '}'
}

func (e *Object) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Object) writeTo(p *printer) {
	if len(e.KeyVals) == 0 {
		p.buf.WriteString("{}")
		return
	}
	p.buf.WriteString("{ ")
	for i, kv := range e.KeyVals {
		if i > 0 {
			p.buf.WriteString(", ")
		}
		kv.writeTo(p)
	}
	p.buf.WriteString(" }")
}

// ObjectKeyVal ...
type ObjectKeyVal struct {
	Key       string
	KeyString *String
	KeyQuery  *Query
	Val       *Query
	Pos       int // byte offset of the key token (or opening '(' for computed keys)
}

func (e *ObjectKeyVal) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *ObjectKeyVal) writeTo(p *printer) {
	if e.Key != "" {
		p.buf.WriteString(e.Key)
	} else if e.KeyString != nil {
		e.KeyString.writeTo(p)
	} else if e.KeyQuery != nil {
		p.buf.WriteByte('(')
		e.KeyQuery.writeTo(p)
		p.buf.WriteByte(')')
	}
	if e.Val != nil {
		p.buf.WriteString(": ")
		e.Val.writeTo(p)
	}
}

// Array ...
type Array struct {
	Query    *Query
	ClosePos int // byte offset of the closing ']'
}

func (e *Array) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Array) writeTo(p *printer) {
	p.buf.WriteByte('[')
	if e.Query != nil {
		e.Query.writeTo(p)
	}
	p.buf.WriteByte(']')
}

// Suffix ...
type Suffix struct {
	Index    *Index
	Iter     bool
	Optional bool
}

func (e *Suffix) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Suffix) writeTo(p *printer) {
	if e.Index != nil {
		if e.Index.Name != "" || e.Index.Str != nil {
			e.Index.writeTo(p)
		} else {
			e.Index.writeSuffixTo(p)
		}
	} else if e.Iter {
		p.buf.WriteString("[]")
	} else if e.Optional {
		p.buf.WriteByte('?')
	}
}

func (e *Suffix) toTerm() *Term {
	if e.Index != nil {
		return &Term{Type: TermTypeIndex, Index: e.Index}
	} else if e.Iter {
		return &Term{Type: TermTypeIdentity, SuffixList: []*Suffix{{Iter: true}}}
	} else {
		return nil
	}
}

func (e *Suffix) toIndices(xs []any) []any {
	if e.Index == nil {
		return nil
	}
	return e.Index.toIndices(xs)
}

// If ...
type If struct {
	Cond    *Query
	Then    *Query
	Elif    []*IfElif
	Else    *Query
	ThenPos int // byte offset of the "then" keyword
	ElsePos int // byte offset of the "else" keyword (0 when no else clause)
	EndPos  int // byte offset of the "end" keyword
}

func (e *If) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *If) writeTo(p *printer) {
	p.buf.WriteString("if ")
	e.Cond.writeTo(p)
	p.buf.WriteString(" then ")
	e.Then.writeTo(p)
	for _, e := range e.Elif {
		p.buf.WriteByte(' ')
		e.writeTo(p)
	}
	if e.Else != nil {
		p.buf.WriteString(" else ")
		e.Else.writeTo(p)
	}
	p.buf.WriteString(" end")
}

// IfElif ...
type IfElif struct {
	Cond    *Query
	Then    *Query
	Pos     int // byte offset of the "elif" keyword
	ThenPos int // byte offset of the "then" keyword
}

func (e *IfElif) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *IfElif) writeTo(p *printer) {
	p.buf.WriteString("elif ")
	e.Cond.writeTo(p)
	p.buf.WriteString(" then ")
	e.Then.writeTo(p)
}

// Try ...
type Try struct {
	Body     *Query
	Catch    *Query
	CatchPos int // byte offset of the "catch" keyword (0 when no catch clause)
}

func (e *Try) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Try) writeTo(p *printer) {
	p.buf.WriteString("try ")
	e.Body.writeTo(p)
	if e.Catch != nil {
		p.buf.WriteString(" catch ")
		e.Catch.writeTo(p)
	}
}

// Reduce ...
type Reduce struct {
	Query    *Query
	Pattern  *Pattern
	Start    *Query
	Update   *Query
	ClosePos int // byte offset of the closing ')'
}

func (e *Reduce) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Reduce) writeTo(p *printer) {
	p.buf.WriteString("reduce ")
	e.Query.writeTo(p)
	p.buf.WriteString(" as ")
	e.Pattern.writeTo(p)
	p.buf.WriteString(" (")
	e.Start.writeTo(p)
	p.buf.WriteString("; ")
	e.Update.writeTo(p)
	p.buf.WriteByte(')')
}

// Foreach ...
type Foreach struct {
	Query    *Query
	Pattern  *Pattern
	Start    *Query
	Update   *Query
	Extract  *Query
	ClosePos int // byte offset of the closing ')'
}

func (e *Foreach) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Foreach) writeTo(p *printer) {
	p.buf.WriteString("foreach ")
	e.Query.writeTo(p)
	p.buf.WriteString(" as ")
	e.Pattern.writeTo(p)
	p.buf.WriteString(" (")
	e.Start.writeTo(p)
	p.buf.WriteString("; ")
	e.Update.writeTo(p)
	if e.Extract != nil {
		p.buf.WriteString("; ")
		e.Extract.writeTo(p)
	}
	p.buf.WriteByte(')')
}

// Label ...
type Label struct {
	Ident string
	Body  *Query
}

func (e *Label) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Label) writeTo(p *printer) {
	p.buf.WriteString("label ")
	p.buf.WriteString(e.Ident)
	p.buf.WriteString(" | ")
	e.Body.writeTo(p)
}

// ConstTerm ...
type ConstTerm struct {
	Object *ConstObject
	Array  *ConstArray
	Number string
	Str    string
	Null   bool
	True   bool
	False  bool
}

func (e *ConstTerm) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *ConstTerm) writeTo(p *printer) {
	if e.Object != nil {
		e.Object.writeTo(p)
	} else if e.Array != nil {
		e.Array.writeTo(p)
	} else if e.Number != "" {
		p.buf.WriteString(e.Number)
	} else if e.Null {
		p.buf.WriteString("null")
	} else if e.True {
		p.buf.WriteString("true")
	} else if e.False {
		p.buf.WriteString("false")
	} else {
		jsonEncodeString(&p.buf, e.Str)
	}
}

func (e *ConstTerm) toValue() any {
	if e.Object != nil {
		return e.Object.ToValue()
	} else if e.Array != nil {
		return e.Array.toValue()
	} else if e.Number != "" {
		return toNumber(e.Number)
	} else if e.Null {
		return nil
	} else if e.True {
		return true
	} else if e.False {
		return false
	} else {
		return e.Str
	}
}

func (e *ConstTerm) toString() (string, bool) {
	if e.Object != nil || e.Array != nil ||
		e.Number != "" || e.Null || e.True || e.False {
		return "", false
	}
	return e.Str, true
}

// ConstObject ...
type ConstObject struct {
	KeyVals []*ConstObjectKeyVal
}

func (e *ConstObject) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *ConstObject) writeTo(p *printer) {
	if len(e.KeyVals) == 0 {
		p.buf.WriteString("{}")
		return
	}
	p.buf.WriteString("{ ")
	for i, kv := range e.KeyVals {
		if i > 0 {
			p.buf.WriteString(", ")
		}
		kv.writeTo(p)
	}
	p.buf.WriteString(" }")
}

// ToValue converts the object to map[string]any.
func (e *ConstObject) ToValue() map[string]any {
	if e == nil {
		return nil
	}
	v := make(map[string]any, len(e.KeyVals))
	for _, e := range e.KeyVals {
		key := e.Key
		if key == "" {
			key = e.KeyString
		}
		v[key] = e.Val.toValue()
	}
	return v
}

// ConstObjectKeyVal ...
type ConstObjectKeyVal struct {
	Key       string
	KeyString string
	Val       *ConstTerm
}

func (e *ConstObjectKeyVal) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *ConstObjectKeyVal) writeTo(p *printer) {
	if e.Key != "" {
		p.buf.WriteString(e.Key)
	} else {
		jsonEncodeString(&p.buf, e.KeyString)
	}
	p.buf.WriteString(": ")
	e.Val.writeTo(p)
}

// ConstArray ...
type ConstArray struct {
	Elems []*ConstTerm
}

func (e *ConstArray) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *ConstArray) writeTo(p *printer) {
	p.buf.WriteByte('[')
	for i, e := range e.Elems {
		if i > 0 {
			p.buf.WriteString(", ")
		}
		e.writeTo(p)
	}
	p.buf.WriteByte(']')
}

func (e *ConstArray) toValue() []any {
	v := make([]any, len(e.Elems))
	for i, e := range e.Elems {
		v[i] = e.toValue()
	}
	return v
}
