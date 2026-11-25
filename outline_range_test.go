package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestGoOutlineSourceRanges(t *testing.T) {
	src := []byte(strings.Join([]string{
		"package p", "",
		"func simple(", " value int,", ") {",
		" if value > 0 {", "  // This row is not in the outline.", "  println(value)", " }",
		"", "}", "",
		"func typed(", " value int,", ") int {", " return value", "}",
		"func empty() {", " // A comment-only body.", "}",
	}, "\n"))
	ext, _ := ExtractorFor("source.go")
	outline, err := ext.Extract("source.go", src)
	if err != nil || outline.Unbalanced {
		t.Fatalf("outline = %+v, error = %v", outline, err)
	}
	applyGoDeclarationRanges(outline, src)
	collapsed := strings.Join(formatOutline(t, 0, outline), "\n")
	for _, want := range []string{
		"func simple( … (3 hidden outline rows) [source L:3-11]",
		"[source L:13-17]",
		"func empty() { [source L:18-20]",
	} {
		if !strings.Contains(collapsed, want) {
			t.Errorf("collapsed outline missing %q:\n%s", want, collapsed)
		}
	}
	expanded := strings.Join(formatOutline(t, 1, outline), "\n")
	for _, want := range []string{
		"[L:3]  func simple( [source L:3-11]",
		"[L:6]    if value > 0 { … (1 hidden outline row) [source L:6-9]",
	} {
		if !strings.Contains(expanded, want) {
			t.Errorf("expanded outline missing %q:\n%s", want, expanded)
		}
	}
	for _, depth := range []int{0, 2} {
		nodes := formattedRangeNodes(t, outline, depth)
		if node := nodes[1]; node.Line != 3 || node.EndLine != 11 || node.HasMore != (depth == 0) ||
			(depth == 0 && node.HiddenRows != 3) || (depth == 2 && node.HiddenRows != 0) {
			t.Fatalf("depth %d simple function = %+v", depth, node)
		}
		if depth == 2 {
			nested := nodes[1].Children[1]
			if nested.Line != 6 || nested.EndLine != 9 {
				t.Fatalf("nested if range = %+v", nested)
			}
		}
	}
}

func TestOutlineRangesByExtractor(t *testing.T) {
	for _, test := range []struct {
		name, source, label string
		end, outlineEnd     int
	}{
		{"sample.ps1", "function Get-Example\r\n{\r\n    1\r\n}\r\n", "[source L:1-4]", 4, 0},
		{"sample.html", "<div>\n  <img\n    alt='sample'\n  />\n</div>\n", "[source L:1-5]", 5, 0},
		{"sample.py", "def example():\n    # comment\n    return 1\n\nnext = 2\n", "[outline L:1-3]", 0, 3},
		{"broken.go", "func broken() {\n    println(1)\n", "[source L:1-?]", 0, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			ext, _ := ExtractorFor(test.name)
			outline, err := ext.Extract(test.name, []byte(test.source))
			if err != nil {
				t.Fatal(err)
			}
			lines := formatOutline(t, 0, outline)
			if !strings.HasSuffix(lines[0], test.label) {
				t.Errorf("range label = %q, want suffix %q", lines[0], test.label)
			}
			node := formattedRangeNodes(t, outline, 0)[0]
			if node.EndLine != test.end || node.OutlineEndLine != test.outlineEnd {
				t.Errorf("range = %+v, want endLine=%d outlineEndLine=%d", node, test.end, test.outlineEnd)
			}
			if test.name == "sample.html" {
				child := formattedRangeNodes(t, outline, 1)[0].Children[0]
				if child.Line != 2 || child.EndLine != 4 {
					t.Errorf("multiline void tag range = %+v", child)
				}
			}
		})
	}
}

func formattedRangeNodes(t *testing.T, outline *Outline, depth int) []*jsonNode {
	t.Helper()
	var buffer bytes.Buffer
	if err := (JSONFormatter{Depth: depth}).Format(&buffer, []*Outline{outline}); err != nil {
		t.Fatal(err)
	}
	var decoded []jsonOutline
	if err := json.Unmarshal(buffer.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded[0].Nodes
}
