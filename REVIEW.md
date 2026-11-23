# codecore — parsing review & brainstorm

Date: 2026-11-23. No code changed for this review. Research report (60 KB, with
sources): `~/workspace/research_notes/tree-sitter-incremental-parsing-20261123-0631/report.md`.

Key focus, per Jo: **the primary consumer is a coding agent / LLM that must
grasp a script at minimum token cost while maximizing abstract-level
understanding.**

## 1. How the current parsing logic works

`lang_go.go` (Go only) uses stdlib `go/parser` + `go/ast` with
`SkipObjectResolution` (no scope resolution, no `go/types`). One pass over
`File.Decls`: `GenDecl` specs become import/type/var/const symbols, `FuncDecl`
becomes func/method symbols; struct fields and interface methods become
children. A second pass nests methods under their receiver type (types
indexed by name first — the same two-pass trick gotags uses). Spans come from
the token file set (exact byte + line). Signatures are rendered from the AST:
`go/printer` for expressions, hand-rolled field-list rendering for params and
results (the printer refuses a bare `*ast.FieldList` — found the hard way).
`outline.go` holds the shared model (`Symbol{Kind, Name, Detail, Span,
Children}`, dot-separated kinds) plus the `Extractor` registry. Measured:
~79 µs to extract the ~90-line fixture; `gofmt`/`go vet`/tests green.

## 2. What the current logic does well

- **Correct foundation.** `go/parser` is the reference Go parser: exact,
  comment/string-safe by construction, zero grammar risk. go-outline and
  gotags do the same thing; we are in good company.
- **Fast, honestly fast.** Single AST walk, no type checking, measured not
  claimed.
- **Deterministic spans.** Byte + line, half-open bytes, source order. An
  agent can point at exact ranges.
- **Clean layering.** Model / extraction / formatting are separate; the
  registry picks extractors by extension or language name; a language lives
  in its own file.
- **Method nesting works.** Two-pass types-first, children in source order,
  `[L:]` still records the true line.
- **Tests assert the right invariants.** Trap names in strings/comments never
  leak, order is source order, spans are sane, plus a benchmark.

## 3. Where it falls short (honest)

1. **Go-only by construction.** Each new language needs a whole new
   hand-written walker (~300 lines). Marginal cost per language is a full
   extractor, not a rule file. This is the core expandability problem.
2. **Fails loudly on syntax errors.** `go/parser` returns a partial AST on
   error, but we discard it and exit 1. Real code being edited is often
   broken; Repomix/Aider never fail on one bad file; tree-sitter always
   returns a tree with `ERROR` nodes.
3. **No partial outlines.** Consequence of (2). For an agent reading code
   mid-edit, this is a real gap.
4. **No cross-file awareness.** Methods on types from other files stay
   top-level; no package view; no import/call edges.
5. **Kind taxonomy is ours alone.** Dot-kinds are readable, but they map to
   no standard. LSP `SymbolKind` (1–26), ast-grep `symbolType`, and
   tree-sitter `tags.scm` captures are the shared taxonomies; an LLM-side
   consumer that knows LSP kinds needs a translation table we don't ship.
6. **Signatures are re-printed, not sliced.** `go/printer` normalises
   formatting, so the signature an agent sees can differ from the source
   text. Every surveyed tool (go-outline, gotags, ast-grep, Repomix, Aider)
   slices verbatim source lines by range instead — cheaper and more
   faithful. Our printer path is also the one place that already bit us
   (FieldList) and it does not generalise to other languages.
7. **No visibility flag.** `isExported`/`isPublic` is one capital-letter
   check in Go and high-signal for an agent; ast-grep ships it, we don't.
8. **No doc comments.** We parse without `ParseComments`, so the cheapest
   semantic signal per token is left on the table. (Docstrings alone are
   weak — 6/45 on behavioural probes — but they are nearly free.)
9. **No ranking or token budget.** Output grows with file size; nothing
   prioritises or caps it. TokenEconomizer (Oct 2026) shows a fixed outline
   injection can *raise* total task cost, because every turn re-reads it.
10. **No caching / incremental parse.** Whole file re-parsed every run.
    Fine for one-shot CLI; wasteful for repeated agent reads.
11. **`recv` leaks into the model.** The unexported receiver field on
    `Symbol` is how nesting works — it stays out of JSON, but the model
    carries extractor plumbing.
12. **Single-file CLI.** Directory mode is flat; no package/repo mode.

## 4. What the research says (2025–2026)

- **Tree-sitter 0.27.x (ABI 15)** is the settled universal parser: GLR,
  incremental re-parse via `tree.edit()` reusing untouched subtrees,
  error recovery scoring branches by `error_cost` with `ERROR`/`MISSING`
  nodes, and an S-expression query language with captures (`@name`) and
  predicates. Official Go binding `github.com/tree-sitter/go-tree-sitter`
  (CGo, Go 1.23+, grammars as modules) is the preferred embedding;
  `smacker/go-tree-sitter` is mature but ~22 months stale; pure-Go
  `gotreesitter` has wildly conflicting benchmarks (vendor ~1.5× faster vs
  independent eval 12–7,500× slower on large files) — do not adopt on
  vendor claims alone.
- **ast-grep `outline` (0.44.0, June 2026) is the model to copy.** Entries
  are `{role: item|member, symbolType, name, range, signature, astKind,
  isImport/isExported/isPublic}`; items are top-level, members attach via
  `parentRuleIds`. Its Go pack is **10 YAML rules**. Adding a language is
  one declarative file + tests, not a walker.
- **LSP `documentSymbol` (spec 3.17)** is the semantic complement, not a
  replacement: `DocumentSymbol{name, detail, kind, range, selectionRange,
  children[]}` or flat `SymbolInformation`; `SymbolKind` 1–26 is the most
  standardised shared taxonomy. Limits: one server per language, 15–120 s
  startup, pooling essential — an opt-in layer, never the default path.
- **For an LLM, the outline is a navigation primitive, not a summary.**
  Aider RepoMap (canonical reference): tree-sitter `tags.scm` def/ref →
  file graph → personalised PageRank → binary-search into a hard token
  budget (default 1,024) → render verbatim signature lines with `⋮`
  elision. Sam-Bodden (June 2026, Claude Sonnet 4.6, N=70): structured
  tiers 23/70 at ~26k tokens vs full source 19/70 at ~94k; keep-only-
  relevant-sections 25/70 at ~19k. Behavioural probes: full source 27/45
  (60%); frontier-model summary 4/45 (9%); signature+docstring 6/45.
  **Do not emit natural-language summaries as a substitute for source.**
- **cAST / Symbols-Not-Chunks:** AST-shaped retrieval beats fixed-size
  chunking (Recall@5 +4.3; 1.6–3.9× fewer tokens per query). Deterministic
  symbol retrieval beats fixed chunks.
- **Survey lessons:** every successful tool separates (a) parser, (b)
  declarative per-language extraction spec, (c) small shared output row.
  Signatures come from source slices. Fallbacks are part of the design
  (Repomix: compress failure → uncompressed; Aider: no refs → Pygments
  lexer). Ranking is a separate layer from extraction.
- **Do not build a bespoke per-language binding DSL.** GitHub's
  stack-graphs was archived 2025-09-09 largely for that maintenance
  burden. **Do not link GPL code** (Universal Ctags: subprocess only).

## 5. Brainstorm — novel ideas, ranked

### A. The expandability unlock (do first)

1. **Declarative rule packs per language.** Replace the hand-written walker
   pattern with a small rule schema (node kind → `symbolType`, name
   capture, parent rules, visibility) — data, not code. New language = one
   rule file + fixture + test. This is the ast-grep insight and the single
   biggest structural change. The Go pack can be bootstrapped by
   translating `lang_go.go`'s type-switch into rules and verifying against
   the existing fixtures.
2. **Tree-sitter as the universal parser; keep `go/ast` as the Go oracle.**
   Migrate breadth to tree-sitter (official `go-tree-sitter`), keep the
   current Go extractor as a verification oracle in tests: parse every Go
   fixture with both, diff the symbol sets. Catches grammar drift and
   proves the rule packs correct. Two parsers disagreeing is a feature,
   not a bug.
3. **Verbatim signature slicing.** Take signatures from source text by byte
   range instead of re-printing via `go/printer`. Cheaper, faithful, and it
   generalises to every language (tree-sitter hands you the range; slice
   it). Kills the only Go-specific rendering code.

### B. Robustness (cheap, high value)

4. **Error-tolerant partial outlines.** On parse error, emit what parsed
   plus a `has_error` flag and error count instead of exiting 1. The
   parser already returns the partial AST; we just throw it away today.
5. **Fallback ladder per file.** Tree-sitter rules → regex/optlib pack →
   plain-text fallback. Never fail the run on one bad file (Repomix rule).
6. **Incremental outline cache.** mtime+size keyed disk cache of outlines
   (Aider's `.aider.tags.cache` pattern), so repeated agent reads of
   unchanged files cost ~0. Later: tree-sitter incremental re-parse for
   edited files.

### C. LLM / token-first output (the key focus)

7. **Multi-resolution views.** `--view names|signatures|expanded`
   (ast-grep's views). Names-only is ~5–10× fewer tokens than signatures;
   the agent zooms in on demand. Default could be names for huge files.
8. **Hard token budget.** `--max-tokens N`: rank symbols (exported first,
   then by in-file reference count) and binary-search the ranked list into
   the budget (Aider's trick), with a trailing line saying what was cut
   and how to get it (`--full`). Directly serves "minimum token cost".
9. **Stable symbol IDs + range fetch.** Each symbol gets a stable ID
   (file + kind + name + line, or a hash of its range) so an agent can
   refer to symbols across turns and fetch exact bodies by range
   (`codecore show file.go --range 120:150`, or by symbol). Turns the
   outline from text into an addressable index. This is the keep/drop
   finding made concrete: keep only relevant sections (25/70 at ~19k
   tokens vs 19/70 at ~94k for full source).
10. **TreeContext-style rendering.** For a symbol, print enclosing scope
    headers + the signature with `⋮` elision — nesting context without the
    body (grep-ast `TreeContext`, directly reusable idea).
11. **Visibility + doc-first-line.** `isExported` flag and the doc comment's
    first line, at most. Nearly free, highest signal per token available.
12. **Edges as a second layer.** Import graph + call edges (`--edges`) as
    optional output — the RepoGraph/cAST finding is that def/ref graphs
    beat flat outlines for retrieval. Keep extraction total and
    deterministic; ranking stays a separate, optional pass.
13. **Measure end-to-end, not outline size.** Build a tiny eval: tokens per
    resolved task on a few scripted agent tasks (localise → edit). A
    smaller outline is not automatically a cheaper task (TokenEconomizer).

### D. Expansion mechanics

14. **Shared taxonomy mapped to LSP `SymbolKind`.** Keep dot-kinds for
    display, add an LSP-compatible numeric `kind` + raw `astKind`. Any
    consumer that speaks LSP understands our output with no translation.
15. **Rule-pack tests as the contract.** Each language ships a fixture file
    + expected outline (like `testdata/sample.go` today). The test IS the
    spec; adding a language = fixture + rules + one test.
16. **Grammar-as-dependency.** Tree-sitter grammars as Go modules
    (official bindings) or a WASM bundle (Repomix model: no build tools at
    runtime, accepts the overhead).
17. **LSP as an opt-in semantic layer.** `--semantic` flag, pooled server,
    only when asked — types, cross-file definitions. Never default
    (15–120 s startup).
18. **SCIP export.** An output format emitting SCIP-compatible symbol
    occurrences, so our outlines are consumable by Sourcegraph-class
    tools. SCIP is a good export format, a bad dependency.

### E. Wilder ideas

19. **Outline diff.** `codecore diff old.go new.go` — structural diff
    (symbols added/removed/moved) for agents reviewing changes; cheaper
    than re-reading the whole file.
20. **Deterministic retrieval mode.** `--ask "where is auth handled"` —
    rank symbols by name/doc match (BM25 over names + doc first lines) and
    return the top symbols with ranges. A tiny deterministic retriever on
    top of the outline, no embeddings.
21. **Outline-aware chunking for RAG.** Emit AST-shaped chunks (whole
    symbols, never split mid-function) instead of fixed-size chunks —
    the cAST result (Recall@5 +4.3) applied to our own output.

## 6. Suggested order (if Jo says go)

1. Rule-pack extraction model + verbatim slicing + shared LSP-mapped kinds
   (the architecture change; Go first, proven against current fixtures).
2. Error-tolerant partial outlines + `has_error` + fallback ladder.
3. Visibility + doc-first-line + multi-resolution views + `--max-tokens`.
4. Stable symbol IDs + range fetch (the index).
5. Tree-sitter migration for language #2 (Python is the natural next
   target: biggest grammar, best `tags.scm` coverage), `go/ast` kept as
   the Go oracle.
6. Edges, cache, LSP opt-in, SCIP export — in that order, each optional.

## 7. What NOT to do

- Bespoke per-language binding DSL (stack-graphs lesson).
- NL summaries as a substitute for source (4/45 vs 27/45).
- Fixed outline injection assumed to save tokens (measure end-to-end).
- Pure-Go `gotreesitter` on vendor benchmarks alone (independent eval
  disagrees by orders of magnitude — benchmark it ourselves first).
- GPL code linked in (ctags: subprocess only, if ever).

## 8. Key sources

- Tree-sitter docs & releases: https://tree-sitter.github.io/tree-sitter/
  and https://github.com/tree-sitter/tree-sitter/releases
- ast-grep outline: https://ast-grep.github.io/guide/outline-code and
  https://ast-grep.github.io/blog/ast-grep-outline.html
- LSP 3.17 documentSymbol: https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#textDocument_documentSymbol
- Aider RepoMap: https://aider.chat/2023/10/22/repomap.html
- Sam-Bodden, "What Context Does a Coding Agent Actually Need to Act?",
  arXiv:2607.09691
- TokenEconomizer, arXiv:2610.22732
- cAST, arXiv:2506.15655; RepoGraph, arXiv:2410.14684
- Tool sources read live: ramya-rao-a/go-outline, jstemmer/gotags,
  Aider-AI/aider repomap.py, yamadashy/repomix parseFile.ts,
  ast-grep default_rules/go.yml, universal-ctags parsers/go.c
- Full research report with per-source notes:
  `~/workspace/research_notes/tree-sitter-incremental-parsing-20261123-0631/report.md`

## 9. Status: implemented 2026-11-23 (option A)

Jo chose tree-sitter for all three languages. Implemented the same day:

- Engine: `rules.go` (Rule, RulePack, TreeSitterExtractor). One engine,
  one mechanism; rules are queries + name/detail/parenting facts.
- Packs: `lang_go.go` (walker output preserved exactly — same kinds,
  names, details, so the original fixtures/tests hold),
  `lang_python.go`, `lang_powershell.go` (airbus-cert grammar v0.26.6).
- Model: Symbol gained `symbolType` (LSP-mapped), `astKind`,
  `exported`; Outline gained `hasError`. Broken files now yield partial
  outlines (rc 0 + stderr warning) instead of failing; empty files yield
  empty outlines.
- Gotchas found live and fixed: a query pattern with a repeated capture
  yields one match per binding — the engine merges bindings of the same
  node (captures unioned, source-ordered); `#not-kind?` is accepted but
  does not filter, so Go's type_spec is one rule with a classifier func;
  `command_elements` is a field holding a `command_elements` node, not a
  direct child; `go mod tidy` needs the requires to exist first or it
  re-resolves the powershell `bindings/go` path from the proxy and chokes
  on a stale nested go.mod (run `go get` first, then tidy).
- Speed (this machine, fixtures): Go ~1.8 ms, Python ~1.0 ms,
  PowerShell ~1.1 ms per file, vs ~79 µs for the old Go walker. Parse
  alone is ~440 µs; the rest is query matching + per-symbol CGo access.
  Two cheap optimisations tried (one shared QueryCursor, cached capture
  names) saved ~60 µs total. ~500 files/sec is ample for outlining;
  further speed work would trade away rule readability.
- Still open from the review: multi-resolution views + token budget,
  stable symbol IDs + range fetch, import/call edges, outline cache,
  LSP opt-in layer, SCIP export, doc comments. None started.

## 10. Default view: collapsed by kind (2026-11-23)

Jo: the default view was too detailed — every import, var and const
listed one by one. Her rule of thumb: follow the root structure; if a
block of code could be unfolded, don't show the rest. So the default
text view now collapses by kind: consecutive top-level imports,
consts, vars, types or classes fold into one `import { ... }` /
`const { ... }` / `var { ... }` / `type { ... }` / `class { ... }`
line, tagged with the first item's line; a lone item stays expanded,
and functions/methods never fold (the signature is the outline item).
`--full` and JSON are unchanged — everything unfolded. Implemented in
the text formatter only (`format_text.go`, `foldKind`), so the model
and the extractors are untouched. Go fixture default: 19 lines → 8;
Python 15 → 7; PowerShell 9 → 6.

## 11. Error report: import names, block nesting, parse errors (2026-11-23)

Jo ran the collapsed-view build on her own scripts and reported three
things: `Import-Module -Name $var ... -Scope Global` showed up as
`import "Global, Stop"`; `--full` on a Python script showed only
imports and functions; and a 2,565-line procedural PowerShell script
(pp.ps1, no functions) came out as one flat list of 500 top-level
symbols with a "parse errors" warning. Diagnosis, all verified live:

- **Import name bug (fixed).** The import query captured every
  bare-word `generic_token` in the command and the engine joined them
  with ", ", so the flags' values became the module name. The rule
  now captures the whole `command_elements` and a NameFn
  (`pwshImportName`) reads the module: the first bare word, or the
  value after `-Name` (literal, `$variable`, or quoted string);
  anything else yields no import rather than a wrong name. A variable
  module renders bare (`import $modPath`, not `import "$modPath"`).
- **Flatness (fixed).** Variables and functions at script level had
  no parent to attach to, so they flattened to the top level — the
  pp.ps1 flatness and the Python `--full` gap are the same cause.
  New `block` rules (Python: if/for/while/try/with; PowerShell:
  if/foreach/for/while/switch/try/scriptblock) give them a parent:
  the block's header is its name (`if ($x)`, `try:`, `scriptblock`
  for a bare scriptblock) and variables/functions nest under their
  enclosing block. Nested functions nest under their enclosing
  function too (the ancestor index now covers members, not just
  items; the key index stays items-only so Go method attachment is
  unaffected). Function locals are still excluded, as before, and Go
  is untouched. In the default view blocks are transparent — their
  children show at depth 0 and fold with the surrounding vars — so
  the default view is unchanged (pp.ps1 still 5 lines; fixtures:
  Go 8 default / 29 --full, Python 7 / 23, PowerShell 6 / 15 — the
  --full counts grew by the new block rows only).
- **Parse errors (grammar gaps, not fixable in codecore).** pp.ps1
  has 7 ERROR nodes: two C-style `/* */` comments (not valid
  PowerShell — Jo should change them to `<# #>` in her script);
  `$using:Global:` (valid PS7, the grammar can't parse it — one
  garbled detail); string-labeled `switch` clauses (`'set' {...}` —
  the grammar fails on them, `default` parses); and one unpinned
  error-recovery quirk that loses 3 assignments (L:2381–2383).
  Extraction recovers almost everything: 497 of 500 assignments
  survive. The grammar is an upstream module, so grammar fixes
  cannot ship in the zip — the limits are recorded in the README.
- The Python `--full` file Jo mentioned was not attached, so it
  could not be verified against her actual file; the same nesting
  fix covers the reported shape (a var under `if __name__ ==
  "__main__":` now nests under that block in --full).


## 12. Rebuild: the fold model replaces tree-sitter (2026-11-23)

Jo reframed the goal: codecore should output the bigger structure the
way you get it by manually collapsing all code blocks — no need to say
which is a function and which is a variable, open one depth at a time.
The tree-sitter build (§9–§11) was the wrong shape for that: it
classified every symbol and its grammar gaps lost real lines on
pp.ps1.

A prototype (`~/workspace/fold-prototype/fold.py`, Python stdlib only)
proved the simple approach first: indentation for Python, brackets for
Go/PowerShell, no kind labels, `--depth N` viewing. Compared against
the tree-sitter build on pp.ps1 (2,565 lines), rules.go, the whistle
transcription script and all three fixtures, it missed zero
multi-line blocks, dropped zero of 781 symbols, matched max depth
(14 = 14 on pp.ps1), and recovered the three assignments the grammar
lost (L:2381–2383) plus the string-labeled `switch` clauses. Its
depth-0 view is longer than the kind-folded default (Go fixture 16 vs
8) because every top-level line shows — except pp.ps1, where its 32
lines show the block skeleton the old 5 folded lines hid.

The rebuild ports that prototype to Go (`fold.go`, `fold_python.go`,
`fold_braces.go`) and deletes the tree-sitter engine, rule packs and
all kind-based code: `rules.go`, `lang_go.go`, `lang_python.go`,
`lang_powershell.go`, the `Symbol`/`Span` model, kind folding, and the
`HasError` flag (replaced by `Unbalanced` for bracket imbalance).
`go.mod` now has zero dependencies — no CGo, no C compiler, no
network on first run. Measured speed on the fixtures: Go ~20 µs,
Python ~24 µs, PowerShell ~14 µs per file (the tree-sitter build was
~1.8 ms / ~1.0 ms / ~1.1 ms). Output is byte-identical to the Python
prototype on every file checked. One real bug was found and fixed
during the port: a `''` escaped quote in a PowerShell literal string
was swallowing the rest of the line (regression test
TestFoldEscapedQuote); the prototype's equivalent check was dead code
and harmless there.

## 13. Depth-first: --full removed, pure-indent, lone-brace (2026-11-23)

Jo: showing everything will cause issues — remove the full feature
and make the tool excel at the depth-limited view. `--full` is gone;
JSON now respects `--depth` too (boundary nodes carry `"hasMore"`),
and the text marker shows how much hides inside a block
(`import ( … (4 more)`). There is no way to dump the whole tree.

Jo then clarified two principles from the failure cases, and both
became fixes the same evening:

- **A multi-line construct hides its contents.** A multi-line call's
  arguments were showing as top-level statements, because the Python
  parser only opened a block on a line ending in `:`. The `:` rule is
  gone: a line opens a block whenever the next code line is more
  indented. The call's arguments and a dict's entries now nest under
  their first line and hide at depth 0, and a bare closer line is not
  shown at all. The fixture is unchanged (36 nodes, 12 blocks); the
  whistle script gains 8 correct blocks (24 → 32) and loses one bare
  `}` node. The same rule folds Ruby and Lua — the `:` was the one
  Python-specific bit — though only `.go`, `.py`, `.ps1`, `.psm1`
  are registered.
- **One level of structure at a time.** An Allman `{` on a line of
  its own had no header, so its body flattened to the enclosing level
  (`DoThing()` showed as a sibling of `if x`). A lone opener now
  attaches its block to the previous code line, so the nesting is one
  level at a time. pp.ps1 has no lone-brace lines, so its counts are
  unchanged (1,494 nodes, 440 blocks, max depth 14). Regression
  tests: TestFoldPythonContinuations, TestFoldAllman.

## 14. Docstring lines are not shown (2026-11-23)

From the first test-database run (Python, TheAlgorithms): a
docstring's opening line showed as an outline node (`[L:1] """`,
`[L:22] """A pure Python implementation...`), while the docstring's
content was correctly hidden. Jo's rule: a line that is pure symbols
with no real content tells you nothing, so it should not be shown.

A line that is only a triple-quoted string is now skipped in the
Python parser's first pass. The string's content is still tracked,
so a multi-line docstring stays hidden. A string on a line of its
own inside a multi-line call or list is an argument, not a
docstring — the previous code line ends in an opener or a comma
there — so those lines stay. Regression test:
TestFoldPythonDocstrings. The Python fixture loses its two
docstring lines (36 → 34 nodes); the test-database files lose
theirs too (merge_sort's `def merge_sort` goes from 16 hidden
lines to 14). A bare single-quoted string statement is not skipped
yet — docstrings are triple-quoted by convention, and the
single-quote case is rare enough to leave for now.

## 15. C-style comments are not shown by the indent parser (2026-11-23)

From the JavaScript test-database run: the indent parser showed
`/* */` and `/** */` comment blocks as nodes (e.g. `[L:1] /*
Bubble Sort is an algorithm... (13 more)`), because it only
skipped `#` comments. Jo's rule again: a line that is pure symbols
with no real content tells you nothing.

The indent parser now skips C-style comments too: a `//` line, and
a `/* */` block (tracked like the triple-quoted strings, so a
multi-line block stays hidden). Only a comment at the start of a
line is treated as one — a `/*` inside a string or after code is
left alone, so a URL or a trailing comment cannot fool it.
Regression test: TestFoldPythonCStyleComments. Re-running the
JavaScript files through the indent parser now gives the same
clean output as the brace parser. (Note: `go vet ./...` and
`go test ./...` now fail on the test-database Go files, which
import an external module — use `go vet .` / `go test .` for the
root package.)

## 16. A split header is one node, not two (2026-11-23)

From the TypeScript test-database run: a multi-line arrow-function
signature showed its closer line as a separate top-level node
(`export const partition = (` L:8, then `) => {` L:12), and the
body nested under the closer line instead of under the function's
name line. Jo: "Fix the split header, then move on."

The brace parser now merges a header-continuation line into the
header above it: a line that is only leading closers, optionally
`=>`, then one opener, then nothing but a comment. Its opener
pushes the header's frame, so the body nests under the header
line. Two details: a `) {` line is not a code line (it holds only
brackets), so the merge is also tracked in the event loop for
non-code lines; and a line with real content before the closer
(`y > 0) {`) or after it (`): number {`) is not a continuation —
those keep their node. Regression test: TestFoldSplitHeader.
Re-running quick_sort.ts gives three top-level lines (partition,
choosePivot, QuickSort) with the bodies under their headers.
Fixtures and pp.ps1 are unchanged (they hold no such line).

## 17. A #[...] attribute is not a comment (2026-11-23)

From the Rust test-database run: the indent parser skipped the
`#[...]` attribute lines (`#[cfg(test)]`, `#[test]`) because it
treats any line starting with `#` as a comment. In Rust, `#`
starts an attribute, not a comment. Jo: "Yeah fix that small
issue first."

The indent parser now skips a `#` line only when it does not
start with `#[`. A `#[...]` line stays as a node, the same as
the brace parser shows it. Regression test:
TestFoldPythonAttributes. Re-running bubble_sort.rs through the
indent parser now matches the brace parser exactly (mod tests
shows 21 hidden, up from 18). The Python fixture and pp.ps1 are
unchanged.
