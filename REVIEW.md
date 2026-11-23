# codecore — review and decision log

A log of the decisions that shaped this tool, kept short on purpose.
The full routing table and the per-language test results live in
`ROUTING.md`; the test database checklist is in `testdb/CHECKLIST.md`.
The primary consumer is a coding agent / LLM that must grasp a script
at minimum token cost while maximizing abstract-level understanding.

## 1. Background: three engines, one goal

codecore started as a Go-only `go/ast` walker with a kind taxonomy
(`Symbol{Kind, Name, Detail, Span}`), then was rewritten to
tree-sitter with per-language rule packs (Jo chose that over the
alternatives on 2026-11-23). Jo then reframed the goal: output the
bigger structure the way you get it by manually collapsing all code
blocks — no need to say which is a function and which is a variable,
open one depth at a time. The tree-sitter build was the wrong shape
for that (it classified every symbol, and its grammar gaps lost real
lines on pp.ps1), so it was replaced by the fold model below. The
tree-sitter review, research notes and brainstorm that led here are
in git history; only the conclusions that still bind are kept here.

## 2. Where the old engine fell short — and whether it still holds

The twelve cons from the original review, with verdicts from the
2026-11-24 re-check against the fold model. Six are gone or moot;
the rest are live decisions.

GONE / MOOT (6): (1) Go-only by construction — no, 34 languages /
3 parsers, adding one is a registration line. (2) Fails loudly on
syntax errors — no, the fold model does not parse syntax and never
exits 1 on a bad file. (3) No partial outlines — no, unbalanced
files yield partial outlines with a warning, rc 0. (5) Kind
taxonomy is ours alone — moot, the kind model was deleted.
(6) Signatures re-printed, not sliced — gone, `go/printer` is
deleted; the outline shows verbatim source lines (trimmed, capped
120 chars). (11) `recv` leaks into the model — moot, the Symbol
model is gone; Node is Line/Text/Children only.

STILL TRUE (4): (4) No cross-file awareness — single-file outlines,
no package view, no import/call edges. It fights the fold model's
philosophy (it needs a semantic layer); the only cheap version that
fits is a repo outline (the file list as a top level). (8) No doc
comments — true as a fact, but by design (Jo's "comments are noise"
rule); the one fitting option is an opt-in doc-first-line flag.
(9) No ranking or token budget — true; the depth limit IS the budget
mechanism Jo chose when removing --full; a real --max-tokens needs
ranking the fold model lacks; the only fitting version is a
`--max-lines` cap. (10) No caching / incremental parse — true but
irrelevant: at ~20 µs/file, re-parsing is cheaper than a cache
lookup.

MOOT IN PRACTICE (1): (7) No visibility flag — no flag because there
is no kind model, but the information is in the line text itself
(capital letter in Go, `pub` in Rust); structuring it would mean
reintroducing kind labels, which Jo explicitly removed.

PARTLY TRUE (1): (12) Single-file CLI — directory mode exists now
(non-recursive, sorted), so not single-file anymore, but flat: no
recursion, no package view. Recursive directory walking is the
cheap, obvious gap; package view is the deeper version.

The open decisions are #4 (leave it, or the cheap repo-outline
idea), #9 (a `--max-lines` cap, if Jo wants a hard budget), #12
(recursive directory walking, the cheap one), and #8 (opt-in
doc-first-line, if Jo wants it).

## 3. Rebuild: the fold model replaces tree-sitter (2026-11-23)

A prototype (`~/workspace/fold-prototype/fold.py`, Python stdlib
only) proved the simple approach first: indentation for Python,
brackets for Go/PowerShell, no kind labels, `--depth N` viewing.
Compared against the tree-sitter build on pp.ps1 (2,565 lines), it
missed zero multi-line blocks, dropped zero of 781 symbols, matched
max depth (14 = 14 on pp.ps1), and recovered the three assignments
the grammar lost (L:2381–2383) plus the string-labeled `switch`
clauses. The rebuild ports that prototype to Go (`fold.go`,
`fold_python.go`, `fold_braces.go`) and deletes the tree-sitter
engine, rule packs and all kind-based code. `go.mod` now has zero
dependencies — no CGo, no C compiler, no network on first run.
Measured speed on the fixtures: Go ~20 µs, Python ~24 µs,
PowerShell ~14 µs per file. Output is byte-identical to the Python
prototype on every file checked. One real bug was found and fixed
during the port: a `''` escaped quote in a PowerShell literal string
was swallowing the rest of the line (regression test
TestFoldEscapedQuote).

Jo's two principles, from the failure-case demos: multi-line
constructs hide their contents, and one level of structure at a
time (proper nesting, never flattened). The pure-indent Python
parser and the lone-opener attachment in the brace parser follow
from those. `--full` was removed at the same time — the tool
excels at the depth-limited view instead.

## 4. Fix log: test database and routing (2026-11-23 to 2026-11-24)

Thirty-three languages were tested, three files each, one at a time,
against real code from GitHub (see `ROUTING.md` for the full table,
the per-language evidence, and the issues left as-is by Jo's call).
The fixes made during that work, in order:

- Docstring lines are not shown (Python). C-style comments are not
  shown by the indent parser. A split header is one node, not two
  (brace parser). A `#[...]` attribute is not a comment (Rust).
  PHP tags (`<?php`, `?>`) are not shown. Lua/Ruby `--` comments
  and `end` are not shown. Preprocessor directives (`#include`,
  `#import`) are statements, not comments. Nested `/* */` block
  comments are hidden (SQL). `%` comments are hidden, Ruby `%`
  literals are kept (MATLAB). HTML/XML closing tags and `<!-- -->`
  comments are hidden (indent parser).
- Routing table: one parser per language — brace (18), indent (14),
  tag (2: HTML, XML). The tag parser was Gemini-generated from
  Hana's prompt, then fixed by Hana for two bugs (comment+code on
  one line lost; multi-line void-element attributes lost) and wired
  in. Default depth 2 for HTML/XML (their top level is a single
  wrapper tag).
- Left as-is by Jo's call: the indent-parser split header
  (Python/Swift/Julia), C Allman `{` with a trailing comment, Bash
  heredocs and shell keywords, LaTeX `\end{...}` lines, C++ flat
  namespaces in the indent parser, Java annotations and CSS `@media`
  merging in the indent parser (both languages route to the brace
  parser, so unaffected), Clojure tail lines in the brace parser
  (Clojure routes to indent, so unaffected).

Verification: the default run matches the routed parser's forced
run on 102 files, 0 mismatched. Fixtures: sample.py 34, sample.go
42, sample.ps1 28, pp.ps1 1494.

## 5. Deterministic retrieval: --ask (2026-11-24)

Jo picked this from the brainstorm's wilder ideas: a tiny
deterministic retriever on top of the outline, no embeddings.
`--ask "query"` ranks the outline nodes whose line text best
matches the query's words, using BM25. `--top N` caps the
results (default 5). `--format json` emits the same results as
a JSON array. `--depth` is ignored (results are flat, not trees);
`--lang` still applies.

How it works: every outline node is a document, its text is the
node's line text, and the parent chain is the node's ancestors'
texts (root first). Query and node text are tokenized the same
way — lowercased, split on any run of non-letters/non-digits
(`authenticate_user` becomes `authenticate`, `user`). BM25 with
k1 = 1.2, b = 0.75; IDF is ln(1 + (N - df + 0.5) / (df + 0.5)).
Only nodes scoring above 0 are candidates; ties break by file
then line, so output is deterministic. Each result carries its
file, line, text, score, and parent chain. No matches prints
`no matches for "<query>"` (text) or `[]` (JSON), exit 0. A
query with no searchable terms and `--top` below 1 are flag
errors (exit 2).

Implementation: `retrieve.go` (tokenize, BM25 index, ranking,
text/JSON rendering) plus `--ask`/`--top` flags and help text in
`main.go`. The outline path is untouched — the `--ask` path
reuses the same outline-building loop. No name extraction and no
end-line ranges in this version (the minimal version Jo chose):
results give the header line only, and the agent reads from there.

This was the first feature built with Codex CLI (Jo asked for
it to assist). It took three attempts: the baked-in Codex
(v0.164.0) could not run at all — its image lacks the
`codex-code-mode-host` binary, so it could not touch the
workspace; the npm Codex (v0.171.0) read the source fine but
crashed with a Rust panic when it dumped ~73 KB into the output
pipe faster than it could be drained. The third attempt (npm
Codex, output redirected to a log file) wrote `retrieve.go`,
`retrieve_test.go`, `main.go`, and the README "Ask" section —
then a VM restart cut it off before verification, wiping Go
and /tmp. Hana reinstalled Go, reviewed the code, and verified
it herself. Lesson: use `/usr/bin/codex` (npm, v0.171.0), never
the baked-in one, and always redirect its output to a file.

Tests: `TestTokenize`, `TestBM25Scoring` (rare term beats common,
short doc beats long doc with the same frequency, identical docs
tie), `TestRetrieveRanking` (order, file/line tie-break, top-N,
no-match), `TestRetrieveFromExtractor` (Python source, top result
and parent chain). gofmt/vet/test/race all pass; fixtures and
pp.ps1 unchanged (34/42/28/1494). `--ask "sort"` on the Python
sorts and `--ask "keymap"` on the real Lua files both return
sensible ranked results with parent chains.

Limits, as designed: it matches words, not meaning — a query
must contain the actual terms (`auth` will not find `login`),
and a partial identifier will not match (`auth` will not find
`authenticate`) — since fixed by prefix matching, see section 8. Name extraction and end-line ranges were the
obvious next steps; both are now done, in sections 6 and 7.

## 6. Retrieval ranges: --ask results carry an end line (2026-11-24)

Jo: "Ranges first." The minimal `--ask` gave the header line
only; now every result also carries the end line of its block,
so the agent knows exactly which lines to read.

The end line is computed from the tree, not from the file: it
is the greatest line number among the node and all its
descendants (the last outline line in the block). A leaf's end
is its own line. No parser changes and no new Outline fields —
`retrieve.go` gains a small `endLine(node)` helper, and each
retrieval document stores its end when it is built. The range
covers exactly the outline lines belonging to the block.

Two consequences of that definition, both honest. For brace
languages the closing brace line is not an outline node, so it
is not included: a Go function's range ends at its last content
line, not at the `}`. And for the last node in a file the end is
still exact, because it comes from the tree, not from the file's
total lines — no over-reading of trailing comments or blank
lines. An exact source span (including the closer) would need
the parsers to record block ends at extraction time; that is a
bigger change, and the tree-derived range is the right first
step.

Output: text shows `[L:50-55]` for a block and `[L:50]` for a
single line; JSON always carries an `end` field alongside
`line`. The scoring is untouched — ranges change what a result
points at, not how it ranks.

This one Hana built directly (small, well-defined change; the
Codex round-trips cost more than the code). Tests:
`TestRetrieveRanges` (a block's end is its last descendant's
line; a leaf's end is its own line) and
`TestFormatResultsRanges` (text shows `[L:1-4]` for a block and
`[L:5]` for a single line; JSON carries both ends). gofmt/vet/
test/race all pass; fixtures and pp.ps1 unchanged
(34/42/28/1494). `--ask "keymap"` on the real Lua files now
shows `function keymap.set [L:66-112]` and `function keymap.del
[L:139-166]` — the ranges are the functions' real spans.

Still to come: name matching (§5's other half).

## 7. Name matching: --ask ranks definitions above uses (2026-11-24)

Jo: "Let's move on to the name matching." The minimal `--ask`
ranked on line text only, so a definition and a call to it scored
the same. Now each node carries a name, and a definition's name
counts extra in BM25, so asking for `quick_sort` finds
`def quick_sort` first, not the calls to it.

The name is extracted by a small language-agnostic heuristic, not
by per-language rules: it is the last identifier before the first
delimiter — `(`, `[`, `=`, `:` or `{` — extended backwards over
".identifier" segments so a method keeps its dotted path
(`keymap.set`, not just `set`). For a definition it is the defined
name; for a call it is the called function. A line counts as a
definition when its first identifier is a definition keyword
(def, func, function, fn, class, struct, interface, type, let,
var, const, local, …) or when it assigns with a single "="
(not "==", "!=", "<=" or ">=") — so `x = 1` defines `x`, while
`if x == y:` does not. Only definitions get the name boost; a
use still carries its name in the output, it just does not get
the extra weight.

Two bugs found by testing the real output, both fixed. First, a
generic broke the heuristic: in
`def quick_sort[T: Comparable](...)` the ":" inside the brackets
is not the header colon, and the scan stopped there, naming the
line "T". A "[" or "{" is now a delimiter only when attached to
a name (immediately after an identifier, a ")", or a ":") — a
standalone list or dict literal is not, so the name is found
inside it (`return [*quick_sort(...)]` names `quick_sort`, not
`return`). Second, the boost was cancelling itself out: the
boosted name tokens were counted in the document length, so a
long definition line was penalised for the boost meant to help
it, and the recursive call outranked the definition. The boost
now raises term frequency only; the document length is the
plain line length.

The name is carried in the JSON output as a `name` field
(omitted when a line has no usable name); the text output is
unchanged, since the name is visible in the line text. The
scoring change is confined to retrieval — the outline itself,
its fixtures, and its counts are untouched.

Known limits, as designed. A Go method with a receiver
(`func (r *Receiver) Method()`) has no usable name: the first
"(" belongs to the receiver, so the heuristic lands on "func"
and there is no earlier identifier to fall back to. The name is
the last identifier, so a line with several calls names only the
last one. And the doc-first-line half of the original idea is
still open: docstrings and doc comments are hidden by design,
so capturing them for ranking would need the parsers to record
them — a separate change.

Tests: `TestExtractName` (definitions, calls, assignments,
dotted methods, generics, comparisons, the receiver case) and
`TestRetrieveNameBoost` (a definition outranks a use of the same
name, and the name is in the JSON). gofmt/vet/test/race all
pass; fixtures and pp.ps1 unchanged (34/42/28/1494); real Lua
keymap.lua still 50 nodes. `--ask "quick_sort"` on the Python
sorts now puts `def quick_sort [L:21-50]` first (6.58), above
the recursive call (6.00) and the print call (5.54); `--ask
"keymap set"` puts `function keymap.set [L:66-112]` first
(10.60). That completes the full version from §5 — BM25 over
the outline tree, with names and ranges.

## 8. Forgiving queries: stopwords and prefix matching (2026-11-24)

Jo: "No embeddings, let's patch it with the cheap approach." Name
matching felt minor, so retrieval was made forgiving of how it is
asked instead of requiring perfect queries.

Two small changes. First, query stopwords: the question words a
natural-language query adds (where, is, how, the, …) are stripped
from the query only, never from the code, so "where is auth
handled" searches for "auth handled". Action words (find, get,
show, list) are kept — they can be part of a function name. A
query that is only stopwords is an error ("must contain a
searchable term"). Second, prefix matching: a query term of 3 or
more letters also matches any longer term it prefixes, so "auth"
finds "authenticate" and "bub" finds "bubble_sort". Shorter terms
match exactly only. The document frequency is the union over the
matched terms, and a document is credited once for its strongest
match, not once per matched word.

The README "Ask" section gains a "How to ask" guide: query with
code terms first, try a shorter prefix or a related word when a
term finds nothing, and ask again with a narrower term rather than
a longer sentence.

Tests: `TestFilterQueryTerms` and `TestRetrievePrefixMatch`
("auth" finds `authenticate_user`, "aut" does, "au" does not).
gofmt/vet/test/race all pass; fixtures and pp.ps1 unchanged
(34/42/28/1494). `--ask "where is auth handled"` now finds
`def authenticate_user` first; `--ask "how do I sort a list"`
finds the sort functions; `--ask "bub"` finds `bubble_sort`.

Still open: the doc-first-line half of the original idea
(docstrings are hidden by design, so capturing them for ranking
would need the parsers to record them), and the open decisions
in section 2 (#4, #8, #9, #12).

## 9. Recursive directory walking (2026-11-24)

Jo: "Yes let's do that." The directory mode was flat — a directory
expanded to its top-level files only. Now `collectFiles` walks
recursively with `filepath.WalkDir`, so `codecore testdb` outlines
all 110 code files across every language subdirectory, and
`--ask "sort" testdb` searches the whole tree at once. That is the
real win: --ask becomes a repo-wide search, with the ranking
computed across all files.

Two filters keep the walk usable. Only files with a registered
extractor extension are included; the rest are skipped silently,
because a repo holds many non-code files and erroring on each
would be noise. And a short hardcoded skip list is never descended
into: `.git`, `node_modules`, `__pycache__`, `.venv`, `venv`,
`vendor`, `target`, `dist`, `build`. The root directory itself is
never skipped, even when its own name is in the list — passing
`build` directly walks it. A file passed directly is always kept,
even without a registered extension; the extractor reports it
when the file is read. Symlinks are not followed, so a symlink
cycle cannot loop the walk. The collected files are sorted by
path, and overlapping inputs are deduplicated, so the output does
not depend on the order the paths were typed.

The outline and retrieval code did not change — they just receive
more files. Respecting `.gitignore` would be the more thorough
version of the skip list; that is a follow-up, not part of this
cut. A package view (which file imports which, call edges across
files) is a separate, deeper feature — the walk is only the
file-finding layer underneath it.

Tests: `TestCollectFilesRecursive` (a nested tree, junk dirs and
unregistered files skipped), `TestCollectFilesDirectFileKept`,
`TestCollectFilesRootSkipDirKept`, `TestCollectFilesSorted`
(overlapping inputs deduplicated). The sorted test caught a real
behaviour gap: passing a directory and one of its subdirectories
duplicated the subdirectory's files, so `collectFiles` now
deduplicates. gofmt/vet/test/race all pass; fixtures and pp.ps1
unchanged (34/42/28/1494). `--ask "sort" testdb` now returns
matches from MATLAB, Ruby, and Rust files in one ranked list;
`--ask "keymap" testdb` finds the real-Lua keymap files. This
closes con #12's cheap half (recursive walking); the package-view
half stays open, as does #4.
