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

## 10. Retrieval precision: short terms, substring bonus, --def-only (2026-11-24)

Jo: "I thought there were cases in the pass test where the agent
didn't get better results using --ask vs grep, could you look into
that and figure out why." The investigation (reproduced live with
the binary, since the /tmp run files were wiped) found six
mechanisms, and three quick fixes address the sharpest ones.

The mechanisms: (1) top-N truncation — --ask returns the top 5 by
default, so a correct answer that ranks low is silently dropped
(Task D's Clojure implementation does not make the top 10, while
grep finds all 7 files mentioning "heap"); (2) false positives
from uses — --ask ranks `main.m` #2 for Task D because it calls
`heapSort(arr)`, though it does not implement it; (3) tokenization
splits compound names — `Write-Warning` becomes "write"+"warning",
so `$warningMessage` lines match; `pp_settings.json` becomes
"pp"+"settings"+"json", so `ConvertFrom-Json` and `'pp list'`
match; (4) prefix matching adds noise — "pp" matches `pp_commands`
and `pp_list`; (5) BM25 ties are arbitrary — the top 3
`Write-Warning` results all score 6.12, so ranking falls back to
line order; (6) the agent verifies with grep anyway, so --ask is
additive rather than substitutive.

The fixes, all in `retrieve.go` / `main.go`:

- **Short query terms dropped.** `filterQueryTerms` now drops terms
  under 3 letters, not just stopwords. A 1-2 letter term is too
  generic to search on — "pp" from "pp_settings" would match every
  "pp_*" identifier. This is a query-side filter only; short tokens
  stay in the code index. (An earlier idea — treating `_` as a
  token character so `pp_commands` stays one token — was rejected
  after analysis: it would break Task D, because the query term
  "heap_sort" would no longer match the Clojure file's hyphenated
  "lazy-heap-sort". Dropping short query terms fixes the "pp"
  false positive without that regression.)
- **Substring bonus.** A query term that appears verbatim (as a
  case-insensitive substring) in the node's line earns +3.0 on top
  of BM25, once per term per document. Terms under 3 letters are
  skipped for the bonus too — a 2-letter substring appears inside
  too many words (the first version of this bonus made "au" match
  "authenticate", failing `TestRetrievePrefixMatch`; the length
  guard fixed it). An exact identifier like `Write-Warning` now
  outranks a line that merely contains the word "warning".
- **`--def-only` flag.** With `--ask`, return definitions only,
  filtering out calls and uses. The `isDef` already computed by
  `extractName` is now stored on each retrieval document and
  honoured by the filter.

Measured before/after on the same queries (new binary vs the
pre-fix binary):

- `Write-Warning`: the two `$warningMessage` near-misses are gone
  from the top 5, replaced by two more real `Write-Warning` calls.
  The top 3 are still tied (12.12 each) — they are all correct
  answers, so the tie does not matter.
- `pp_settings.json`: the `pp_commands` and `'pp list'` false
  positives are gone (the "pp" term is dropped); only 3 results
  now, with the correct line still #1 by a wide margin (21.39 vs
  7.73). Two `ConvertFrom/To-Json` lines remain as lower-ranked
  near-misses (they match "json").
- `configFilePath`: order unchanged, all scores +3.00 (every result
  contains the term as a substring). No harm.
- Task C (`RunCommand AddCommand`): order unchanged, top 2 still
  tied (9.01) — a parameter declaration ahead of the implementation
  block. Not fixed by these changes.
- Task D (`heapsort heap_sort heapify sift_down`, top 10): without
  `--def-only`, `main.m` is still #2 and Clojure is still absent
  (ranks 28-29). With `--def-only`, `main.m` is gone, but Clojure
  is still absent — a blind spot in `extractName`: Lisp lines start
  with `(`, so `(defn lazy-heap-sort` gets no name and `isDef`
  false, and `--def-only` filters it out. Fixing `extractName` for
  parenthesised definitions is the follow-up, not part of this cut.
  The `--def-only` top 10 also still includes other sort files
  (MATLAB/Ruby) that match "sort" but are not heap sort.

Tests: `TestFilterQueryTermsShortTerms`, `TestRetrieveSubstringBonus`,
`TestRetrieveDefOnly`. gofmt/vet/test/race all pass; fixtures and
pp.ps1 unchanged (34/42/28/1494). Tested locally by Hana only, not
on the agent yet, per Jo's instruction.

## 11. Lisp definitions and per-file results (2026-11-24)

Jo: "Fix the two and push if you tested out fine, we'll move on
agent later." The two are the Lisp blind spot and the per-file
dedup from §10.

**Lisp definitions.** `extractName` gave a Lisp definition no name
and `isDef` false, because the line starts with `(` and the
heuristic treated that as the end of the name scan — so `(defn
lazy-heap-sort` was invisible to `--def-only` and got no name
boost. Three changes in `extractName`:

- Leading whitespace and opening parens are stripped before the
  scan, so "(defn foo" is read as "defn foo".
- `defn`, `defmacro`, `defmethod` join the definition keywords.
- The name extends backwards over `-identifier` segments as well
  as `.identifier`, so a kebab-case name keeps its hyphens
  (`lazy-heap-sort`, not just `sort`). The hyphen must sit between
  two identifier characters, so `a - b` does not extend.
- A bracket after a space is now a delimiter on a definition
  header (`(defn f [x]` ends the name at `[`), while in an
  expression it stays a literal (`return [*quick_sort(...)]`
  still names `quick_sort`, inside the brackets). The scan first
  finds the opening identifier to decide which rule applies.

**Per-file results.** `--ask` gains a `--per-file` flag: after
ranking, only the best result from each file is kept, so one file
cannot fill the whole top-N list and push other files out. The
dedup runs before the top cut, so `--top` then limits the number
of files. The default (no flag) is unchanged.

Measured on Task D (`heapsort heap_sort heapify sift_down`,
`--top 10`, testdb): without `--per-file`, Clojure ranks 28-29
and never appears. With `--per-file`, all four implementation
files land in the top 5 — Go `heapsort.go` (24.53), Java
`HeapSort.java` (23.72), Objective-C `sorts.m` (21.07), and
Clojure `heap_sort.clj` (17.45, up from 7.92 once the Lisp fix
gave it a name and a definition boost). `main.m` (a caller, not
an implementation) still takes one slot at #2; `--def-only`
removes it, but then `sorts.m` also drops out, because a C
function definition (`void heapSort(...)`) starts with its return
type, not a definition keyword, so it is not recognised as a
definition — a known gap, left open. The `--per-file` top 10
also surfaces weak matches from other files (a merge-sort file,
a sort-utils file), since it returns the best match from every
file even when that file only weakly matches; the scores make
the weak ones easy to spot.

Tests: `TestExtractName` gains Lisp cases (`(defn lazy-heap-sort`
→ `lazy-heap-sort`, true; `(defmacro with-temp [x]` → `with-temp`;
`(ns sorts.heap-sort` → `sorts.heap-sort`, false, since a
namespace declaration is not a definition); `TestRetrievePerFile`
(one result per file, the kept result from a file is its best
match). One test-writing catch: the per-file test first asserted
a fixed file order, but BM25 legitimately ranks the rarer term
higher, so it now asserts the file set instead. gofmt/vet/test/
race all pass; fixtures and pp.ps1 unchanged (34/42/28/1494).
Tested locally by Hana only, not on the agent yet, per Jo's
instruction.

## 12. Plain-text fallback: --ask works on any file (2026-11-24)

Jo: "Does the --ask and the go sub command works on any file?
They shouldn't be limited by file type right? Should we expand
that capability?" Measured first: `--ask` on a `.md` file errored
with "no extractor", `--ask` on a `.toml` file errored the same
way, `--ask` on a directory of only `.md` files silently skipped
them and printed "no matches", and codegos `go` on a `.js` file
errored "unsupported file extension". So no — `--ask` was limited
to the 34 registered languages, codegos `go` to 3.

The codecore half is built here (the codegos half — comment-syntax
families — is Jo's repo and a separate decision, not built). A new
parser, `parseText` in `fold_text.go`, folds any plain-text file by
indentation only: every non-blank line is a node, nested under the
nearest preceding less-indented line. No comment, string, or
decorator rules — a markdown heading, a TOML section header, and
a config key are all just lines, so they all show. It is
registered as language "text" for the common text extensions
(.md, .markdown, .txt, .toml, .ini, .cfg, .conf, .log, .rst,
.adoc, .text, .env, .properties), so those are included in
directory walks. And `main.go` falls back to it for a directly
passed file with no registered language, so `--ask` (and outline)
work on any text file — including one with no extension — instead
of erroring. Binary files are rejected in `Extract` (NUL bytes),
not outlined as garbage. `--lang text` forces the parser on any
file. Directory walks still only include registered extensions,
so a walk does not try to outline every unknown file it finds.

Measured: `--ask "serde"` on a `.toml` file finds `serde = "1.0"`;
`--ask "login"` on an extensionless `NOTES` file finds the
indented "fix the login bug" line under its "TODO" parent;
`--ask "BM25"` on `README.md` finds the two BM25 lines;
`--ask "retrieval" --per-file` on the codecore directory surfaces
`REVIEW.md` at #3, so `.md` files are searched in walks now.
`--lang text` on a `.py` file shows the docstring line the
Python parser hides — the expected difference, since the plain
parser has no language rules.

Tests: `TestFoldText` (headings nest, blank lines skipped),
`TestFoldTextRegistered` (text extensions route to the parser,
an unregistered extension has no extractor in the registry),
`TestFoldTextRejectsBinary`, `TestFoldTextAsk`. One existing test
changed with the behaviour: `TestCollectFilesRecursive` used to
assert that `README.md` and `d.txt` were skipped as unregistered;
they are now included as text files, so the expected list grew.
gofmt/vet/test/race all pass; fixtures and pp.ps1 unchanged
(34/42/28/1494). Tested locally by Hana only, not on the agent
yet, per Jo's instruction.

## 13. cgrep borrowings: --no-tests, --word, --filename-only, --count (2026-11-24)

Jo: "Check the tool called cgrep on GitHub and see if we can
borrow anything to our tool" (https://github.com/awgn/cgrep).
cgrep is a context-aware grep for source code (Haskell, v10, 743
stars, GPL-2.0): it tokenizes code per language and searches within
a context — code only, comments only, or strings only — because
plain grep returns 66% false positives from comments and strings.
It also has token filters (--identifier, --type, --keyword), word /
prefix / suffix matching, fuzzy edit-distance search, semantic
patterns (-S: `_1 = _1` finds self-assignments, `TYP _ ( ... )`
finds definitions), test filtering (--no-tests / --tests-only
across 30 languages), and file type/kind filters. Hana's report
ranked the borrowable ideas: three quick wins (below), two medium
ones (comment search, a real --lang filter for directory searches),
one big direction question (semantic patterns — a token-pattern
engine, ~500+ lines, changes codecore's identity), and two not
recommended (fuzzy matching would add false positives; multi-
threading/mmap is unnecessary at ~20µs/file). Jo: "Let's do the
small fix first" — build the three quick wins. License note: cgrep
is GPL-2.0, but these are ideas implemented from scratch in Go
(cgrep is Haskell, so no code transfers).

- **--no-tests.** `isTestFile` in `main.go` (path heuristic only,
  it does not read the file): a file under a `test`, `tests`,
  `__tests__`, `spec`, or `specs` directory, or named `test_*`,
  `*_test`, `*_spec`, `*.spec.*`, `*.test.*`, or `conftest.py`.
  `collectFiles` gains a `noTests` parameter and skips test files —
  whether from a walk or a direct path — so a repo-wide --ask does
  not rank test code above the implementation it calls. Measured:
  `--ask "sort" --count` on the codecore directory lists
  `retrieve_test.go` with 31 matches; with `--no-tests` it is gone
  (the top counts are REVIEW.md 39, merge_sort.rs 38, main.m 36).
  One edge case noted: a documentation line that merely mentions a
  test file name (REVIEW.md L:167 mentions `retrieve_test.go`) is
  not a test file and is kept — correct, since the filter is on the
  path, not the content.
- **--word.** `prefixMatches` gains a `wordOnly` flag: no prefix
  expansion at all, so "read" does not match "reader" and "auth"
  does not match "authenticate". The substring bonus becomes
  whole-word only too (new `containsWord` helper), so a line
  containing only "reader" does not earn the bonus for "read" —
  without that, the bonus alone would make it a match and defeat
  the flag. Measured on pp.ps1: `--ask "read" --word` returns only
  lines with the word "read" (e.g. "Failed to read or parse CSV
  file"); without it, "reading" lines rank first.
- **--filename-only / --count.** Two new output modes for --ask
  (mutually exclusive, both need --ask). `--filename-only` prints
  each matching file once, in rank order (the file with the best
  match first); `--count` prints the number of matching outline
  lines per file, most first, counted before any --top cut (a file
  with many matches shows its full count, not just the ones that
  made the top N). Both support --format json (an array of file
  names, or an array of {file, count}). Implementation: `retrieve`
  is split into `allMatches` (walk + score + sort, no per-file /
  top / rank) plus `retrieve` (per-file dedup + top cut + ranks)
  and `countMatches` (aggregates allMatches by file). Measured:
  `--ask "heapsort" --filename-only` on testdb lists 7 files;
  `--ask "heapsort" --count` shows CHECKLIST.md 2, the
  implementation files 1 each.

Tests: `TestRetrieveWordOnly`, `TestCountMatches`,
`TestFormatFileNames`, `TestFormatCounts`, `TestIsTestFile`,
`TestCollectFilesNoTests`. All existing `retrieve(...)` call sites
gain the `wordOnly` argument (false) and all `collectFiles(...)`
call sites gain the `noTests` argument (false). gofmt/vet/test/
race all pass; fixtures and pp.ps1 unchanged (34/42/28/1494).
Tested locally by Hana only, not on the agent yet, per Jo's
instruction. The medium and big cgrep ideas (comment search,
--lang filter, semantic patterns) are NOT built — they are
separate decisions.

## 14. Semantic pattern search (--pattern), Phase 1 (2026-11-24)

`--pattern` scans source tokens at every start position and reports overlapping
matches in file and source order. Matches carry the source lines spanned by
their first and last tokens. The CLI supports text and JSON output and warns
when the 1000-result cap truncates the output.

The Go implementation follows cgrep's ideas of token-pattern matching,
numbered placeholder unification, and lazy, bracket-balanced ellipses. An
ellipsis can skip up to 64 tokens and stops at a depth-zero semicolon or
unmatched closer. Phase 1 leaves per-language keyword and native type tables,
`%` and docstring comment skipping, prefiltering, and matcher fast paths for
future work.

The demos produced one self-assignment, two function matches, and three
assignments whose right-hand token matched `NOT STR`.

Preprocessor fix (2026-11-24, same day): the first `--pattern` build
treated every `#` as a comment, so `#define FOO 1` was invisible to
patterns. The tokenizer now treats a `#` at the start of a line
followed by a preprocessor keyword (`include`, `define`, `ifdef`, ...)
as code — `#` becomes an operator token — while a `#` anywhere else
stays a comment. `--pattern 'FOO'` now finds both the `#define` and
its uses. Test `TestTokenizeCodePreprocessor` covers it.

```text
$ /tmp/codecore-fold --pattern '_1 = _1' /tmp/codecore-pattern-demo.txt
  /tmp/codecore-pattern-demo.txt [L:1] x = x
$ /tmp/codecore-fold --pattern 'func _ ( ... )' testdata/sample.go
  testdata/sample.go [L:58] func NewServer(cfg Config) *Server { return &Server{cfg: cfg} }
  testdata/sample.go [L:75] func main() {
$ /tmp/codecore-fold --pattern '_ = NOT STR' /tmp/codecore-pattern-demo.txt
  /tmp/codecore-pattern-demo.txt [L:1] x = x
  /tmp/codecore-pattern-demo.txt [L:2] x = y
  /tmp/codecore-pattern-demo.txt [L:3] n = 42
```

At `--depth 99`, the outline counts were 34 for `sample.py`, 42 for
`sample.go`, 28 for `sample.ps1`, and 1494 for `pp.ps1`.

## 15. Semantic pattern search, Phase 2 (2026-11-24)

The keyword table covers all 35 extractor languages, with empty sets for
HTML, XML, YAML, JSON, CSS, and plain text. Each file uses its extractor's
language to classify words for `KEY` and `TYP`. Table lookups match source
spelling case-sensitively, except SQL: its `foldCase` flag upper-cases the
word before lookup, so lowercase `select` classifies the same as `SELECT`
(SQL is case-insensitive; every other language is case-sensitive).

The CLI parses the pattern once without a language so it can reuse the
same atoms across files. Exact words compare by text across identifiers,
keywords, and native types. This lets a literal `if` in the pattern match
a Go keyword token. Placeholders continue to match any word token.

On `testdata/sample.go`, `KEY` returned 29 matches and `TYP` returned 11.

### Fixes after Phase 2 (2026-11-24)

Two gaps found during verification, both fixed and pushed with the docs:

1. **Duplicate lines for single-token patterns.** `patternMatches` yielded
   one result per matching token position, so a line with two keywords
   appeared twice for `--pattern 'KEY'`. It now dedups identical line
   spans, but only for single-token matches (a `seen` set keyed on
   `[line, end]`): multi-token overlapping matches such as `_1 = _1`
   on `x = x = x` are distinct results and are still reported twice.
   `KEY` on sample.go now returns 19 matches (was 29 with duplicates);
   `TYP` returns 9 (was 11). The first version deduped all matches and
   broke the overlapping-matches test; the single-token rule is the fix.
2. **SQL lowercase keywords.** The SQL table was uppercase-only and the
   lookup case-sensitive, so lowercase `select`/`from`/`where` did not
   classify as `KEY`. The `langWords.foldCase` flag (SQL only) upper-cases
   the word before lookup, so all casings classify.

### Pattern syntax documentation (2026-11-24)

The README's pattern section was rewritten as a full syntax reference:
a table of every atom (exact words/strings/numbers/operators, ANY, KEY,
TYP, OP, NUM, OCT, HEX, STR, LIT, `_`, `_1`/`_2`, NOT, OR, `...`),
the literal-escaping rule (`_ANY`, `_KEY`, `__1`), worked examples, a
"What the pattern sees" section (comments, preprocessor `#`, per-file
language, SQL case-insensitivity, 1000-match cap), and a "Pattern vs
ask" paragraph (ask = name/topic, ranked top-N; pattern = shape,
exhaustive). The `--help` text carries a condensed version of the
same reference and points to the README for the full table.
Each result corresponds to one token, so lines with multiple matching
tokens appear more than once. The exact `if` search returned zero
matches on that file because it contains no `if` token. The same
search returned one match on a Go snippet containing `if ok { return }`.

At `--depth 99`, the fixture outputs matched the baseline byte for byte,
with 34 nodes for `sample.py`, 42 for `sample.go`, 28 for `sample.ps1`,
and 1494 for `pp.ps1`.
