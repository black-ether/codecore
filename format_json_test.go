package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// JSON is cut to the same depth as text: children past the depth limit
// are dropped and the boundary node carries hasMore instead.
func TestJSONFormatterDepth(t *testing.T) {
	o := &Outline{File: "x.go", Language: "go", Nodes: []*Node{
		{Line: 1, Text: "func a() {", Children: []*Node{
			{Line: 2, Text: "if x {", Children: []*Node{
				{Line: 3, Text: "do()"},
			}},
		}},
	}}
	var buf bytes.Buffer
	if err := (JSONFormatter{Depth: 0}).Format(&buf, []*Outline{o}); err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || len(out[0]["nodes"].([]any)) != 1 {
		t.Fatalf("depth-0 JSON nodes = %v, want 1 top-level node", out)
	}
	node := out[0]["nodes"].([]any)[0].(map[string]any)
	if node["hasMore"] != true {
		t.Errorf("depth-0 JSON node missing hasMore: %v", node)
	}
	if _, ok := node["children"]; ok {
		t.Errorf("depth-0 JSON node should not carry children: %v", node)
	}

	buf.Reset()
	if err := (JSONFormatter{Depth: 1}).Format(&buf, []*Outline{o}); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	node = out[0]["nodes"].([]any)[0].(map[string]any)
	children := node["children"].([]any)
	if len(children) != 1 {
		t.Fatalf("depth-1 JSON children = %d, want 1", len(children))
	}
	inner := children[0].(map[string]any)
	if inner["hasMore"] != true {
		t.Errorf("depth-1 JSON inner node missing hasMore: %v", inner)
	}
	if _, ok := inner["children"]; ok {
		t.Errorf("depth-1 JSON inner node should not carry children: %v", inner)
	}

	// Deep enough to show the whole tree: no hasMore anywhere.
	buf.Reset()
	if err := (JSONFormatter{Depth: 2}).Format(&buf, []*Outline{o}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "hasMore") {
		t.Errorf("deep JSON should not mark hasMore:\n%s", buf.String())
	}
}
