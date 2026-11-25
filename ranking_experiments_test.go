package main

import (
	"math"
	"slices"
	"testing"
)

func TestRankingCoverageUsesDistinctTerms(t *testing.T) {
	o := &Outline{File: "sample.go", Nodes: []*Node{
		{Line: 1, Text: "checksum checksum checksum checksum"},
		{Line: 2, Text: "checksum execution"},
	}}
	opts := retrievalOptions{rankMode: "coverage", explainRanking: true}
	got := opts.retrieve([]*Outline{o}, []string{"checksum", "checksum", "execution"}, 5, false, false, true)
	if got[0].Line != 2 || got[0].Explanation.Covered != 2 || got[0].Explanation.Adjustment <= 0 ||
		got[1].Explanation.Covered != 1 || got[1].Explanation.Adjustment != 0 {
		t.Fatalf("distinct term scores = %+v", got)
	}
}

func TestRankingHeaderContext(t *testing.T) {
	o := &Outline{Language: "go", File: "sample.go", Nodes: []*Node{
		{Line: 1, Text: "func KeepChecksum() {", Comments: []string{"execution execution"}, Children: []*Node{
			{Line: 2, Text: "return fresh"},
			{Line: 3, Text: "irrelevant()"},
		}},
		{Line: 5, Text: "return fresh"},
	}}
	opts := retrievalOptions{rankMode: "context", explainRanking: true, splitIdentifiers: true}
	got := opts.retrieve([]*Outline{o}, []string{"fresh", "checksum", "execution"}, 5, false, false, true)
	for _, row := range got {
		if row.Line == 3 {
			t.Fatal("header context created an unevidenced result")
		}
		if row.Line == 2 {
			if row.Explanation.Adjustment != 4 || !slices.Equal(row.Explanation.SupportingTerms, []string{"checksum", "execution"}) {
				t.Fatalf("context score = %+v", row.Explanation)
			}
			return
		}
	}
	t.Fatal("missing contextual evidence")
}

func TestCompactUnitRankingKeepsPreciseEvidence(t *testing.T) {
	o := &Outline{Language: "go", File: "sample.go", Nodes: []*Node{
		{Line: 1, Text: "func operation() {", Children: []*Node{
			{Line: 2, Text: "return ready"},
			{Line: 3, Text: "checksum()"},
			{Line: 4, Text: "checksum()"},
			{Line: 5, Text: "execution()"},
		}},
		{Line: 7, Text: "func alternative() {", Children: []*Node{
			{Line: 8, Text: "return ready"},
		}},
	}}
	opts := retrievalOptions{rankMode: "unit", explainRanking: true}
	got := opts.retrieve([]*Outline{o}, []string{"ready", "checksum", "execution"}, 5, false, false, true)
	row := got[0]
	if len(got) != 2 || row.Unit == nil || row.Unit.Start != 1 || len(row.Support) != 2 ||
		row.Explanation.Covered != 1 || math.Abs(row.Explanation.Adjustment-4) > 1e-12 {
		t.Fatalf("unit evidence = %+v, trace = %+v", got, row.Explanation)
	}
	for _, support := range row.Support {
		if support.Line == row.Line || support.Line == 4 || len(support.Terms) != 1 {
			t.Fatalf("unexpected support = %+v", support)
		}
	}
}

func TestCategoryRankingPreservesFocusedImports(t *testing.T) {
	o := &Outline{Language: "go", File: "sample.go", Nodes: []*Node{
		{Line: 1, Text: "import (", Children: []*Node{{Line: 2, Text: "\"example/tool\""}}},
		{Line: 4, Text: "tool installation()"},
	}}
	opts := retrievalOptions{rankMode: "category", explainRanking: true}
	for _, example := range []struct {
		terms  []string
		change bool
	}{
		{[]string{"tool"}, false},
		{[]string{"example", "tool"}, false},
		{[]string{"tool", "installation"}, true},
	} {
		for _, row := range opts.retrieve([]*Outline{o}, example.terms, 5, false, false, true) {
			if row.Line == 2 && (row.Explanation.Adjustment < 0) != example.change {
				t.Fatalf("category evidence for %v = %+v", example.terms, row.Explanation)
			}
		}
	}
}
