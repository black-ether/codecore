package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// helpText is the default page: shown with no arguments and with -h/--help.
func helpText() string {
	return `codecore — fast structural outline extractor

Usage:
  go run . [flags] <file|dir> ...    outline files (dev: run from source)

Flags:
  --format text|json   output format (default text)
  --lang <name>        language: auto (by extension) or a name (default auto)
  --depth N            show the first N levels (default: per language —
                       0 for most, 2 for HTML/XML, whose top level is a
                       single wrapper tag)
  --ask "query"        rank matching outline lines at any depth
  --pattern "pat"      find all structural token-pattern matches
  --top N              maximum results for --ask (default 5)
  --def-only           with --ask, return definitions only
  --split-identifiers  with --ask, index compound identifier parts
  --per-file           with --ask, return the best match per file only
  --word               with --ask, match whole words only (no prefix
                       matching, so "read" does not match "reader")
  --group-structure   with --ask, return the best match per structural unit
  --categories LIST   with --ask, include production,docs,tests,fixtures,generated
  --no-tests           exclude test files (test_*.py, *_test.go,
                       *.spec.js, tests/ directories, and so on)
  --filename-only      with --ask, print only the names of files
                       with matches
  --count              with --ask, print the number of matching
                       outline lines per file
  -h, --help           show this help

With no arguments, this help is shown. Flags come before paths.

How it works: codecore folds code blocks the way an editor's "fold
all" does — indentation for Python, brackets for Go and PowerShell.
Every code line is shown; a line that opens a multi-line block shows
its contents nested underneath it. There are no kind labels and no
parsing: the outline is the file's lines, nested by their blocks.

Pattern search (--pattern) matches a sequence of atoms, one token
each, separated by spaces. Exact words match by text across
identifiers, keywords, and native types; quoted strings compare
their contents across quote styles. Wildcards: ANY (any token),
KEY (any keyword), TYP (any native type), OP (operator), NUM
(number), STR (string), LIT (string or number). _ matches any word;
_1, _2, ... bind a word and must repeat it (_1 = _1 finds
self-assignments). NOT negates the next atom; OR joins
single-token alternatives. ... skips tokens (shortest first, at
most 64, stops at unmatched closers and top-level ;). Prefix a
special word with _ to match it literally (_ANY, _KEY). Examples:
"func _ ( ... )" finds function headers; "_ = NOT STR" finds
assignments to anything but a string. See README.md for the full
syntax table. Use --ask for a name or topic, --pattern for a shape.

Output (text):
  [L:4]  package sample
  [L:6]  import ( …
  [L:15] const ( …

  One line per code line. The [L:n] tag is the 1-based line number,
  always at the far left and padded so the lines line up; children are
  indented two spaces per level. By default the top level is shown —
  the big picture (HTML/XML show two levels, since their top level is
  a single wrapper tag). A line marked " … (N more)" opens further:
  it hides N outline lines inside it. Use --depth 1 to see one level
  inside each block, --depth 2 for two, and so on, one depth at a time.
  --format json emits the same tree, cut to the same depth, with
  "hasMore" on the lines that hide more.

  A file whose brackets do not balance still produces an outline, with
  a warning on stderr; the outline is partial and the JSON marks it
  with "unbalanced".

Languages: ` + strings.Join(Languages(), ", ") + `

Adding a language: add a fold parser in a new fold_<name>.go and call
RegisterExtractor in init. See README.md.
`
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	fs := flag.NewFlagSet("codecore", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	formatFlag := fs.String("format", "text", "output format: text|json")
	langFlag := fs.String("lang", "auto", "language name or auto")
	askFlag := fs.String("ask", "", "rank matching outline lines")
	patternFlag := fs.String("pattern", "", "find structural token-pattern matches")
	topFlag := fs.Int("top", 5, "maximum results for --ask")
	defOnlyFlag := fs.Bool("def-only", false, "only definitions for --ask")
	perFileFlag := fs.Bool("per-file", false, "best match per file for --ask")
	splitIdentifiersFlag := fs.Bool("split-identifiers", false, "index compound identifier parts for --ask")
	wordFlag := fs.Bool("word", false, "whole-word only for --ask (no prefix matching)")
	groupStructureFlag := fs.Bool("group-structure", false, "group matches by enclosing structural unit for --ask")
	categoriesFlag := fs.String("categories", "", "comma-separated source categories for --ask")
	noTestsFlag := fs.Bool("no-tests", false, "exclude test files")
	filenameOnlyFlag := fs.Bool("filename-only", false, "with --ask, print only matching file names")
	countFlag := fs.Bool("count", false, "with --ask, print match counts per file")
	// -1 means "not specified": the language's default depth is used
	// (0 for most languages, 2 for HTML/XML). An explicit --depth 0
	// still shows only the top level.
	depthFlag := fs.Int("depth", -1, "levels to show (-1: language default)")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			fmt.Print(helpText())
			return 0
		}
		fmt.Fprintln(os.Stderr, "codecore: "+err.Error())
		fmt.Print(helpText())
		return 2
	}
	paths := fs.Args()
	var patternSet, askSet, categoriesSet bool
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "pattern":
			patternSet = true
		case "ask":
			askSet = true
		case "categories":
			categoriesSet = true
		}
	})
	var atoms []atom
	if patternSet {
		if askSet {
			fmt.Fprintln(os.Stderr, "codecore: --pattern and --ask are mutually exclusive")
			return 2
		}
		var err error
		atoms, err = parsePattern(strings.TrimSpace(*patternFlag))
		if err != nil {
			fmt.Fprintf(os.Stderr, "codecore: --pattern: %v\n", err)
			return 2
		}
	}
	var categories map[string]bool
	if categoriesSet {
		if *askFlag == "" {
			fmt.Fprintln(os.Stderr, "codecore: --categories needs --ask")
			return 2
		}
		var err error
		categories, err = parseCategories(*categoriesFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "codecore: --categories: %v\n", err)
			return 2
		}
	}
	opts := retrievalOptions{splitIdentifiers: *splitIdentifiersFlag, groupStructure: *groupStructureFlag}
	if opts.groupStructure && (*askFlag == "" || *countFlag) {
		fmt.Fprintln(os.Stderr, "codecore: --group-structure needs --ask and cannot be used with --count")
		return 2
	}
	var terms []string
	if *askFlag != "" {
		if *topFlag < 1 {
			fmt.Fprintln(os.Stderr, "codecore: --top must be 1 or more")
			return 2
		}
		terms = tokenize(*askFlag)
		if opts.splitIdentifiers {
			terms = wholeIdentifierTerms(*askFlag)
		}
		terms = filterQueryTerms(terms)
		if len(terms) == 0 {
			fmt.Fprintln(os.Stderr, "codecore: --ask query must contain a searchable term")
			return 2
		}
	}
	if len(paths) == 0 {
		fmt.Print(helpText())
		return 0
	}
	if opts.splitIdentifiers && *askFlag == "" {
		fmt.Fprintln(os.Stderr, "codecore: --split-identifiers needs --ask")
		return 2
	}
	if *wordFlag && *askFlag == "" {
		fmt.Fprintln(os.Stderr, "codecore: --word needs --ask")
		return 2
	}
	if *filenameOnlyFlag && *countFlag {
		fmt.Fprintln(os.Stderr, "codecore: --filename-only and --count are mutually exclusive")
		return 2
	}
	if (*filenameOnlyFlag || *countFlag) && *askFlag == "" {
		fmt.Fprintln(os.Stderr, "codecore: --filename-only and --count need --ask")
		return 2
	}
	if *askFlag == "" && *depthFlag < -1 {
		fmt.Fprintln(os.Stderr, "codecore: --depth must be 0 or more")
		return 2
	}
	depth := *depthFlag

	var fmtr Formatter
	switch *formatFlag {
	case "text":
		fmtr = TextFormatter{Depth: depth}
	case "json":
		fmtr = JSONFormatter{Depth: depth}
	default:
		fmt.Fprintf(os.Stderr, "codecore: unknown format %q (want text|json)\n", *formatFlag)
		return 2
	}

	files, err := collectFiles(paths, *noTestsFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "codecore: "+err.Error())
		return 1
	}

	if patternSet {
		if err := runPattern(files, *formatFlag, *patternFlag, atoms); err != nil {
			fmt.Fprintf(os.Stderr, "codecore: %v\n", err)
			return 1
		}
		return 0
	}

	outlines := make([]*Outline, 0, len(files))
	for _, f := range files {
		var ext Extractor
		var ok bool
		if *langFlag == "auto" {
			ext, ok = ExtractorFor(f)
			if !ok {
				// No registered language for this extension: fall
				// back to the plain-text parser, so --ask (and
				// outline) work on any text file instead of
				// erroring. The text parser rejects binary files
				// itself (NUL bytes). Directory walks still only
				// include registered extensions, so a walk does
				// not try to outline every unknown file it finds.
				ext, ok = ExtractorByLanguage("text")
			}
		} else {
			ext, ok = ExtractorByLanguage(*langFlag)
		}
		if !ok {
			fmt.Fprintf(os.Stderr, "codecore: no extractor for %s (languages: %s)\n",
				f, strings.Join(Languages(), ", "))
			return 1
		}
		src, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "codecore: %v\n", err)
			return 1
		}
		if categories != nil && !categories[sourceCategory(f, src)] {
			continue
		}
		o, err := ext.Extract(f, src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "codecore: %v\n", err)
			return 1
		}
		// Attach the standalone comments above each node, so --ask
		// finds code from the comment's words. The formatters
		// ignore Comments, so the fold output is unchanged.
		attachComments(o, src)
		if o.Unbalanced {
			fmt.Fprintf(os.Stderr, "codecore: %s: unbalanced brackets; outline may be incomplete\n", f)
		}
		outlines = append(outlines, o)
	}

	if *askFlag != "" {
		if *countFlag {
			err = formatCounts(os.Stdout, *formatFlag, *askFlag, opts.countMatches(outlines, terms, *defOnlyFlag, *wordFlag))
		} else if *filenameOnlyFlag {
			results := opts.retrieve(outlines, terms, *topFlag, *defOnlyFlag, *perFileFlag, *wordFlag)
			err = formatFileNames(os.Stdout, *formatFlag, *askFlag, results)
		} else {
			results := opts.retrieve(outlines, terms, *topFlag, *defOnlyFlag, *perFileFlag, *wordFlag)
			err = formatResults(os.Stdout, *formatFlag, *askFlag, results)
		}
	} else {
		err = fmtr.Format(os.Stdout, outlines)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "codecore: %v\n", err)
		return 1
	}
	return 0
}

func runPattern(files []string, format, pat string, atoms []atom) error {
	const maxMatches = 1000
	results := make([]patternResult, 0)
search:
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		lang := ""
		if ext, ok := ExtractorFor(file); ok {
			lang = ext.Language()
		}
		for result := range patternMatches(file, src, atoms, tokenizeCode(src, lang)) {
			if len(results) == maxMatches {
				fmt.Fprintln(os.Stderr, "codecore: --pattern found more than 1000 matches; showing the first 1000")
				break search
			}
			results = append(results, result)
		}
	}

	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}
	if len(results) == 0 {
		_, err := fmt.Fprintf(os.Stdout, "no matches for pattern %q\n", pat)
		return err
	}
	for _, result := range results {
		lineRange := fmt.Sprint(result.Line)
		if result.End != result.Line {
			lineRange = fmt.Sprintf("%d-%d", result.Line, result.End)
		}
		lineText := strings.ReplaceAll(result.Text, "\n", `\n`)
		if _, err := fmt.Fprintf(os.Stdout, "  %s [L:%s] %s\n", result.File, lineRange, lineText); err != nil {
			return err
		}
	}
	return nil
}

// skipDirs are the directories a recursive walk never descends into:
// version control, dependency trees, build output, and caches. Walking
// into them would flood the output with files the user did not ask for.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "__pycache__": true,
	".venv": true, "venv": true, "vendor": true,
	"target": true, "dist": true, "build": true,
}

// testDirs are the directory names that mark a whole subtree as test
// code: tests/, test/, __tests__/, spec/, specs/.
var testDirs = map[string]bool{
	"test": true, "tests": true, "__tests__": true,
	"spec": true, "specs": true,
}

// isTestFile reports whether a path is a test file, by the file-layout
// conventions cgrep's --no-tests uses (test_*.py, *_test.go,
// *.spec.js, tests/ directories, conftest.py, and so on). This is a
// path heuristic only — it does not read the file — so a helper named
// test_utils.go is treated as a test file too. --no-tests is opt-in,
// so the user chooses to apply it.
func isTestFile(path string) bool {
	// A file under a test directory is a test file.
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if testDirs[part] {
			return true
		}
	}
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	if strings.HasPrefix(name, "test_") || strings.HasPrefix(name, "test-") {
		return true
	}
	if strings.HasSuffix(name, "_test") || strings.HasSuffix(name, "-test") {
		return true
	}
	if strings.HasSuffix(name, "_spec") || strings.HasSuffix(name, ".spec") || strings.HasSuffix(name, ".test") {
		return true
	}
	return name == "conftest"
}

// collectFiles expands paths: a file is kept as-is, a directory expands
// recursively to its files with a registered extractor extension
// (sorted by path). Directories in skipDirs are never descended into,
// and files without a registered extension are skipped silently — a
// repo holds many non-code files, and erroring on each would be noise.
// A file passed directly is kept even without a registered extension;
// the extractor reports it when the file is read. The root directory
// itself is never skipped, even when its name is in skipDirs. With
// noTests, test files (isTestFile) are skipped — whether they come
// from a walk or a direct path — so a repo-wide --ask does not rank
// test code above the implementation it calls.
func collectFiles(paths []string, noTests bool) ([]string, error) {
	var files []string
	seen := make(map[string]bool)
	add := func(p string) {
		if noTests && isTestFile(p) {
			return
		}
		if !seen[p] {
			seen[p] = true
			files = append(files, p)
		}
	}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			add(p)
			continue
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != p && skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if _, ok := ExtractorFor(d.Name()); ok {
				add(path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}
