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
	Meta     *ConstObject `json:"meta,omitempty"`
	Imports  []*Import    `json:"imports,omitempty"`
	FuncDefs []*FuncDef   `json:"func_defs,omitempty"`
	Term     *Term        `json:"term,omitempty"`
	Left     *Query       `json:"left,omitempty"`
	Right    *Query       `json:"right,omitempty"`
	Patterns []*Pattern   `json:"patterns,omitempty"`
	Op       Operator     `json:"op,omitempty"`
	Pos      int          `json:"pos,omitempty"`      // byte offset of the first token of this query in the source
	OpPos    int          `json:"op_pos,omitempty"`   // byte offset of the binary operator token (|, ,, //, +, etc.); 0 for non-binary queries
	Comments []Comment    `json:"comments,omitempty"` // all comments in the program; populated only on the root Query
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
	ImportPath  *Token       `json:"import_path,omitempty"`
	ImportAlias *Token       `json:"import_alias,omitempty"`
	IncludePath *Token       `json:"include_path,omitempty"`
	Meta        *ConstObject `json:"meta,omitempty"`
}

func (e *Import) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Import) writeTo(p *printer) {
	if e.ImportPath != nil && e.ImportPath.Str != "" {
		p.buf.WriteString("import ")
		jsonEncodeString(&p.buf, e.ImportPath.Str)
		p.buf.WriteString(" as ")
		p.buf.WriteString(e.ImportAlias.Str)
	} else {
		p.buf.WriteString("include ")
		jsonEncodeString(&p.buf, e.IncludePath.Str)
	}
	if e.Meta != nil {
		p.buf.WriteByte(' ')
		e.Meta.writeTo(p)
	}
	p.buf.WriteString(";\n")
}

// FuncDef ...
type FuncDef struct {
	Name *Token   `json:"name,omitempty"`
	Args []*Token `json:"args,omitempty"`
	Body *Query   `json:"body,omitempty"`
	Pos  int      `json:"pos,omitempty"` // byte offset of the "def" keyword
}

func (e *FuncDef) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *FuncDef) writeTo(p *printer) {
	p.buf.WriteString("def ")
	p.buf.WriteString(e.Name.Str)
	if len(e.Args) > 0 {
		p.buf.WriteByte('(')
		for i, e := range e.Args {
			if i > 0 {
				p.buf.WriteString("; ")
			}
			p.buf.WriteString(e.Str)
		}
		p.buf.WriteByte(')')
	}
	p.buf.WriteString(": ")
	e.Body.writeTo(p)
	p.buf.WriteByte(';')
}

// Term ...
type Term struct {
	Type       TermType   `json:"type,omitempty"`
	Index      *Index     `json:"index,omitempty"`
	Func       *Func      `json:"func,omitempty"`
	Object     *Object    `json:"object,omitempty"`
	Array      *Array     `json:"array,omitempty"`
	Number     *Token     `json:"number,omitempty"`
	Unary      *Unary     `json:"unary,omitempty"`
	Format     *Token     `json:"format,omitempty"`
	Str        *String    `json:"str,omitempty"`
	If         *If        `json:"if,omitempty"`
	Try        *Try       `json:"try,omitempty"`
	Reduce     *Reduce    `json:"reduce,omitempty"`
	Foreach    *Foreach   `json:"foreach,omitempty"`
	Label      *Label     `json:"label,omitempty"`
	Break      *Token     `json:"break,omitempty"`
	Query      *Query     `json:"query,omitempty"`
	SuffixList []*Suffix  `json:"suffix_list,omitempty"`
	Pos        int        `json:"pos,omitempty"`       // byte offset of the first token of this term in the source
	ClosePos   int        `json:"close_pos,omitempty"` // byte offset of the closing ')' for TermTypeQuery terms (0 otherwise)
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
		p.buf.WriteString(e.Number.Str)
	case TermTypeUnary:
		e.Unary.writeTo(p)
	case TermTypeFormat:
		p.buf.WriteString(e.Format.Str)
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
		p.buf.WriteString(e.Break.Str)
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
		return toNumber(e.Number.Str)
	case TermTypeUnary:
		return e.Unary.toNumber()
	case TermTypeString:
		if e.Str.Queries == nil && e.Str.Str != nil {
			return e.Str.Str.Str
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
		return toNumber(e.Number.Str)
	}
	return nil
}

// Unary ...
type Unary struct {
	Op   Operator `json:"op,omitempty"`
	Term *Term    `json:"term,omitempty"`
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
	Name   *Token           `json:"name,omitempty"`
	Array  []*Pattern       `json:"array,omitempty"`
	Object []*PatternObject `json:"object,omitempty"`
}

func (e *Pattern) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Pattern) writeTo(p *printer) {
	if e.Name != nil && e.Name.Str != "" {
		p.buf.WriteString(e.Name.Str)
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
	Key       *Token   `json:"key,omitempty"`
	KeyString *String  `json:"key_string,omitempty"`
	KeyQuery  *Query   `json:"key_query,omitempty"`
	Val       *Pattern `json:"val,omitempty"`
}

func (e *PatternObject) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *PatternObject) writeTo(p *printer) {
	if e.Key != nil && e.Key.Str != "" {
		p.buf.WriteString(e.Key.Str)
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
	Name    *Token  `json:"name,omitempty"`
	Str     *String `json:"str,omitempty"`
	Start   *Query  `json:"start,omitempty"`
	End     *Query  `json:"end,omitempty"`
	IsSlice bool    `json:"is_slice,omitempty"`
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
	if e.Name != nil && e.Name.Str != "" {
		p.buf.WriteString(e.Name.Str)
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
	if e.Name != nil && e.Name.Str != "" {
		return e.Name.Str
	} else if e.Str != nil {
		if e.Str.Queries == nil {
			if e.Str.Str != nil {
				return e.Str.Str.Str
			}
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
	Name *Token   `json:"name,omitempty"`
	Args []*Query `json:"args,omitempty"`
}

func (e *Func) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Func) writeTo(p *printer) {
	p.buf.WriteString(e.Name.Str)
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
	Str     *Token   `json:"str,omitempty"`
	Queries []*Query `json:"queries,omitempty"`
}

func (e *String) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *String) writeTo(p *printer) {
	if e.Queries == nil {
		if e.Str != nil {
			jsonEncodeString(&p.buf, e.Str.Str)
		} else {
			jsonEncodeString(&p.buf, "")
		}
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
	KeyVals  []*ObjectKeyVal `json:"key_vals,omitempty"`
	ClosePos int             `json:"close_pos,omitempty"` // byte offset of the closing '}'
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
	Key       *Token  `json:"key,omitempty"`
	KeyString *String `json:"key_string,omitempty"`
	KeyQuery  *Query  `json:"key_query,omitempty"`
	Val       *Query  `json:"val,omitempty"`
	Pos       int     `json:"pos,omitempty"` // byte offset of the key token (or opening '(' for computed keys)
}

func (e *ObjectKeyVal) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *ObjectKeyVal) writeTo(p *printer) {
	if e.Key != nil && e.Key.Str != "" {
		p.buf.WriteString(e.Key.Str)
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
	Query    *Query `json:"query,omitempty"`
	ClosePos int    `json:"close_pos,omitempty"` // byte offset of the closing ']'
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
	Index    *Index `json:"index,omitempty"`
	Iter     bool   `json:"iter,omitempty"`
	Optional bool   `json:"optional,omitempty"`
}

func (e *Suffix) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Suffix) writeTo(p *printer) {
	if e.Index != nil {
		if (e.Index.Name != nil && e.Index.Name.Str != "") || e.Index.Str != nil {
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
	Cond    *Query   `json:"cond,omitempty"`
	Then    *Query   `json:"then,omitempty"`
	Elif    []*IfElif `json:"elif,omitempty"`
	Else    *Query   `json:"else,omitempty"`
	ThenPos int      `json:"then_pos,omitempty"` // byte offset of the "then" keyword
	ElsePos int      `json:"else_pos,omitempty"` // byte offset of the "else" keyword (0 when no else clause)
	EndPos  int      `json:"end_pos,omitempty"`  // byte offset of the "end" keyword
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
	Cond    *Query `json:"cond,omitempty"`
	Then    *Query `json:"then,omitempty"`
	Pos     int    `json:"pos,omitempty"`      // byte offset of the "elif" keyword
	ThenPos int    `json:"then_pos,omitempty"` // byte offset of the "then" keyword
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
	Body     *Query `json:"body,omitempty"`
	Catch    *Query `json:"catch,omitempty"`
	CatchPos int    `json:"catch_pos,omitempty"` // byte offset of the "catch" keyword (0 when no catch clause)
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
	Query    *Query   `json:"query,omitempty"`
	Pattern  *Pattern `json:"pattern,omitempty"`
	Start    *Query   `json:"start,omitempty"`
	Update   *Query   `json:"update,omitempty"`
	ClosePos int      `json:"close_pos,omitempty"` // byte offset of the closing ')'
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
	Query    *Query   `json:"query,omitempty"`
	Pattern  *Pattern `json:"pattern,omitempty"`
	Start    *Query   `json:"start,omitempty"`
	Update   *Query   `json:"update,omitempty"`
	Extract  *Query   `json:"extract,omitempty"`
	ClosePos int      `json:"close_pos,omitempty"` // byte offset of the closing ')'
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
	Ident *Token `json:"ident,omitempty"`
	Body  *Query `json:"body,omitempty"`
}

func (e *Label) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *Label) writeTo(p *printer) {
	p.buf.WriteString("label ")
	p.buf.WriteString(e.Ident.Str)
	p.buf.WriteString(" | ")
	e.Body.writeTo(p)
}

// ConstTerm ...
type ConstTerm struct {
	Object *ConstObject `json:"object,omitempty"`
	Array  *ConstArray  `json:"array,omitempty"`
	Number *Token       `json:"number,omitempty"`
	Str    *Token       `json:"str,omitempty"`
	Null   bool         `json:"null,omitempty"`
	True   bool         `json:"true,omitempty"`
	False  bool         `json:"false,omitempty"`
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
	} else if e.Number != nil && e.Number.Str != "" {
		p.buf.WriteString(e.Number.Str)
	} else if e.Null {
		p.buf.WriteString("null")
	} else if e.True {
		p.buf.WriteString("true")
	} else if e.False {
		p.buf.WriteString("false")
	} else {
		if e.Str != nil {
			jsonEncodeString(&p.buf, e.Str.Str)
		} else {
			jsonEncodeString(&p.buf, "")
		}
	}
}

func (e *ConstTerm) toValue() any {
	if e.Object != nil {
		return e.Object.ToValue()
	} else if e.Array != nil {
		return e.Array.toValue()
	} else if e.Number != nil && e.Number.Str != "" {
		return toNumber(e.Number.Str)
	} else if e.Null {
		return nil
	} else if e.True {
		return true
	} else if e.False {
		return false
	} else {
		if e.Str != nil {
			return e.Str.Str
		}
		return ""
	}
}

func (e *ConstTerm) toString() (string, bool) {
	if e.Object != nil || e.Array != nil ||
		(e.Number != nil && e.Number.Str != "") || e.Null || e.True || e.False {
		return "", false
	}
	if e.Str != nil {
		return e.Str.Str, true
	}
	return "", true
}

// ConstObject ...
type ConstObject struct {
	KeyVals []*ConstObjectKeyVal `json:"key_vals,omitempty"`
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
		key := ""
		if e.Key != nil {
			key = e.Key.Str
		}
		if key == "" && e.KeyString != nil {
			key = e.KeyString.Str
		}
		v[key] = e.Val.toValue()
	}
	return v
}

// ConstObjectKeyVal ...
type ConstObjectKeyVal struct {
	Key       *Token     `json:"key,omitempty"`
	KeyString *Token     `json:"key_string,omitempty"`
	Val       *ConstTerm `json:"val,omitempty"`
}

func (e *ConstObjectKeyVal) String() string {
	p := newPrinter(nil)
	e.writeTo(p)
	return p.buf.String()
}

func (e *ConstObjectKeyVal) writeTo(p *printer) {
	if e.Key != nil && e.Key.Str != "" {
		p.buf.WriteString(e.Key.Str)
	} else {
		if e.KeyString != nil {
			jsonEncodeString(&p.buf, e.KeyString.Str)
		} else {
			jsonEncodeString(&p.buf, "")
		}
	}
	p.buf.WriteString(": ")
	e.Val.writeTo(p)
}

// ConstArray ...
type ConstArray struct {
	Elems []*ConstTerm `json:"elems,omitempty"`
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

// CompletionSentinel is the synthetic identifier appended by ParseForCompletion
// when it recovers from a trailing dot.
const CompletionSentinel = "__cursor__"

// ParseForCompletion parses src, recovering from a trailing "." by appending
// CompletionSentinel so path-context analysis can locate the completion
// position. Returns (query, true, nil) on recovery, (query, false, nil) on a
// clean parse, or (nil, false, err) on an unrecoverable error.
func ParseForCompletion(src string) (*Query, bool, error) {
	trimmed := strings.TrimRight(src, " \t\n\r")
	if strings.HasSuffix(trimmed, ".") {
		// Insert the sentinel immediately after the dot, before any trailing
		// whitespace. This keeps the sentinel's byte offset aligned with the
		// editor cursor position (which is always placed right after the dot,
		// not after any trailing newline).
		dotEnd := len(trimmed)
		modified := src[:dotEnd] + CompletionSentinel + src[dotEnd:]
		if q2, err2 := Parse(modified); err2 == nil {
			return q2, true, nil
		}
	}
	q, err := Parse(src)
	if err == nil {
		return q, false, nil
	}
	return nil, false, err
}
