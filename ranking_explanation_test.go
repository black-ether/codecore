package main

import (
	"math"
	"testing"
)

func TestRankingExplanation(t *testing.T) {
	outlines := []*Outline{{File: "sample.go", Nodes: []*Node{
		{Line: 1, Text: "func reader()"},
		{Line: 2, Text: "read()"},
	}}}
	terms := []string{"read", "read", "reader"}
	original := (retrievalOptions{}).retrieve(outlines, terms, 5, false, false, false)
	explained := (retrievalOptions{explainRanking: true}).retrieve(outlines, terms, 5, false, false, false)
	for i, row := range explained {
		trace := row.Explanation
		if row.Score != original[i].Score || row.Line != original[i].Line || trace.Documents != 2 || trace.MeanLength != 1.5 {
			t.Fatalf("trace changed ranking or corpus stats: %+v %+v", row, trace)
		}
		var total float64
		for _, term := range trace.Terms {
			total += term.BM25 + term.Bonus
		}
		if math.Abs(total-row.Score) > 1e-12 {
			t.Fatalf("contributions = %v, score = %v", total, row.Score)
		}
		if row.Line == 1 && (trace.Covered != 2 || trace.Terms[0].TF != 3 || trace.Terms[0].Bonus != 0 || trace.Terms[2].Bonus != 3) {
			t.Fatalf("unexpected prefix or boost trace: %+v", trace)
		}
	}
}
