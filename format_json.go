package main

import (
	"encoding/json"
	"io"
)

// JSONFormatter emits the outlines as indented JSON — the same tree as
// the text output, cut to the same depth. A node whose children are
// hidden by the depth limit carries "hasMore": true instead of a
// children list, so a reader can tell it opens further without the
// whole tree being dumped.
type JSONFormatter struct {
	Depth int
}

// jsonNode is one node in the JSON tree.
type jsonNode struct {
	Line     int         `json:"line"`
	Text     string      `json:"text"`
	HasMore  bool        `json:"hasMore,omitempty"`
	Children []*jsonNode `json:"children,omitempty"`
}

// jsonOutline is one file in the JSON output.
type jsonOutline struct {
	File       string      `json:"file"`
	Language   string      `json:"language"`
	Unbalanced bool        `json:"unbalanced,omitempty"`
	Nodes      []*jsonNode `json:"nodes"`
}

// Format writes outlines as a JSON array.
func (f JSONFormatter) Format(w io.Writer, outlines []*Outline) error {
	out := make([]*jsonOutline, 0, len(outlines))
	for _, o := range outlines {
		out = append(out, &jsonOutline{
			File:       o.File,
			Language:   o.Language,
			Unbalanced: o.Unbalanced,
			Nodes:      convertNodes(o.Nodes, 0, f.Depth),
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// convertNodes copies nodes to the JSON tree, dropping children past
// maxDepth and marking those nodes with HasMore.
func convertNodes(nodes []*Node, depth, maxDepth int) []*jsonNode {
	var out []*jsonNode
	for _, n := range nodes {
		jn := &jsonNode{Line: n.Line, Text: n.Text}
		if len(n.Children) > 0 {
			if depth < maxDepth {
				jn.Children = convertNodes(n.Children, depth+1, maxDepth)
			} else {
				jn.HasMore = true
			}
		}
		out = append(out, jn)
	}
	return out
}
