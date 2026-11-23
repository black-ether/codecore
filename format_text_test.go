package main

import (
	"bytes"
	"strings"
	"testing"
)

// formatOutline renders one outline at the given depth (-1 for the
// language's default depth) and returns its lines. Every line must
// start with [L: at column 0.
func formatOutline(t *testing.T, depth int, o *Outline) []string {
	t.Helper()
	var buf bytes.Buffer
	if err := (TextFormatter{Depth: depth}).Format(&buf, []*Outline{o}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	for _, l := range lines {
		if !strings.HasPrefix(l, "[L:") {
			t.Errorf("line does not start with [L: at column 0: %q", l)
		}
	}
	return lines
}

func TestTextFormatterDepth0(t *testing.T) {
	o := mustExtract(t, "testdata/sample.go")
	lines := formatOutline(t, 0, o)
	// package + import group + single import + const/var groups +
	// 2 single types + 4 block types + 5 funcs = 16 top-level lines.
	if len(lines) != 16 {
		t.Fatalf("depth-0 lines = %d, want 16:\n%v", len(lines), lines)
	}
	joined := strings.Join(lines, "\n")
	// Block headers are marked " … (N more)" so you can tell they open
	// further, and how much hides inside them.
	for _, want := range []string{
		"import ( … (4 more)", "const ( … (2 more)", "var ( … (2 more)",
		"type Config struct { … (4 more)", "func Map[T any, U any](in []T, f func(T) U) []U { … (4 more)",
		"func main() { … (6 more)",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("depth-0 output missing marked block header %q:\n%v", want, lines)
		}
	}
	// Nothing inside a block leaks into the top level.
	for _, leaked := range []string{`"fmt"`, "DefaultPort", "DefaultTimeout", "Port        int", "out :="} {
		if strings.Contains(joined, leaked) {
			t.Errorf("depth-0 output leaked block contents %q:\n%v", leaked, lines)
		}
	}
}

func TestTextFormatterDepth1(t *testing.T) {
	o := mustExtract(t, "testdata/sample.go")
	lines := formatOutline(t, 1, o)
	if len(lines) != 41 {
		t.Fatalf("depth-1 lines = %d, want 41 (16 top-level + 25 children):\n%v", len(lines), lines)
	}
	// The for loop's body is one level deeper, so it stays hidden.
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "out[i] = f(v)") {
		t.Errorf("depth-1 output leaked the depth-2 for-loop body:\n%v", lines)
	}
	// The for loop itself is a depth-1 block header, marked with how
	// much hides inside it (its one body line).
	if !strings.Contains(joined, "for i, v := range in { … (1 more)") {
		t.Errorf("depth-1 output missing the marked for-loop header:\n%v", lines)
	}
}

// Deep enough to show the whole tree, nothing is hidden and nothing
// is marked.
func TestTextFormatterWholeTree(t *testing.T) {
	o := mustExtract(t, "testdata/sample.go")
	lines := formatOutline(t, 2, o)
	if len(lines) != 42 {
		t.Fatalf("full lines = %d, want 42", len(lines))
	}
	for _, l := range lines {
		if strings.Contains(l, " …") {
			t.Errorf("full output marked a hidden-children node: %q", l)
		}
	}
}

func TestTextFormatterPythonDepth0(t *testing.T) {
	o := mustExtract(t, "testdata/sample.py")
	lines := formatOutline(t, 0, o)
	if len(lines) != 16 {
		t.Fatalf("depth-0 lines = %d, want 16:\n%v", len(lines), lines)
	}
	joined := strings.Join(lines, "\n")
	// The module docstring is not shown.
	if strings.Contains(joined, "Demo module") {
		t.Errorf("depth-0 output should not show the docstring:\n%v", lines)
	}
	if !strings.Contains(joined, "@decorator def decorated():") {
		t.Errorf("depth-0 output missing the merged decorator header:\n%v", lines)
	}
	if !strings.Contains(joined, "class Animal: … (10 more)") {
		t.Errorf("depth-0 output missing the marked class header:\n%v", lines)
	}
}

func TestTextFormatterTagAlignment(t *testing.T) {
	o := &Outline{File: "x.go", Language: "go", Nodes: []*Node{
		{Line: 1, Text: "package x"},
		{Line: 4, Text: "func a() {"},
		{Line: 120, Text: "type S struct {", Children: []*Node{
			{Line: 121, Text: "F int"},
		}},
		{Line: 123, Text: "func b() {"},
	}}
	lines := formatOutline(t, 1, o)
	if len(lines) != 5 {
		t.Fatalf("lines = %d, want 5", len(lines))
	}
	// Widest tag is "[L:123]" (7 chars); every depth-0 line starts at
	// column 8, the depth-1 line exactly 2 columns later.
	col := func(l, word string) int { return strings.Index(l, word) }
	if got := col(lines[1], "func"); got != 8 {
		t.Errorf("depth-0 text column = %d, want 8: %q", got, lines[1])
	}
	if got := col(lines[2], "type"); got != 8 {
		t.Errorf("depth-0 text column = %d, want 8: %q", got, lines[2])
	}
	if got := col(lines[3], "F int"); got != 10 {
		t.Errorf("depth-1 text column = %d, want 10: %q", got, lines[3])
	}
	if got := col(lines[4], "func"); got != 8 {
		t.Errorf("depth-0 text column = %d, want 8: %q", got, lines[4])
	}
}

// Depth 2 shows two levels of children and marks the depth-2 blocks
// that still hide children.
func TestTextFormatterDepth2(t *testing.T) {
	o := &Outline{File: "x.go", Language: "go", Nodes: []*Node{
		{Line: 1, Text: "func a() {", Children: []*Node{
			{Line: 2, Text: "if x {", Children: []*Node{
				{Line: 3, Text: "for y {", Children: []*Node{
					{Line: 4, Text: "do()"},
				}},
			}},
		}},
	}}
	lines := formatOutline(t, 2, o)
	if len(lines) != 3 {
		t.Fatalf("depth-2 lines = %d, want 3", len(lines))
	}
	if strings.Contains(lines[1], "…") {
		t.Errorf("depth-2 line 1 = %q, want no marker (its child is shown)", lines[1])
	}
	if !strings.Contains(lines[2], "for y { … (1 more)") {
		t.Errorf("depth-2 line 2 = %q, want the marked for header", lines[2])
	}
}

// Depth -1 means "use the language's default depth": HTML/XML
// default to 2 (their top level is a single wrapper tag, so 0
// shows almost nothing), everything else defaults to 0.
func TestDefaultDepth(t *testing.T) {
	html := mustExtract(t, "testdb/html/boilerplate-index.html")
	if html.DefaultDepth != 2 {
		t.Errorf("html DefaultDepth = %d, want 2", html.DefaultDepth)
	}
	py := mustExtract(t, "testdata/sample.py")
	if py.DefaultDepth != 0 {
		t.Errorf("python DefaultDepth = %d, want 0", py.DefaultDepth)
	}
	// An HTML outline at Depth -1 renders the same as at depth 2;
	// a Python outline at Depth -1 renders the same as at depth 0.
	if a, b := formatOutline(t, -1, html), formatOutline(t, 2, html); len(a) != len(b) {
		t.Errorf("html Depth -1 = %d lines, want depth-2 count %d", len(a), len(b))
	}
	if a, b := formatOutline(t, -1, py), formatOutline(t, 0, py); len(a) != len(b) {
		t.Errorf("python Depth -1 = %d lines, want depth-0 count %d", len(a), len(b))
	}
	// The HTML default actually shows two levels: <head> and <body>
	// nest under <html>, and their children show too.
	joined := strings.Join(formatOutline(t, -1, html), "\n")
	for _, want := range []string{"<head>", "<body>", `<meta charset="utf-8">`, "<p>Hello world! This is HTML5 Boilerplate.</p>"} {
		if !strings.Contains(joined, want) {
			t.Errorf("html default depth missing %q", want)
		}
	}
	// An explicit --depth 0 still shows only the top level, even
	// for HTML: the default does not override a user's choice.
	if got := formatOutline(t, 0, html); len(got) != 2 {
		t.Errorf("html Depth 0 = %d lines, want 2 (doctype + html)", len(got))
	}
}
