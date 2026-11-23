package main

import (
	"os"
	"path/filepath"
	"testing"
)

func mustExtract(t *testing.T, path string) *Outline {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ext, ok := ExtractorFor(path)
	if !ok {
		t.Fatalf("no extractor for %s", path)
	}
	o, err := ext.Extract(path, src)
	if err != nil {
		t.Fatalf("extract %s: %v", path, err)
	}
	return o
}

// mustExtractSource extracts inline source as if it were a file with
// the given name (the extension picks the language).
func mustExtractSource(t *testing.T, filename, src string) *Outline {
	t.Helper()
	path := filepath.Join(t.TempDir(), filename)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return mustExtract(t, path)
}

func walkNodes(o *Outline, fn func(*Node)) {
	var walk func(n *Node)
	walk = func(n *Node) {
		fn(n)
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, n := range o.Nodes {
		walk(n)
	}
}

// countNodes returns the total node count, the count of nodes with
// children (blocks), and the maximum depth (0 for a flat outline).
func countNodes(o *Outline) (total, blocks, maxDepth int) {
	var walk func(ns []*Node, depth int)
	walk = func(ns []*Node, depth int) {
		for _, n := range ns {
			total++
			if depth > maxDepth {
				maxDepth = depth
			}
			if len(n.Children) > 0 {
				blocks++
				walk(n.Children, depth+1)
			}
		}
	}
	walk(o.Nodes, 0)
	return total, blocks, maxDepth
}

func findNode(o *Outline, line int) *Node {
	var found *Node
	walkNodes(o, func(n *Node) {
		if n.Line == line {
			found = n
		}
	})
	return found
}

// findParent returns the parent of the node at the given line, or nil
// when it is top-level (or absent).
func findParent(o *Outline, line int) *Node {
	var parent *Node
	var walk func(n *Node, p *Node)
	walk = func(n *Node, p *Node) {
		if parent != nil {
			return
		}
		if n.Line == line {
			parent = p
			return
		}
		for _, c := range n.Children {
			walk(c, n)
		}
	}
	for _, n := range o.Nodes {
		walk(n, nil)
	}
	return parent
}

func TestFoldSampleGo(t *testing.T) {
	o := mustExtract(t, "testdata/sample.go")
	if o.Unbalanced {
		t.Error("sample.go should not be unbalanced")
	}
	total, blocks, maxDepth := countNodes(o)
	if total != 42 || blocks != 10 || maxDepth != 2 {
		t.Errorf("sample.go: total=%d blocks=%d maxDepth=%d, want 42/10/2", total, blocks, maxDepth)
	}

	// The import group folds under its header line; the strings
	// inside it are children, not top-level nodes.
	imp := findNode(o, 6)
	if imp == nil || imp.Text != "import (" {
		t.Fatalf("import group header not found at L:6 (got %v)", imp)
	}
	if len(imp.Children) != 4 {
		t.Errorf("import group children = %d, want 4", len(imp.Children))
	}
	if p := findParent(o, 7); p != imp {
		t.Errorf(`"fmt" parent = %v, want the import group`, p)
	}

	// Struct/interface fields nest under their type.
	cfg := findNode(o, 34)
	if cfg == nil || len(cfg.Children) != 4 {
		t.Errorf("Config children = %d, want 4 fields", len(cfg.Children))
	}
	greeter := findNode(o, 47)
	if greeter == nil || len(greeter.Children) != 2 {
		t.Errorf("Greeter children = %d, want 2", len(greeter.Children))
	}

	// Function bodies nest: Map's for loop nests under Map.
	m := findNode(o, 61)
	if m == nil || len(m.Children) != 3 {
		t.Errorf("Map children = %d, want 3", len(m.Children))
	}
	if p := findParent(o, 64); p == nil || p.Line != 63 {
		t.Errorf("out[i] parent line = %v, want 63 (the for loop)", p)
	}

	// Traps: a "func" inside a string literal is just text on its
	// line, and the line is a leaf — it never opens a block. The
	// anonymous func literal is a leaf too.
	trap := findNode(o, 78)
	if trap == nil || len(trap.Children) != 0 {
		t.Errorf("trap line should be a leaf, got %v", trap)
	}
	if trap != nil && trap.Text != `trap := "func fake() {}"` {
		t.Errorf("trap text = %q", trap.Text)
	}
	if anon := findNode(o, 81); anon == nil || len(anon.Children) != 0 {
		t.Errorf("anon func line should be a leaf, got %v", anon)
	}

	// Top-level nodes stay in source order.
	prev := 0
	for _, n := range o.Nodes {
		if n.Line < prev {
			t.Errorf("top-level nodes out of order at [L:%d]", n.Line)
		}
		prev = n.Line
	}
}

func TestFoldPython(t *testing.T) {
	o := mustExtract(t, "testdata/sample.py")
	if o.Unbalanced {
		t.Error("sample.py should not be unbalanced")
	}
	total, blocks, maxDepth := countNodes(o)
	if total != 34 || blocks != 12 || maxDepth != 2 {
		t.Errorf("sample.py: total=%d blocks=%d maxDepth=%d, want 34/12/2", total, blocks, maxDepth)
	}

	// Decorators merge into the following definition's header.
	dec := findNode(o, 28)
	if dec == nil || dec.Text != "@decorator def decorated():" {
		t.Errorf("decorated header = %v, want \"@decorator def decorated():\"", dec)
	}

	// Class members nest under their class.
	animal := findNode(o, 32)
	if animal == nil || len(animal.Children) != 6 {
		t.Errorf("Animal children = %d, want 6", len(animal.Children))
	}
	dog := findNode(o, 51)
	if dog == nil || len(dog.Children) != 1 {
		t.Errorf("Dog children = %d, want 1 (speak)", len(dog.Children))
	}

	// The main guard is a block; the call inside it nests under it.
	guard := findNode(o, 60)
	if guard == nil || len(guard.Children) != 1 || guard.Children[0].Line != 61 {
		t.Errorf("main-guard children = %v, want [L:61]", guard)
	}

	// Trap: a "def" inside a string is a leaf, never a block.
	trap := findNode(o, 13)
	if trap == nil || len(trap.Children) != 0 {
		t.Errorf("TRAP_STRING line should be a leaf, got %v", trap)
	}
}

func TestFoldPowerShell(t *testing.T) {
	// The .ps1 fixture is left out of the emailed zip (Gmail blocks .ps1
	// attachments); skip instead of failing when it is absent.
	if _, err := os.Stat("testdata/sample.ps1"); err != nil {
		t.Skip("testdata/sample.ps1 not present (excluded from the emailed zip: Gmail blocks .ps1 attachments)")
	}
	o := mustExtract(t, "testdata/sample.ps1")
	if o.Unbalanced {
		t.Error("sample.ps1 should not be unbalanced")
	}
	total, blocks, maxDepth := countNodes(o)
	if total != 28 || blocks != 8 || maxDepth != 2 {
		t.Errorf("sample.ps1: total=%d blocks=%d maxDepth=%d, want 28/8/2", total, blocks, maxDepth)
	}

	// Class members nest under the class (the constructor shares the
	// class's name, so look it up by line).
	widget := findNode(o, 31)
	if widget == nil || len(widget.Children) != 5 {
		t.Errorf("Widget children = %d, want 5 (2 properties + 3 methods)", len(widget.Children))
	}

	// A param( block nests under its function.
	if p := findParent(o, 12); p == nil || p.Line != 11 {
		t.Errorf("param( parent line = %v, want 11 (Get-Thing)", p)
	}

	// The single-line scriptblock is a leaf, not a block.
	sb := findNode(o, 6)
	if sb == nil || len(sb.Children) != 0 {
		t.Errorf("$sb line should be a leaf, got %v", sb)
	}

	// Traps: a "function" inside a string and one inside a comment are
	// just text; neither opens a block.
	for _, line := range []int{8} {
		if n := findNode(o, line); n == nil || len(n.Children) != 0 {
			t.Errorf("trap line L:%d should be a leaf, got %v", line, n)
		}
	}
}

// Nested functions nest under their enclosing function and variables
// under a block nest under that block, instead of flattening to the
// top level.
func TestFoldPythonNesting(t *testing.T) {
	o := mustExtractSource(t, "nest.py", `import os

VERSION = "1.0"

class Animal:
    species = "mammal"
    def __init__(self, name):
        self.name = name

def outer():
    def inner():
        pass

if __name__ == "__main__":
    mode = "run"
`)
	if p := findParent(o, 11); p == nil || p.Line != 10 {
		t.Errorf("inner's parent line = %v, want 10 (outer)", p)
	}
	guard := findNode(o, 14)
	if guard == nil || len(guard.Children) != 1 {
		t.Fatalf("main-guard block not found at L:14 (got %v)", guard)
	}
	if p := findParent(o, 15); p != guard {
		t.Errorf("mode's parent = %v, want the main-guard block", p)
	}
	if p := findParent(o, 3); p != nil {
		t.Errorf("VERSION's parent = %v, want top level", p)
	}
}

// Every code line shows — there is no kind classification and nothing
// is skipped — and nesting follows the blocks.
func TestFoldPowerShellNesting(t *testing.T) {
	o := mustExtractSource(t, "nest.ps1", `Import-Module MyModule
Import-Module -Name Foo
Import-Module -Name $modPath -Force -Scope Global -ErrorAction Stop
Import-Module -Name "Bar.psm1"
Import-Module -Force -Scope Global
function Outer {
    param($x)
    function Inner { $a = 1 }
    $top = 2
}
if ($cond) {
    $insideIf = 1
    $alsoIf = 2
}
$list | ForEach-Object -Parallel {
    $p = $using:foo
}
$after = 3
`)
	// All five Import-Module lines are top-level leaves, including
	// the flags-only one: the fold model shows every code line.
	for _, line := range []int{1, 2, 3, 4, 5} {
		n := findNode(o, line)
		if n == nil || len(n.Children) != 0 {
			t.Errorf("Import-Module line L:%d should be a top-level leaf, got %v", line, n)
		}
		if p := findParent(o, line); p != nil {
			t.Errorf("Import-Module line L:%d parent = %v, want top level", line, p)
		}
	}
	if p := findParent(o, 8); p == nil || p.Line != 6 {
		t.Errorf("Inner's parent line = %v, want 6 (Outer)", p)
	}
	if p := findParent(o, 9); p == nil || p.Line != 6 {
		t.Errorf("$top's parent line = %v, want 6 (Outer)", p)
	}
	for _, line := range []int{12, 13} {
		if p := findParent(o, line); p == nil || p.Line != 11 {
			t.Errorf("if-block var L:%d parent line = %v, want 11", line, p)
		}
	}
	if p := findParent(o, 16); p == nil || p.Line != 15 {
		t.Errorf("$p's parent line = %v, want 15 (the parallel scriptblock)", p)
	}
	if p := findParent(o, 18); p != nil {
		t.Errorf("$after's parent = %v, want top level", p)
	}
}

// A block that opens and closes on one line is a leaf; } else { is a
// block header of its own at the same depth as the if; a multi-line
// hashtable is a block.
func TestFoldSingleLineBlocks(t *testing.T) {
	o := mustExtractSource(t, "blocks.ps1", "if ($x) { $y = 1 }\nif ($a) {\n    $b = 2\n} else {\n    $c = 3\n}\n$h = @{\n    k = 1\n    j = 2\n}\n")
	if n := findNode(o, 1); n == nil || len(n.Children) != 0 {
		t.Errorf("single-line if should be a leaf, got %v", n)
	}
	if n := findNode(o, 2); n == nil || len(n.Children) != 1 || n.Children[0].Line != 3 {
		t.Errorf("if block children = %v, want [L:3]", n)
	}
	elseNode := findNode(o, 4)
	if elseNode == nil || len(elseNode.Children) != 1 || elseNode.Children[0].Line != 5 {
		t.Errorf("else block children = %v, want [L:5]", elseNode)
	}
	if p := findParent(o, 4); p != nil {
		t.Errorf("else block parent = %v, want top level (sibling of the if)", p)
	}
	hash := findNode(o, 7)
	if hash == nil || len(hash.Children) != 2 {
		t.Errorf("hashtable children = %d, want 2", len(hash.Children))
	}
}

// A ” escaped quote inside a PowerShell literal string must not
// swallow the rest of the line: the block after it still nests
// correctly, and the file is not unbalanced.
func TestFoldEscapedQuote(t *testing.T) {
	o := mustExtractSource(t, "quote.ps1", "$s = 'it''s'\nif ($x) {\n    $y = 1\n}\n")
	if o.Unbalanced {
		t.Error("escaped-quote file should not be unbalanced")
	}
	if n := findNode(o, 2); n == nil || len(n.Children) != 1 || n.Children[0].Line != 3 {
		t.Errorf("if block children = %v, want [L:3]", n)
	}
	if p := findParent(o, 2); p != nil {
		t.Errorf("if block parent = %v, want top level", p)
	}
}

// A multi-line call, list or dict nests under its first line: its
// contents are inside it, not top-level statements, and a bare closer
// is not shown at all.
func TestFoldPythonContinuations(t *testing.T) {
	o := mustExtractSource(t, "cont.py", `result = some_function(
    arg1,
    arg2,
)
config = {
    "host": "localhost",
    "port": 8080,
}
def main():
    print(result)
`)
	call := findNode(o, 1)
	if call == nil || len(call.Children) != 2 {
		t.Errorf("call children = %d, want 2 (arg1, arg2)", len(call.Children))
	}
	if p := findParent(o, 2); p != call {
		t.Errorf("arg1's parent = %v, want the call line", p)
	}
	// The bare closers are not nodes.
	for _, line := range []int{4, 8} {
		if n := findNode(o, line); n != nil {
			t.Errorf("bare closer L:%d should not be a node, got %v", line, n)
		}
	}
	dict := findNode(o, 5)
	if dict == nil || len(dict.Children) != 2 {
		t.Errorf("dict children = %d, want 2 (host, port)", len(dict.Children))
	}
	if p := findParent(o, 6); p != dict {
		t.Errorf("host's parent = %v, want the dict line", p)
	}
	total, blocks, _ := countNodes(o)
	if total != 8 || blocks != 3 {
		t.Errorf("continuations: total=%d blocks=%d, want 8/3", total, blocks)
	}
}

// A '{' on a line of its own (Allman style) attaches its block to the
// previous code line, so the nesting stays one level at a time: the
// body's lines sit under the 'if', not beside it.
func TestFoldAllman(t *testing.T) {
	o := mustExtractSource(t, "allman.go", `package demo

func check(x bool) {
    if x
    {
        DoThing()
    }
    else
    {
        DoOther()
    }
}
`)
	if o.Unbalanced {
		t.Error("allman file should not be unbalanced")
	}
	ifNode := findNode(o, 4)
	if ifNode == nil || len(ifNode.Children) != 1 || ifNode.Children[0].Line != 6 {
		t.Errorf("if children = %v, want [L:6] (DoThing)", ifNode)
	}
	if p := findParent(o, 6); p != ifNode {
		t.Errorf("DoThing's parent = %v, want the if line", p)
	}
	if p := findParent(o, 8); p == nil || p.Line != 3 {
		t.Errorf("else's parent line = %v, want 3 (the func)", p)
	}
	if p := findParent(o, 10); p == nil || p.Line != 8 {
		t.Errorf("DoOther's parent line = %v, want 8 (the else)", p)
	}
	// The lone '{' lines are not nodes.
	for _, line := range []int{5, 9} {
		if n := findNode(o, line); n != nil {
			t.Errorf("lone brace L:%d should not be a node, got %v", line, n)
		}
	}
}

// A /* */ block comment and a // line comment are not statements
// and are not shown. A string that happens to contain /* stays —
// only a comment at the start of a line is skipped.
func TestFoldPythonCStyleComments(t *testing.T) {
	o := mustExtractSource(t, "comments.py", `/* A block comment.
Spans two lines.
*/
/** A doc comment. */
function f() {
    // a line comment
    let url = "https://example.com/a/*b"
    return url
}
`)
	for _, line := range []int{1, 2, 3, 4, 6} {
		if n := findNode(o, line); n != nil {
			t.Errorf("comment L:%d should not be a node, got %v", line, n)
		}
	}
	if n := findNode(o, 7); n == nil {
		t.Error("string with /* inside it (L:7) should still be a node")
	}
	if n := findNode(o, 5); n == nil {
		t.Error("function header L:5 should be a node")
	}
	total, blocks, _ := countNodes(o)
	if total != 3 || blocks != 1 {
		t.Errorf("comments: total=%d blocks=%d, want 3/1", total, blocks)
	}
}

// A #[...] attribute line is not a comment: in Rust, # starts an
// attribute, not a comment. A plain # line is still skipped.
func TestFoldPythonAttributes(t *testing.T) {
	o := mustExtractSource(t, "attrs.py", `#[cfg(test)]
mod tests {
    # a comment
    #[test]
    fn descending() {
        assert!(true)
    }
}
`)
	if n := findNode(o, 1); n == nil {
		t.Error("#[cfg(test)] (L:1) should be a node")
	}
	if n := findNode(o, 3); n != nil {
		t.Errorf("# comment L:3 should not be a node, got %v", n)
	}
	if n := findNode(o, 4); n == nil {
		t.Error("#[test] (L:4) should be a node")
	}
	total, blocks, _ := countNodes(o)
	if total != 5 || blocks != 2 {
		t.Errorf("attributes: total=%d blocks=%d, want 5/2", total, blocks)
	}
}

// A line that is only a triple-quoted string (a docstring) is not
// shown — it is pure symbols, or text with no structure. But a
// string on a line of its own inside a multi-line call is an
// argument, not a docstring, and it stays.
func TestFoldPythonDocstrings(t *testing.T) {
	o := mustExtractSource(t, "docs.py", `"""Module docstring.
Spans two lines.
"""
def greet(name):
    """Say hello."""
    return "hi " + name

result = some_function(
    """a long string argument""",
    arg2,
)
`)
	// The module docstring (lines 1-3) and the function docstring
	// (line 5) are not nodes.
	for _, line := range []int{1, 2, 3, 5} {
		if n := findNode(o, line); n != nil {
			t.Errorf("docstring L:%d should not be a node, got %v", line, n)
		}
	}
	// The string argument stays, nested under the call.
	if n := findNode(o, 9); n == nil {
		t.Error("string argument L:9 should be a node")
	}
	if p := findParent(o, 9); p == nil || p.Line != 8 {
		t.Errorf("string argument's parent line = %v, want 8 (the call)", p)
	}
	total, blocks, _ := countNodes(o)
	if total != 5 || blocks != 2 {
		t.Errorf("docstrings: total=%d blocks=%d, want 5/2", total, blocks)
	}
}

// A multi-line signature whose closer sits on its own line (") {")
// does not split the function in two: the body nests under the
// header line, and the closer line is not a node.
func TestFoldSplitHeader(t *testing.T) {
	o := mustExtractSource(t, "split.go", `package demo

func doThing(
    x int,
    y int,
) {
    println(x)
}
`)
	if o.Unbalanced {
		t.Error("split-header file should not be unbalanced")
	}
	if n := findNode(o, 6); n != nil {
		t.Errorf("closer line L:6 should not be a node, got %v", n)
	}
	fn := findNode(o, 3)
	if fn == nil || len(fn.Children) != 3 {
		t.Errorf("func children = %d, want 3 (x, y, println)", len(fn.Children))
	}
	if p := findParent(o, 7); p != fn {
		t.Errorf("println's parent = %v, want the func line", p)
	}
	total, blocks, _ := countNodes(o)
	if total != 5 || blocks != 1 {
		t.Errorf("split header: total=%d blocks=%d, want 5/1", total, blocks)
	}
}

// A broken file still yields an outline instead of an error: the
// package and the func are there, and Unbalanced is set because the
// func's brackets never close. (The .txt fixtures have no extractor by
// extension, so the Go extractor is picked by language.)
func TestFoldBroken(t *testing.T) {
	ext, ok := ExtractorByLanguage("go")
	if !ok {
		t.Fatal("no Go extractor")
	}
	src, err := os.ReadFile("testdata/broken.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	o, err := ext.Extract("testdata/broken.go.txt", src)
	if err != nil {
		t.Fatalf("extract broken.go.txt: %v", err)
	}
	if !o.Unbalanced {
		t.Error("expected Unbalanced for broken.go.txt")
	}
	total, _, _ := countNodes(o)
	if total != 2 {
		t.Errorf("broken.go.txt nodes = %d, want 2 (package + func)", total)
	}
	if n := findNode(o, 1); n == nil || n.Text != "package broken" {
		t.Errorf("package node = %v, want 'package broken'", n)
	}
}

// A stray closer marks the file Unbalanced but the outline is still
// produced; an unclosed opener does the same.
func TestFoldUnbalanced(t *testing.T) {
	o := mustExtractSource(t, "stray.go", "package x\n\nfunc f() {\n    return\n}\n}\n")
	if !o.Unbalanced {
		t.Error("expected Unbalanced for the stray closer")
	}
	total, _, _ := countNodes(o)
	if total != 3 {
		t.Errorf("stray.go nodes = %d, want 3 (package, func, return)", total)
	}
	o2 := mustExtractSource(t, "open.go", "package x\n\nfunc f() {\n    return\n")
	if !o2.Unbalanced {
		t.Error("expected Unbalanced for the unclosed func")
	}
}

// An empty file yields an empty outline, no error.
func TestFoldEmpty(t *testing.T) {
	ext, ok := ExtractorByLanguage("go")
	if !ok {
		t.Fatal("no Go extractor")
	}
	src, err := os.ReadFile("testdata/empty.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	o, err := ext.Extract("testdata/empty.go.txt", src)
	if err != nil {
		t.Fatalf("extract empty.go.txt: %v", err)
	}
	if o.Unbalanced {
		t.Error("empty file should not be unbalanced")
	}
	if len(o.Nodes) != 0 {
		t.Errorf("empty file: got %d nodes, want 0", len(o.Nodes))
	}
}

// The real-world check: pp.ps1, Jo's 2,565-line script. Skipped when
// the file is not beside the project (it is Jo's private script, not a
// fixture). The fold must recover the three assignments the old
// tree-sitter grammar lost, nest the string-labeled switch clauses,
// and match the prototype's counts exactly.
func TestFoldPP(t *testing.T) {
	path := "../pp-report/pp.ps1"
	if _, err := os.Stat(path); err != nil {
		t.Skip("pp.ps1 not present beside the project")
	}
	o := mustExtract(t, path)
	if o.Unbalanced {
		t.Error("pp.ps1 should not be unbalanced")
	}
	total, blocks, maxDepth := countNodes(o)
	if total != 1494 || blocks != 440 || maxDepth != 14 {
		t.Errorf("pp.ps1: total=%d blocks=%d maxDepth=%d, want 1494/440/14", total, blocks, maxDepth)
	}
	// The three assignments the old grammar lost are present.
	for _, line := range []int{2381, 2382, 2383} {
		if n := findNode(o, line); n == nil {
			t.Errorf("lost assignment L:%d missing from the outline", line)
		}
	}
	// The switch nests its string-labeled clauses.
	sw := findNode(o, 192)
	if sw == nil {
		t.Fatal("switch not found at L:192")
	}
	found := false
	for _, c := range sw.Children {
		if c.Line == 193 {
			found = true
		}
	}
	if !found {
		t.Error("switch child 'add' (L:193) not found under the switch")
	}
}

func BenchmarkFoldSample(b *testing.B) {
	src, err := os.ReadFile("testdata/sample.go")
	if err != nil {
		b.Fatal(err)
	}
	ext, ok := ExtractorFor("testdata/sample.go")
	if !ok {
		b.Fatal("no extractor for sample.go")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ext.Extract("testdata/sample.go", src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFoldPython(b *testing.B) {
	src, err := os.ReadFile("testdata/sample.py")
	if err != nil {
		b.Fatal(err)
	}
	ext, ok := ExtractorFor("testdata/sample.py")
	if !ok {
		b.Fatal("no extractor for sample.py")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ext.Extract("testdata/sample.py", src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFoldPowerShell(b *testing.B) {
	src, err := os.ReadFile("testdata/sample.ps1")
	if err != nil {
		b.Fatal(err)
	}
	ext, ok := ExtractorFor("testdata/sample.ps1")
	if !ok {
		b.Fatal("no extractor for sample.ps1")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ext.Extract("testdata/sample.ps1", src); err != nil {
			b.Fatal(err)
		}
	}
}
