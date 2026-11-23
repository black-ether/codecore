# codecore

Fast structural outline extractor. Dev phase: run from source, no binary.

```sh
go run .                  # help page
go run . <file>           # outline one file (top level only)
go run . --depth 1 <file> # top level + one level inside each block
go run . <file> ...       # outline several files
go run . <dir>            # outline all supported files in a directory
go run . --format json <file>
```

Languages: Go, PowerShell, JavaScript, TypeScript, Rust, C, C++,
C#, Java, Kotlin, Swift, Scala, Dart, Groovy, Zig, CSS, JSON, PHP
(brace parser); Python, Ruby, Perl, Lua, R, Julia, Objective-C,
Bash, SQL, MATLAB, LaTeX, Clojure, Haskell, YAML (indent parser);
HTML, XML (tag parser). One parser per language, chosen by the
33-language test database — see `ROUTING.md` for the table and the
reasons.

## How it works

codecore folds code blocks the way an editor's "fold all" does:
indentation for Python, brackets for Go and PowerShell, tags for
HTML and XML. Every code line is shown; a line that opens a
multi-line block shows its contents nested underneath it. There are
no kind labels and no parsing — the outline is the file's lines,
nested by their blocks. That is the whole model, and it is the same
for every language.

## Output

Text format, one line per code line. The `[L:n]` tag is the 1-based
line number, always at the far left and padded to the widest tag in
the output so the lines line up; children are indented two spaces per
level.

By default the top level is shown — the big picture (HTML/XML show
two levels, since their top level is a single wrapper tag). A line
marked ` … (N more)` opens further, and N says how many outline lines
hide inside it, so you can tell at a glance which blocks are worth
opening. `--depth 1` shows one level inside each block, `--depth 2`
two levels, and so on, one depth at a time. There is no way to dump
the whole tree: that is the point of the tool — the big picture first,
drill in only where it matters. JSON (`--format json`) is cut to the
same depth, with `"hasMore": true` on the lines that hide more.

```
[L:4]  package sample
[L:6]  import ( … (4 more)
[L:13] import "strings"
[L:15] const ( … (2 more)
[L:21] var ( … (2 more)
[L:34] type Config struct { … (4 more)
[L:61] func Map[T any, U any](in []T, f func(T) U) []U { … (4 more)
[L:75] func main() { … (6 more)

--depth 1 opens one level inside each block:

[L:6]  import (
[L:7]    "fmt"
[L:8]    "io"
[L:34] type Config struct {
[L:35]   Port        int    `json:"port"`
[L:36]   Host        string `json:"host"`
[L:61] func Map[T any, U any](in []T, f func(T) U) []U {
[L:62]   out := make([]U, len(in))
[L:63]   for i, v := range in { …
```

A line longer than 120 characters is capped with `…`; the full line
stays in the source file. Blank lines and comment-only lines are not
shown. A block that opens and closes on one line (`if ($x) { $y = 1 }`,
`$sb = { param($x) $x * 2 }`) is a leaf — there is nothing inside it
to open.

## Design

Three layers, kept separate:

- **Extraction** — one fold parser per language. Python folds by
  indentation: a line opens a block when the next code line is more
  indented — no `:` is required. That is the whole rule, and it is why
  the parser is not Python-specific: a multi-line call, list or dict
  nests under its first line (its contents are inside it, not
  top-level statements), and the same rule folds Ruby, Lua and
  anything else that is consistently indented. A line that is only
  closers is not shown, and neither is a line that is only a
  triple-quoted string (a docstring) — it is pure symbols, or text
  with no structure. C-style comments are skipped too: a `//` line,
  and a `/* */` block (only a comment at the start of a line is
  treated as one, so a `/*` inside a string or after code is left
  alone). Block comments nest (SQL), so the depth is counted, not
  just a flag. A `#` line is a comment — except `#[...]`, which is an
  attribute (Rust), not a comment, and a preprocessor directive
  like `#include` / `#import` (C, Objective-C, C++), which is a
  statement, not a comment. A `--` line is a comment too
  (Lua, SQL). A `%` line is a comment too (MATLAB), and `%%` is a
  section header — except when the `%` starts a Ruby string
  literal (`%w[...]`, `%q{...}`), which is code. A bare `end` line
  closes a block (Ruby, Lua), like
  `}` — it is not a statement, so it is not shown. A closing tag
  (`</...>`, HTML/XML) closes a block the same way, so it is not
  shown either, and an HTML/XML `<!-- -->` comment is not a
  statement (a multi-line one is tracked like a `/* */` block).
  A `<?php` / `?>` tag line is boilerplate, not a statement, so it
  is not shown either. A string on a line of its own
  inside a multi-line call is an argument, not a docstring, so it
  stays.
  Go and PowerShell fold by brackets: `{`, `(` and `[` open, their
  closers close (`fold_braces.go`). A small scanner skips strings
  and comments so braces inside them never count, and a closer pops
  only a frame whose opener matches, so a stray bracket in broken
  code is ignored instead of un-nesting everything after it. A `{`
  on a line of its own (Allman style) attaches its block to the
  previous code line, so the nesting stays one level at a time.
  Decorator lines merge into the following definition's header.
- **Model** — `Node` (`Line`, `Text`, `Children`) and `Outline`
  (with `Unbalanced`) in `outline.go`.
- **Formatting** — a `Formatter` renders outlines. Text and JSON ship
  now; a new format is one method, never an extractor change.

Dependencies: none. The standard library only — no tree-sitter, no
CGo, no C compiler, no network on first run. `go mod tidy` works; see
`go.mod`.

## Adding a language

There are three parsers, not one per language: the indent parser
(`parsePython` in `fold_python.go`), the brace parser
(`parseBraces` in `fold_braces.go`), and the tag parser
(`parseTags` in `fold_tags.go`, for HTML/XML). Adding a language
means choosing which one handles it best — test it against both on
real code first (see `testdb/CHECKLIST.md` for the method), then
register it in `init()` in `fold.go` with its language name,
extensions, and `parser: "indent"`, `parser: "braces"`, or
`parser: "tags"`. The routing table in `ROUTING.md` is the record
of those choices.
If neither parser handles the language, write a new fold parser
in a `fold_<name>.go` (a function that turns lines into
`[]*Node`), register it the same way, and add a
`testdata/sample.<ext>` fixture and a `TestFold<Lang>` test before
trusting it. Nothing else changes: the CLI picks it up by
extension, or by name with `--lang <name>`.

## Speed

Measured on this machine (shared 1.5-core VM, 2026-11-23) on the
`testdata` fixtures (~90 lines each):

| language | per file |
| --- | --- |
| Go | ~20 µs |
| Python | ~24 µs |
| PowerShell | ~14 µs |

The previous tree-sitter build did the same fixtures in ~1.8 ms (Go),
~1.0 ms (Python) and ~1.1 ms (PowerShell) — the fold is roughly
50–90x faster, and faster than the original Go-only `go/ast` walker
(~79 µs) too. At ~50,000 files/second it is ample for outlining.

## Error tolerance

A file whose brackets do not balance still produces an outline — the
CLI prints a warning on stderr, the outline is partial, and JSON
marks it with `"unbalanced": true`. The exit code stays 0. An empty
file yields an empty outline.

## Known limits

- The outline shows lines, not kinds: there is no func/var/type
  distinction, no signatures beyond the line text, and no detail
  field. A method's receiver and a struct field's type are visible
  only because they are written on the line itself.
- Python: a dict value split across lines (`"key":` / value) nests
  under its key line; a multi-line def signature's header is the
  line ending in `:`.
- Braces: braces inside `$(...)` in a PowerShell double-quoted string
  are not counted. C-style `/* */` comments in PowerShell are not
  treated as comments (they are not valid PowerShell — use `<# #>`);
  harmless unless they contain braces. A lone `{` after a closed
  block attaches to the previous code line — right for Allman style,
  wrong for a bare block standing on its own.
- No `go/types` checking by design — this is a structural outline, not a
  semantic index.
