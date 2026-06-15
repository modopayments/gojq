# gojq — Modo fork

Modo's fork of [itchyny/gojq](https://github.com/itchyny/gojq), extended with AST enhancements required by `jq-lsp` for accurate LSP position mapping and completion support.

---

## Changes from upstream

### Full source positions on all AST nodes
Every AST node carries a `Token` value with:
- `Line` / `Col` — 1-based byte-offset position (used internally)
- `UTF16Line` / `UTF16Col` — UTF-16 code-unit position (required by the LSP protocol, which uses UTF-16 offsets)

Nodes covered: all identifiers, operators, keywords, string literals, format strings, labels, variables, function calls, object keys, `if`/`else`/`end`, `reduce`/`foreach`, `try`/`catch`, `break`, `$__loc__`, and all closing tokens (`]`, `}`, `end`).

### Comment tracking
Comments are attached to the AST and preserved through the parse tree, enabling formatters to round-trip jq source with comments intact.

### Completion sentinel insertion
`ParseForCompletion(src string, offset int)` injects a `__cursor__` placeholder token at `offset` before parsing. This allows jq-lsp to request completions mid-expression without requiring syntactically complete input. The sentinel is inserted before any trailing whitespace so it lands inside the expression being typed rather than after it.

---

## Why a fork

The upstream `gojq` parser exposes byte-offset positions only on `*gojq.Query` nodes (not on individual tokens), and does not support UTF-16 positions or completion-mode parsing. These changes are too invasive and LSP-specific to upstream.

---

## Staying in sync

Upstream releases are merged periodically. The Modo-specific commits sit on top of the upstream tip:

```
b76755e  Add comment tracking and source positions to AST      ← Modo
8913de5  Add EndPos to If struct                               ← Modo
51c8b5e  Add source positions for all remaining structural tokens ← Modo
339c31d  Add ClosePos to Object, Array, Reduce, Foreach        ← Modo
66587b7  feat: add Token type with UTF-16 positions            ← Modo
b14d91e  fix: completion sentinel before trailing whitespace   ← Modo
```

When rebasing onto a new upstream release, conflicts are typically limited to `parser.go.y` (yacc grammar) and `lexer.go`.
