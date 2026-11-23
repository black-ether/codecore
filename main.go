package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
  --depth N            show the first N levels (default 0: top level only)
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
  indented two spaces per level. By default only the top level is
  shown — the big picture. A line marked " … (N more)" opens further:
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
	depthFlag := fs.Int("depth", 0, "levels to show (0: top level only)")
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
	if len(paths) == 0 {
		fmt.Print(helpText())
		return 0
	}
	if *depthFlag < 0 {
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

	if err := fmtr.Format(os.Stdout, outlines); err != nil {
		fmt.Fprintf(os.Stderr, "codecore: %v\n", err)
		return 1
	}
	return 0
}

// collectFiles expands paths: a file is kept as-is, a directory expands to
// its files with a registered extractor extension (non-recursive, sorted
// — os.ReadDir already sorts).
func collectFiles(paths []string) ([]string, error) {
	var files []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			files = append(files, p)
			continue
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				if _, ok := ExtractorFor(e.Name()); ok {
					files = append(files, filepath.Join(p, e.Name()))
				}
			}
		}
	}
	return files, nil
}
