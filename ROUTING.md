# codecore — routing table and final analysis

Built 2026-11-23 from the 33-language test database
(`testdb/CHECKLIST.md`, one language at a time, 3 files each, real
code from GitHub). The test database is the evidence; this table is
the conclusion.

## The routing table

One parser per language. The choice is which parser handled the
language best in the tests — not which one is "correct" in theory.

### Brace parser (structure is in the brackets)

Go, PowerShell, JavaScript, TypeScript, Rust, C, C++, C#, Java,
Kotlin, Swift, Scala, Dart, Groovy, Zig, CSS, JSON, PHP.

Why: these languages delimit blocks with `{`/`}` (and `(`/`)`,
`[`/`]`). The brace parser folds by brackets and does not depend
on the code being well-indented, so it works on minified files too.
The C-style comment rules (`/* */`, `//`) and backslash escapes
now apply to every brace language except PowerShell (which uses
`<# #>`, `#`, and `` ` `` escapes) — that generalization was needed
because the tests ran with `--lang go`, which had those rules, and
the routing table registers each language under its own name.

### Indent parser (structure is in the indentation)

Python, Ruby, Perl, Lua, R, Julia, Objective-C, Bash, SQL, MATLAB,
HTML, XML, LaTeX, Clojure, Haskell, YAML.

Why: these languages either have no braces (Ruby, Lua, Julia,
Haskell, YAML, SQL, Bash, MATLAB, LaTeX, HTML, XML) or the indent
parser handles them better than the brace parser (Objective-C
merges multi-line signatures and nests `case` bodies; Clojure
merges multi-line forms; Perl/R hide `#` comments that the brace
parser shows). The indent parser carries the generic rules the
tests added: `#` and `//` and `--` and `%` comments, bare `end`,
`</...>` closers, `<!-- -->` comments, `<?php` tags,
`#include`/`#import` directives, nested `/* */` comments.

### The shared `.m` extension

Objective-C and MATLAB both use `.m`, and both route to the indent
parser, so the shared extension is not a conflict. The registry
holds both; the extension maps to the indent parser either way.

### Where the two parsers agree

For C#, Java, Kotlin, Swift, Scala, Dart, Groovy, Zig, JSON, and
PHP, both parsers give identical correct output on the test files.
The brace parser is the route for those because it does not depend
on indentation (it works on minified or badly indented files), and
because they are brace languages. For normalize.css (no `@media`)
and heap_sort.clj, both parsers also agree.

## Issues found

### Fixed during testing (8 fixes)

1. **Docstrings** (Python): standalone `"""`/`'''` lines showed as
   nodes. Fixed — skipped unless the previous line ends with `(`, `[`,
   `{`, or `,` (a string argument stays).
2. **C-style comments** (JS/TS): `/* */` and `//` lines leaked as
   nodes in the indent parser. Fixed — tracked like triple-quotes.
3. **Split header, brace parser** (TS): a multi-line arrow-function
   signature split into two nodes. Fixed — `isHeaderContinuation`
   merges closer-only tail lines into the header above.
4. **Rust attributes** (Rust): `#[...]` lines were skipped as `#`
   comments. Fixed — a `#` line is skipped only when it does not
   start with `#[`.
5. **PHP tags**: `<?php`/`?>` showed as nodes. Fixed — skipped in
   both parsers.
6. **Lua/Ruby `--` and `end`**: `--` comment lines and bare `end`
   lines showed as nodes. Fixed — `--` lines skipped (Lua, SQL),
   bare `end` skipped (Ruby, Lua). This also fixed the real-Lua
   noise (Neovim keymap.lua went from 155 nodes to 50).
7. **Preprocessor directives** (C/C++/ObjC): `#include`/`#import`
   lines were skipped as `#` comments. Fixed — `isPreprocessorLine`
   spares directive lines.
8. **Nested `/* */` comments** (SQL): SQL allows nested block
   comments; the first `*/` closed the comment early. Fixed — the
   indent parser counts block-comment depth instead of a flag.
9. **`%` comments** (MATLAB): `%` and `%%` lines showed as nodes.
   Fixed — skipped, except when the `%` starts a Ruby string
   literal (`%w[...]`, `%q{...}`).
10. **HTML/XML closing tags and comments**: `</...>` and `<!-- -->`
    showed as nodes. Fixed — `</` lines skipped, `<!-- -->`
    comments skipped (single-line, or multi-line tracked with a
    flag). HTML and XML are the only tag-based languages in the 33.

### Left as is (by Jo's call)

- **Split header, indent parser** (Python api.py, Swift MergeSort,
  Julia merge_sort): a multi-line signature splits into two nodes
  (e.g. `def request(` and `) -> Response:`). Jo: "it reads okay,
  leave it for now."
- **Ruby `end` nodes**: fixed anyway by the Lua patch (same rule).
- **C Allman `{` with a trailing `/* */` comment**: shows as a
  node (bubble_sort L:46/L:51/L:53). The C options widget was never
  answered; gap 1 (the `#include` lines) is fixed, gap 2 remains.
- **Bash heredocs and shell keywords**: heredoc content shows as
  nodes in both parsers; `fi`/`then`/`else`/`esac` show as nodes in
  the indent parser. Jo: "leave it for now."
- **LaTeX `\end{...}` lines**: `\end{itemize}`, `\end{document}`
  show as nodes. Jo: "leave it now." A fix would skip lines
  starting with `\end{` (same shape as the HTML `</` rule).
- **C++ flat namespaces in the indent parser**: C++ does not
  indent namespace contents, so nothing nests under `namespace`.
  A structural limit of indent folding for brace languages; the
  brace parser handles it.
- **Java annotations in the indent parser**: `@Override` merges
  into the following declaration (the decorator rule fires). Java
  routes to the brace parser, so this does not affect it.
- **CSS `@media` in the indent parser**: `@media` merges with the
  next line (the decorator rule fires on `@`). CSS routes to the
  brace parser, so this does not affect it. Same shape as Java.
- **Clojure tail lines in the brace parser**: `int-seq)))` and
  `done))))` show as stray nodes (real content before the closers).
  Clojure routes to the indent parser, so this does not affect it.
- **Julia merge_sort split header**: same shape as the Python case.
  Jo: "leave it for now."

### Known limits (accepted, not bugs)

- **Preprocessor and SQL `BEGIN`/`END` blocks** are invisible to
  both parsers (no generic fix; accepted as a limit).
- **Dict `:` false positive** (Python): a dict value nests under
  its key under pure-indent. Arguably fine; left as a known wart.
- **The brace parser's `/* */` handling stays non-nested** (correct
  for its languages — C/Go/JS do not allow nested block comments).
- **`--[[ ]]` long comments** (Lua) and **`end` with a trailing
  comment** are not skipped (neither appears in the test files).
- **Minified HTML/XML** (all on one line) comes out flat in the
  indent parser — it trusts indentation, not the tags. A separate
  tag parser would fix this; a prompt for Gemini to build one is
  saved at `~/workspace/gemini-test/tag_parser_prompt.txt` (the
  Gemini API was overloaded when it was written; the prompt was
  emailed to Jo 2026-11-23).

## What the tests did not cover

- **The `.m` clash** is handled (both languages use the indent
  parser), but a file's language cannot be told from `.m` alone.
- **Languages not in the 33**: F#, Elixir, Erlang, VHDL/Verilog,
  Prolog, Assembly, Fortran, COBOL, VBA, Delphi, D, Nim, Crystal,
  Terraform (HCL). Their structures mostly map onto something
  tested (brace, indent, or `end`-keyword), except VHDL/Verilog
  (hardware description) and Prolog (logic). Elixir/Erlang are
  `end`-keyword languages like Ruby/Lua.

## Verification

After the routing table was registered, the default run (no
`--lang`) was compared against the forced run of the routed parser
for every test file: **93 files pass** (all files with a registered
extension). The 3 Bash test files (git-jump, pre-commit.sample,
pre-rebase.sample) have no `.sh` extension, so they cannot route
by extension — they still work with `--lang python`. Fixtures and
pp.ps1 are unchanged (34/42/28/1494); real Lua keymap.lua is still
50 nodes. `gofmt`, `go vet`, `go test`, `go test -race` all pass.
