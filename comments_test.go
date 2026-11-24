package main

import (
	"strings"
	"testing"
)

// Standalone comment blocks are extracted per language: a run of
// consecutive line comments is one block, a block comment is one
// block. A marker after code is a trailing comment, already part of
// that line's node, so it is not extracted.
func TestExtractCommentsGo(t *testing.T) {
	src := `package main

// Check for Configuration File
// (the second line of the run)
func CheckConfig() {}

x := 1 // initialize the counter

/* A block comment
   spanning two lines */
func Other() {}

//
func Third() {}
`
	blocks := extractComments(strings.Split(src, "\n"), "go")
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks %+v, want 2", len(blocks), blocks)
	}
	if blocks[0].start != 3 || blocks[0].end != 4 || !strings.Contains(blocks[0].text, "Configuration File") {
		t.Errorf("block 0 = %+v, want the two-line // run at L:3-4", blocks[0])
	}
	if blocks[1].start != 9 || blocks[1].end != 10 || !strings.Contains(blocks[1].text, "spanning two lines") {
		t.Errorf("block 1 = %+v, want the /* */ block at L:9-10", blocks[1])
	}
	// The trailing "// initialize" is not extracted (it is already
	// part of its line's node), and the lone "//" line is dropped
	// for having no text.
	for _, b := range blocks {
		if strings.Contains(b.text, "initialize") {
			t.Errorf("block %+v should not exist", b)
		}
	}
}

func TestExtractCommentsPython(t *testing.T) {
	src := `# module header
# second line
def foo():
    pass
`
	blocks := extractComments(strings.Split(src, "\n"), "python")
	if len(blocks) != 1 || blocks[0].start != 1 || blocks[0].end != 2 || !strings.Contains(blocks[0].text, "module header") {
		t.Fatalf("got %+v, want one # block at L:1-2", blocks)
	}
}

func TestExtractCommentsPowerShell(t *testing.T) {
	src := `<# Block comment
spanning lines #>
# line comment
function Get-Thing { }
`
	blocks := extractComments(strings.Split(src, "\n"), "powershell")
	if len(blocks) != 2 {
		t.Fatalf("got %+v, want 2 blocks", blocks)
	}
	if !strings.Contains(blocks[0].text, "spanning lines") {
		t.Errorf("block 0 = %+v, want the <# #> block", blocks[0])
	}
	if blocks[1].text != "line comment" {
		t.Errorf("block 1 = %+v, want the # line", blocks[1])
	}
}

func TestExtractCommentsHTML(t *testing.T) {
	src := `<!-- page header -->
<div>
  <!-- inner note -->
  <p>hi</p>
</div>
`
	blocks := extractComments(strings.Split(src, "\n"), "html")
	if len(blocks) != 2 {
		t.Fatalf("got %+v, want 2 blocks", blocks)
	}
	if blocks[0].text != "page header" || blocks[1].text != "inner note" {
		t.Errorf("got %+v, want the two <!-- --> texts", blocks)
	}
}

func TestExtractCommentsUnknownLanguage(t *testing.T) {
	blocks := extractComments([]string{"// not a comment here"}, "json")
	if len(blocks) != 0 {
		t.Errorf("got %+v, want none for a language with no comments", blocks)
	}
}

// A comment block attaches to the next outline node in source order
// — the code it describes — and a block after the last node attaches
// to the last node.
func TestAttachComments(t *testing.T) {
	src := `package main

// Check for Configuration File
func CheckConfig() {}

// trailing note
`
	o := mustExtractSource(t, "test.go", src)
	attachComments(o, []byte(src))
	var found *Node
	var walk func(nodes []*Node)
	walk = func(nodes []*Node) {
		for _, n := range nodes {
			if strings.Contains(n.Text, "CheckConfig") {
				found = n
			}
			walk(n.Children)
		}
	}
	walk(o.Nodes)
	if found == nil {
		t.Fatal("CheckConfig node not found in outline")
	}
	// Both blocks attach to CheckConfig: the doc comment above it,
	// and the trailing note (no node follows it, so it lands on the
	// last node).
	if len(found.Comments) != 2 || !strings.Contains(found.Comments[0], "Configuration File") || !strings.Contains(found.Comments[1], "trailing note") {
		t.Errorf("CheckConfig.Comments = %v, want [Configuration File, trailing note]", found.Comments)
	}
}

// End to end: a word that only appears in a comment finds the
// function the comment describes. This is the pp.ps1 case — "0.2
// Check for Configuration File" above the code that checks it.
func TestCommentFindable(t *testing.T) {
	src := `package main

// Check for Configuration File
func CheckConfig() {}

func Other() {}
`
	o := mustExtractSource(t, "test.go", src)
	attachComments(o, []byte(src))
	got := retrieve([]*Outline{o}, tokenize("configuration"), 5, false, false, false)
	if len(got) != 1 {
		t.Fatalf("got %+v, want 1 result", got)
	}
	if !strings.Contains(got[0].Text, "CheckConfig") {
		t.Errorf("result = %+v, want the CheckConfig function, not the comment line", got[0])
	}
}

// A Python docstring attaches to the node it documents (the
// def/class above it, not the next node), and a module docstring
// attaches to the first node. The attachment happens inside the
// parser, so mustExtractSource alone is enough — no attachComments
// call.
func TestDocstringAttached(t *testing.T) {
	src := `"""Module overview: sorting helpers."""

def sort_items(items):
    """Sort the list in place, stable."""
    items.sort()

def other():
    pass
`
	o := mustExtractSource(t, "test.py", src)
	var sortNode, firstNode *Node
	var walk func(nodes []*Node)
	walk = func(nodes []*Node) {
		for _, n := range nodes {
			if firstNode == nil {
				firstNode = n
			}
			if strings.Contains(n.Text, "sort_items") {
				sortNode = n
			}
			walk(n.Children)
		}
	}
	walk(o.Nodes)
	if sortNode == nil || firstNode == nil {
		t.Fatal("sort_items node not found in outline")
	}
	// Both docstrings land on sort_items (the first node): the
	// module docstring (no node precedes it) and the function
	// docstring (the def above it), in source order.
	if len(sortNode.Comments) != 2 || !strings.Contains(sortNode.Comments[0], "Module overview") || !strings.Contains(sortNode.Comments[1], "stable") {
		t.Errorf("sort_items.Comments = %v, want [Module overview, Sort the list in place, stable]", sortNode.Comments)
	}
}

// A single-line docstring attaches the same way.
func TestDocstringSingleLine(t *testing.T) {
	src := `def greet(name):
    """Say hello by name."""
    return "hi " + name
`
	o := mustExtractSource(t, "test.py", src)
	var greet *Node
	var walk func(nodes []*Node)
	walk = func(nodes []*Node) {
		for _, n := range nodes {
			if strings.Contains(n.Text, "greet") {
				greet = n
			}
			walk(n.Children)
		}
	}
	walk(o.Nodes)
	if greet == nil {
		t.Fatal("greet node not found")
	}
	if len(greet.Comments) != 1 || !strings.Contains(greet.Comments[0], "Say hello") {
		t.Errorf("greet.Comments = %v, want the single-line docstring", greet.Comments)
	}
}

// End to end: a word that only appears in a docstring finds the
// function it documents.
func TestDocstringFindable(t *testing.T) {
	src := `def sort_items(items):
    """Sort the list in place, stable."""
    items.sort()

def other():
    pass
`
	o := mustExtractSource(t, "test.py", src)
	got := retrieve([]*Outline{o}, tokenize("stable"), 5, false, false, false)
	if len(got) != 1 {
		t.Fatalf("got %+v, want 1 result", got)
	}
	if !strings.Contains(got[0].Text, "sort_items") {
		t.Errorf("result = %+v, want the sort_items function, not the docstring line", got[0])
	}
}
