# gojq — Modo fork

Modo's fork of [itchyny/gojq](https://github.com/itchyny/gojq), extended with AST enhancements required by `jq-lsp` for accurate LSP position mapping and completion support.

Module path: `github.com/modopayments/gojq`
Branch: `feat/composition-lsp` (7 commits ahead of upstream)

---

## Changes from upstream

### UTF-16 positions on all AST tokens

Every token in the AST carries its UTF-16 code-unit start and stop offsets (required by the LSP protocol). A lookup table is built upfront at lex time so each token read is O(1). All AST string fields were converted to typed token values; consumer code reads `.Str` to get the string.

### Structural positions on AST nodes

Additional position fields track structural tokens (keywords, closing brackets, operators) on non-leaf AST nodes — `Query`, `FuncDef`, `Term`, `Object`, `Array`, `Reduce`, `Foreach`, `ObjectKeyVal`, `If`, `IfElif`, and `Try`.

### Comment tracking

Comments are attached to the root AST node in source order and preserved through the parse tree. The printer emits buffered comments before each AST node, enabling formatters to round-trip jq source with comments intact.

**Known caveat:** A comment on its own line before a pipe (`.foo\n# mid\n| .bar`) is re-emitted inline after the LHS. This is documented as expected behavior.

**Behavioral change from upstream:** Upstream supported `\`-continuation on comments (backslash at end of line). This was removed.

### `ParseForCompletion` — trailing-dot completion

Trims trailing whitespace from the source; if the trimmed string ends with `.`, injects a `__cursor__` sentinel immediately after the dot and parses. This allows jq-lsp to request completions inside incomplete dot expressions without requiring syntactically complete input.

**Important limitation:** This only handles the trailing-dot case — it is not a general cursor-offset injection API.

### JSON-serializable AST

`TermType` and `Operator` gain JSON marshaling and `FromString` constructors, enabling full AST JSON serialization for use by jq-lsp over stdio.

---

## Why a fork

Upstream `gojq` exposes byte-offset positions only on root query nodes and has no UTF-16 position support or completion-mode parsing. The changes are too invasive and LSP-specific to upstream.

---

## Known issues

- **`parser.go.y` is stale.** The yacc grammar source has not been updated to match the generated `parser.go`; re-running `goyacc` from it would regress the Token work.
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
