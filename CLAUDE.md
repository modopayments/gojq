# gojq

Fork of `itchyny/gojq` on branch `feat/composition-lsp`. Extends the jq parser with UTF-16 positions, comment tracking, and completion-sentinel insertion for use by `jq-lsp`. Module: `github.com/modopayments/gojq`.

## Upstream

- `origin` → `itchyny/gojq` (upstream)
- `modopayments` → `modopayments/gojq` (our fork)

## Build & test

```sh
make build   # builds the gojq CLI
make test    # go test -v -race ./...
go test ./...
```

## Key files

- `lexer.go` — token type with UTF-16 positions, `buildByteToUTF16Pos`, `ParseForCompletion`
- `query.go` — AST types with added position fields, comment tracking, `printer`
- `parser.go` — generated from `parser.go.y` (see warning below)
- `operator.go` — `Operator` with JSON marshaling
- `term_type.go` — `TermType` with JSON marshaling

## Critical: do not regenerate `parser.go` from `parser.go.y`

`parser.go.y` is stale — it still uses `[]string` where the generated `parser.go` correctly uses `[]*Token`. Running `goyacc` from the `.y` source will regress the Token work. Edit `parser.go` directly if grammar changes are needed; update `parser.go.y` to match afterward.

## `ParseForCompletion`

Trims trailing whitespace; if the source ends with `.`, injects `__cursor__` immediately after the dot. Only handles the trailing-dot case. Used exclusively by `jq-lsp` for dot-triggered completions.

## Staying in sync with upstream

```sh
git fetch origin
git rebase origin/master
```

Conflicts are typically in `lexer.go` and `parser.go`. Do not run `make update` (it regenerates from upstream's build system and would overwrite Modo changes).

## UTF-16 position correctness

All AST string fields are `*Token` with `Start`/`Stop` as flat UTF-16 code-unit offsets (not line/column). The lookup table is built over the full source at lex time. There are currently no tests for non-ASCII input — be careful when handling multi-byte characters.
