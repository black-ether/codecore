package main

import (
	"slices"
	"strings"
	"testing"
)

func TestTemplateURL(t *testing.T) {
	for _, file := range []string{"example.mjs", "example.ts"} {
		t.Run(file, func(t *testing.T) {
			outline := mustExtractSource(t, file, "new URL(`https://docs.invalid/`);\n"+
				"function next() {\n  return 1;\n}\n")
			if outline.Unbalanced || len(outline.Nodes) != 2 {
				t.Fatalf("want two balanced top-level nodes, got %+v", outline)
			}
			if node := outline.Nodes[1]; node.Line != 2 || node.SourceEnd != 4 {
				t.Fatalf("next function range = %+v, want lines 2–4", node)
			}
		})
	}
}

func TestTemplateTextAndNestedInterpolation(t *testing.T) {
	src := strings.Join([]string{
		"const output = `",
		"  opaque ){ [ /* //",
		"  escaped \\` and \\${ not code",
		"  ${(() => {",
		"    // A comment containing } and `",
		"    const inner = `inner ${({ label: '}' }).label}",
		"  text [ // opaque",
		"  `;",
		"    return inner;",
		"  })()}",
		"  `;",
		"function next() {",
		"  return output;",
		"}",
	}, "\n")
	for _, file := range []string{"example.mjs", "example.ts"} {
		t.Run(file, func(t *testing.T) {
			outline := mustExtractSource(t, file, src)
			if outline.Unbalanced || len(outline.Nodes) != 2 {
				t.Fatalf("want a template and a top-level function, got %+v", outline)
			}
			for start, end := range map[int]int{1: 11, 4: 10, 6: 8, 12: 14} {
				node := findNode(outline, start)
				if node == nil || node.SourceEnd != end {
					t.Errorf("node at %d = %+v, want end %d", start, node, end)
				}
			}
			for _, line := range []int{2, 3, 5, 7, 8, 11} {
				if node := findNode(outline, line); node != nil {
					t.Errorf("template text, comment or closing delimiter became a node: %+v", node)
				}
			}
			if outer, expr, inner := findNode(outline, 1), findNode(outline, 4), findNode(outline, 6); findParent(outline, 4) != outer || findParent(outline, 6) != expr || inner == nil {
				t.Fatal("nested interpolation lost its enclosing structure")
			}
		})
	}
}

func TestUnclosedTemplateWarns(t *testing.T) {
	for _, src := range []string{"const value = `open", "const value = `open ${value"} {
		outline := mustExtractSource(t, "example.ts", src)
		if !outline.Unbalanced || len(outline.Nodes) != 1 || outline.Nodes[0].SourceEnd != -1 {
			t.Fatalf("want an unclosed template boundary for %q, got %+v", src, outline)
		}
	}
}

func TestTemplateTaskMarkdownLinkChecker(t *testing.T) {
	// Unmodified fixture from go-task/task at b5aef1c287a9be8038347a17646bdf4bc7185bce,
	// website/scripts/check-markdown-links.mjs. See the adjacent MIT license.
	outline := mustExtract(t, "testdata/task-template/check-markdown-links.mjs")
	if outline.Unbalanced {
		t.Fatal("the pinned link checker should fold without a bracket warning")
	}
	var top []int
	for _, node := range outline.Nodes {
		top = append(top, node.Line)
	}
	want := []int{1, 2, 3, 4, 8, 9, 12, 14, 15, 16, 18, 39, 44, 47}
	if !slices.Equal(top, want) {
		t.Fatalf("top-level lines = %v, want %v", top, want)
	}
	for start, end := range map[int]int{18: 37, 39: 42, 44: 46} {
		if node := findNode(outline, start); node == nil || node.SourceEnd != end {
			t.Errorf("node at %d = %+v, want end %d", start, node, end)
		}
	}
}

func TestGoBackticksRemainRaw(t *testing.T) {
	outline := mustExtractSource(t, "example.go", "const payload = `\n${\n// ( ] }\n`\nfunc next() {\n}\n")
	if outline.Unbalanced || len(outline.Nodes) != 2 || outline.Nodes[1].Line != 5 ||
		outline.Nodes[1].SourceEnd != 6 {
		t.Fatalf("Go raw string or following function misfolded: %+v", outline)
	}
}
