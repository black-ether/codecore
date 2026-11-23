package main

import (
	"fmt"
	"io"
	"strings"
)

// TextFormatter renders one line per node:
//
//	[L:4]  package sample
//	[L:6]  import ( … (4 more)
//	[L:7]    "fmt"
//
// The [L:n] tag (1-based line number) always starts at column 0, padded
// to the widest tag in the output so the lines line up; children sit two
// spaces deeper than their parent. When several files are outlined, each
// file gets a "== path ==" header.
//
// Depth limits how many levels are shown: 0 is the top level only (the
// big picture — every block header, nothing inside it), 1 shows one
// level of children, and so on. A node whose children are hidden by the
// depth limit is marked " … (N more)", where N is how many outline
// lines hide inside it — so you can tell at a glance which blocks are
// worth opening, without opening them.
type TextFormatter struct {
	Depth int
}

// textRow is one rendered line: its line tag, its depth, its text, and
// how many outline lines hide inside it (0 when nothing is hidden).
type textRow struct {
	tag    string
	depth  int
	text   string
	hidden int
}

// Format writes outlines in text form.
func (f TextFormatter) Format(w io.Writer, outlines []*Outline) error {
	rows := make([][]textRow, len(outlines))
	width := 0
	for i, o := range outlines {
		rows[i] = collectRows(o.Nodes, 0, f.Depth)
		for _, r := range rows[i] {
			if len(r.tag) > width {
				width = len(r.tag)
			}
		}
	}
	for i, o := range outlines {
		if len(outlines) > 1 {
			if i > 0 {
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, "== %s ==\n", o.File)
		}
		for _, r := range rows[i] {
			marker := ""
			if r.hidden > 0 {
				marker = fmt.Sprintf(" … (%d more)", r.hidden)
			}
			fmt.Fprintf(w, "%-*s %s%s%s\n", width, r.tag, strings.Repeat("  ", r.depth), r.text, marker)
		}
	}
	return nil
}

// collectRows flattens nodes into rows in source order, up to maxDepth
// levels of children deep.
func collectRows(nodes []*Node, depth, maxDepth int) []textRow {
	var rows []textRow
	for _, n := range nodes {
		hidden := 0
		if len(n.Children) > 0 && depth >= maxDepth {
			hidden = countDescendants(n)
		}
		rows = append(rows, textRow{
			tag:    fmt.Sprintf("[L:%d]", n.Line),
			depth:  depth,
			text:   strings.TrimSpace(n.Text),
			hidden: hidden,
		})
		if len(n.Children) > 0 && depth < maxDepth {
			rows = append(rows, collectRows(n.Children, depth+1, maxDepth)...)
		}
	}
	return rows
}

// countDescendants returns how many outline lines hide inside a node
// (its children, grandchildren, and so on).
func countDescendants(n *Node) int {
	c := len(n.Children)
	for _, ch := range n.Children {
		c += countDescendants(ch)
	}
	return c
}
