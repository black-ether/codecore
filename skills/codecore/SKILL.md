---
name: codecore
description: Fast structural outline extractor for code. Use codecore --ask FIRST when you need to find where something is defined or handled, especially across multiple files or a whole repository — run --ask before grepping file by file. Use the outline to understand a file's or repository's structure without reading whole files. Outlines code by its block structure (fold model), searches with BM25 (--ask), and walks directories recursively. Token-efficient navigation for coding agents.
metadata:
  short-description: Outline and search code structure without reading whole files
---

# codecore

codecore extracts a structural outline of code the way an editor's "fold all" does: it shows the bigger structure, one level at a time, with `[L:<line>]` anchors. It does not classify kinds (no function/variable labels) and it does not include comments or docstrings in the outline. Use it to get the big picture of a file or repo, or to find where something is defined, before reading the actual code.

Prefer `--ask` over grep when you need to find where something is defined or handled. Run `--ask` first; only grep if `--ask` returns nothing useful or you need an exhaustive list of every occurrence of a string.

Binary: `~/go/bin/codecore` (use this full path; it may not be on PATH in every shell). Zero dependencies, stdlib only, runs in ~20µs per file.

## When to use it

- You are new to a file or repo and need the big picture before reading code.
- You need to find where a function, class, or symbol is defined.
- You need to locate the code that handles a topic (auth, config, sorting) without reading every file.
- You want a token-cheap map of a codebase to decide what to read next.

Do not use it when you need the actual code — after the outline or `--ask` points at a location, read the source lines directly. Do not use it for call graphs or import relationships; it has no cross-file awareness.

## Commands

Flags must come BEFORE the paths. Go flag parsing stops at the first positional argument, so `codecore testdb --format json` fails — write `codecore --format json testdb`.

```sh
~/go/bin/codecore <file>                  # outline one file (per-language default depth)
~/go/bin/codecore --depth 0 <file>        # top level only
~/go/bin/codecore --depth 1 <file>        # top level + one level inside each block
~/go/bin/codecore --depth 99 <file>       # full tree (for cross-checks; no --full flag)
~/go/bin/codecore <dir>                   # outline all supported files in a directory, recursively
~/go/bin/codecore --format json <file>    # JSON output
~/go/bin/codecore --lang <name> <file>    # force a language parser (e.g. --lang python)
~/go/bin/codecore --ask "query" [paths]   # rank matching outline lines (BM25), top 5 by default
~/go/bin/codecore --ask "query" --top 10 <dir>   # top 10 results, repo-wide when given a directory
~/go/bin/codecore --ask "query" --no-tests <dir> # exclude test files from a repo-wide search
~/go/bin/codecore --ask "query" --word <dir>     # whole-word matching only (no prefix matching)
~/go/bin/codecore --ask "query" --filename-only <dir>  # print only the names of files with matches
~/go/bin/codecore --ask "query" --count <dir>     # print match counts per file
```

A directory argument is walked recursively (sorted by path, overlapping inputs deduplicated). `.git`, `node_modules`, `__pycache__`, `.venv`, `vendor`, `target`, `dist`, and `build` are never descended into; files without a supported extension are skipped silently. A file passed directly is always kept.

## --ask: how to ask

`--ask` ranks outline lines by BM25 over the outline tree (k1=1.2, b=0.75), with parent chains shown. Each result carries a range `[L:start-end]` (end = last content line of the block; for brace languages the closing `}` is not included) and, in JSON, a `name` field. Definitions rank above uses of the same name.

- Query with code terms first: `codecore --ask "authenticate_user" src/` beats `codecore --ask "where is auth handled"`.
- Question words (where, is, how, do, the, ...) are stripped automatically, but code terms rank better, so prefer them.
- Prefix matching: a query term of 3+ letters matches any longer term it prefixes, so `auth` finds `authenticate`. Use `--word` when you want whole-word matching only (`read` must not match `reader`).
- When you are looking for the implementation, not the tests, add `--no-tests`: test files call the implementation, so they rank high as uses and pollute repo-wide results. Leave it off when the question is about the tests themselves.
- If results are only uses, add the defining word (`def`, `func`, `class`) or the type it works on, or use `--def-only` to return definitions only.
- Use `--per-file` for a repo-wide "which files implement X" question, so one file cannot fill the whole top-N list.
- Use `--filename-only` when you only need the file list, and `--count` when you need to know which files mention a term most.
- If a term finds nothing, try a shorter prefix or a related word (`login`, `token`).
- Ask again with a narrower term rather than a longer sentence.
- A query that is only stopwords errors out (exit 2). `--top` below 1 errors out (exit 2). No matches prints `no matches for "<query>"` and exits 0.

## Output

Text output is one line per outline node: `[L:<line>] <source line>`, nested by indentation. A node with hidden descendants shows ` … (N more)`. In JSON, boundary nodes carry `"hasMore": true`.

## What it does not do

- No kind labels: it never tells you a line is a function, class, or variable.
- No comments or docstrings in the outline (by design). A file with no code structure and only comments yields almost nothing.
- No cross-file awareness: no import graph, no call graph, no "who calls this".
- Ranges end at the last content line; the closing brace/closer line is not included.
- Exact-word matching only (plus prefix matching). No synonyms, no semantic/embedding search. `auth` will not find `login` unless the word appears.
- Single-file and directory modes only; it does not fetch code for you. After it points at a location, read the source lines with your own tools.

## Languages

34 languages across three parsers: brace (Go, PowerShell, JavaScript, TypeScript, Rust, C, C++, C#, Java, Kotlin, Swift, Scala, Dart, Groovy, Zig, CSS, JSON, PHP), indent (Python, Ruby, Perl, Lua, R, Julia, Objective-C, Bash, SQL, MATLAB, LaTeX, Clojure, Haskell, YAML), tags (HTML, XML). HTML/XML default depth is 2 (their top level is a single wrapper tag); most languages default to depth 0. The full routing table and its reasons are in `~/workspace/codecore/ROUTING.md`.
