package main

import (
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
  --top N              maximum results for --ask (default 5)
  --def-only           with --ask, return definitions only
  --per-file           with --ask, return the best match per file only
  -h, --help           show this help

With no arguments, this help is shown. Flags come before paths.

How it works: codecore folds code blocks the way an editor's "fold
all" does — indentation for Python, brackets for Go and PowerShell.
Every code line is shown; a line that opens a multi-line block shows
its contents nested underneath it. There are no kind labels and no
parsing: the outline is the file's lines, nested by their blocks.

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
	topFlag := fs.Int("top", 5, "maximum results for --ask")
	defOnlyFlag := fs.Bool("def-only", false, "only definitions for --ask")
	perFileFlag := fs.Bool("per-file", false, "best match per file for --ask")
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
	var terms []string
	if *askFlag != "" {
		if *topFlag < 1 {
			fmt.Fprintln(os.Stderr, "codecore: --top must be 1 or more")
			return 2
		}
		terms = filterQueryTerms(tokenize(*askFlag))
		if len(terms) == 0 {
			fmt.Fprintln(os.Stderr, "codecore: --ask query must contain a searchable term")
			return 2
		}
	}
	if len(paths) == 0 {
		fmt.Print(helpText())
		return 0
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

	files, err := collectFiles(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, "codecore: "+err.Error())
		return 1
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
		o, err := ext.Extract(f, src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "codecore: %v\n", err)
			return 1
		}
		if o.Unbalanced {
			fmt.Fprintf(os.Stderr, "codecore: %s: unbalanced brackets; outline may be incomplete\n", f)
		}
		outlines = append(outlines, o)
	}

	if *askFlag != "" {
		results := retrieve(outlines, terms, *topFlag, *defOnlyFlag, *perFileFlag)
		err = formatResults(os.Stdout, *formatFlag, *askFlag, results)
	} else {
		err = fmtr.Format(os.Stdout, outlines)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "codecore: %v\n", err)
		return 1
	}
	return 0
}

// skipDirs are the directories a recursive walk never descends into:
// version control, dependency trees, build output, and caches. Walking
// into them would flood the output with files the user did not ask for.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "__pycache__": true,
	".venv": true, "venv": true, "vendor": true,
	"target": true, "dist": true, "build": true,
}

// collectFiles expands paths: a file is kept as-is, a directory expands
// recursively to its files with a registered extractor extension
// (sorted by path). Directories in skipDirs are never descended into,
// and files without a registered extension are skipped silently — a
// repo holds many non-code files, and erroring on each would be noise.
// A file passed directly is kept even without a registered extension;
// the extractor reports it when the file is read. The root directory
// itself is never skipped, even when its name is in skipDirs.
func collectFiles(paths []string) ([]string, error) {
	var files []string
	seen := make(map[string]bool)
	add := func(p string) {
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
