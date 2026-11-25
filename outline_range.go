package main

import (
	"fmt"
	"go/parser"
	gotoken "go/token"
)

// Go signatures can end with a return type on a different line from the
// declaration. Use Go's parser to recover the physical declaration range
// without changing outline ancestry or matching spans.
func applyGoDeclarationRanges(outline *Outline, src []byte) {
	positions := gotoken.NewFileSet()
	file, err := parser.ParseFile(positions, outline.File, src, parser.SkipObjectResolution)
	if err != nil {
		return
	}
	ends := make(map[int]int)
	for _, decl := range file.Decls {
		start := positions.PositionFor(decl.Pos(), false).Line
		end := positions.PositionFor(decl.End()-1, false).Line
		ends[start] = max(ends[start], end)
	}
	for _, node := range outline.Nodes {
		if end, ok := ends[node.Line]; ok {
			node.SourceEnd = end
		}
	}
}

// outlineRange returns a source boundary and, when unavailable, the
// last visible line in the full outline subtree. Both are depth-independent.
func outlineRange(n *Node) (sourceEnd, outlineEnd int) {
	if n.SourceEnd > 0 {
		return n.SourceEnd, 0
	}
	if n.SourceEnd < 0 || len(n.Children) > 0 {
		return 0, endLine(n)
	}
	return 0, 0
}

func sourceRangeLabel(start, sourceEnd, outlineEnd int, unknown bool) string {
	switch {
	case sourceEnd > 0:
		return fmt.Sprintf("[source L:%d-%d]", start, sourceEnd)
	case unknown:
		return fmt.Sprintf("[source L:%d-?]", start)
	default:
		return fmt.Sprintf("[outline L:%d-%d]", start, outlineEnd)
	}
}
