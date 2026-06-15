# gojq — Modo fork

Modo's fork of [itchyny/gojq](https://github.com/itchyny/gojq), extended with AST enhancements required by `jq-lsp` for accurate LSP position mapping and completion support.

Module path: `github.com/modopayments/gojq`
Branch: `feat/composition-lsp` (7 commits ahead of upstream)

---

## Changes from upstream

### `Token` type with UTF-16 positions (`lexer.go`)

A new `Token` struct carries the source text and its UTF-16 code-unit offsets:

```go
type Token struct {
    Str   string `json:"str"`
    Start int    `json:"start"` // flat UTF-16 code-unit offset of first character
    Stop  int    `json:"stop"`  // flat UTF-16 code-unit offset past last character
}
```

`Start`/`Stop` are flat offsets into the UTF-16 encoding of the full source string (not line/column pairs). A `buildByteToUTF16Pos(s string) []int` lookup table is built upfront at lex time (O(n)); each token read is O(1).

All AST string fields were converted from `string` to `*Token`. Affected fields include: `FuncDef.Name`, `FuncDef.Args`, `Import.ImportPath`/`Alias`/`IncludePath`, `Term.Number`/`Format`/`Break`, `Index.Name`, `Pattern.Name`, `PatternObject.Key`, `Func.Name`, `ObjectKeyVal.Key`, `String.Str`, `ConstTerm.Number`/`Str`, `ConstObjectKeyVal.Key`/`KeyString`. Consumer code dereferences `.Str` to get the string value.

### Structural position fields on AST nodes

Additional byte-offset position fields on non-leaf AST nodes for tracking structural tokens:

| Node | Field(s) | Meaning |
|---|---|---|
| `Query` | `Pos`, `OpPos` | First token; binary operator (`\|`, `,`, `//`, etc.) |
| `FuncDef` | `Pos` | `def` keyword |
| `Term` | `Pos`, `ClosePos` | First token; closing `)` for parenthesized terms |
| `Object` | `ClosePos` | `}` |
| `Array` | `ClosePos` | `]` |
| `Reduce` | `ClosePos` | `)` |
| `Foreach` | `ClosePos` | `)` |
| `ObjectKeyVal` | `Pos` | Key or `(` for computed keys |
| `If` | `ThenPos`, `ElsePos`, `EndPos` | Those keywords |
| `IfElif` | `Pos`, `ThenPos` | Those keywords |
| `Try` | `CatchPos` | `catch` keyword (`0` if absent) |

### Comment tracking

```go
type Comment struct {
    Text   string // full text including leading #, no trailing newline
    Pos    int    // byte offset of #
    Col    int    // bytes from last newline to #
    Inline bool   // true when non-whitespace precedes # on same line
}
```

`Query.Comments []Comment` is populated on the root `Query` only (all comments in source order). The `printer` type's `flush(beforePos int)` emits buffered comments before each AST node, enabling round-trip formatting with comments preserved.

**Known caveat:** A comment on its own line before a pipe (`.foo\n# mid\n| .bar`) is re-emitted inline after the LHS. This is documented as expected behavior.

**Behavioral change from upstream:** Upstream `skipComment()` supported `\`-continuation (backslash at end of line to continue a comment). Modo's `scanComment()` dropped this; the corresponding test cases were removed.

### `ParseForCompletion` — trailing-dot completion

```go
const CompletionSentinel = "__cursor__"

func ParseForCompletion(src string) (*Query, bool, error)
```

Trims trailing whitespace from `src`. If the trimmed string ends with `.`, inserts `__cursor__` immediately after the dot (making it a field access like `.foo.__cursor__`) and calls `Parse` on the result. Returns `(query, true, nil)` when the sentinel was injected, `(query, false, nil)` on a clean parse, `(nil, false, err)` on failure.

**Important limitation:** This only handles the trailing-dot case — it is not a general cursor-offset injection API. `jq-lsp` uses it exclusively for dot-triggered completions.

### JSON-serializable AST

`TermType` and `Operator` gain `MarshalJSON`/`UnmarshalJSON`/`GoString` methods and `FromString` constructors (`TermTypeFromString`, `OperatorFromString`), enabling full AST JSON serialization for use by jq-lsp over stdio.

---

## Why a fork

Upstream `gojq` exposes byte-offset positions only on root `*gojq.Query` nodes and has no UTF-16 position support or completion-mode parsing. The changes are too invasive and LSP-specific to upstream.

---

## Known issues

- **`parser.go.y` is stale.** The yacc grammar source still uses `[]string` type assertions for `funcargs`, but the generated `parser.go` correctly uses `[]*Token`. Re-running `goyacc` from the `.y` source would regress the Token work; `parser.go.y` must be updated manually before any future yacc regeneration.
- **Line-continuation comments silently dropped.** Upstream's `\`-continuation support was removed without replacement.
- **No tests for `ParseForCompletion`** or for UTF-16 position correctness on non-ASCII (multi-byte) input.

---

## Staying in sync

Upstream releases are merged periodically. The Modo-specific commits sit on top of the upstream tip:

```
03734b1  docs: add MODO.md                                      ← Modo
b14d91e  fix: completion sentinel before trailing whitespace    ← Modo
66587b7  feat: Token type with UTF-16 positions on all AST nodes ← Modo
339c31d  Add ClosePos to Object, Array, Reduce, Foreach         ← Modo
51c8b5e  Add source positions for remaining structural tokens   ← Modo
8913de5  Add EndPos to If struct                                ← Modo
b76755e  Add comment tracking and source positions to AST       ← Modo
```

When rebasing onto a new upstream release, conflicts are typically limited to `parser.go.y` (yacc grammar) and `lexer.go`.
