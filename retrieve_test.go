package main

import (
	"slices"
	"testing"
)

func TestTokenize(t *testing.T) {
	got := tokenize(" __authenticate_user--HTTP.2(ÉTÉ)\t")
	want := []string{"authenticate", "user", "http", "2", "été"}
	if !slices.Equal(got, want) {
		t.Errorf("tokens = %q, want %q", got, want)
	}
	if got := tokenize(" _-(). "); len(got) != 0 {
		t.Errorf("separator-only tokens = %q, want none", got)
	}
}

func TestBM25Scoring(t *testing.T) {
	outlines := []*Outline{{File: "terms.go", Nodes: []*Node{
		{Line: 1, Text: "common"},
		{Line: 2, Text: "rare"},
		{Line: 3, Text: "common longer document"},
		{Line: 4, Text: "common"},
	}}}
	got := retrieve(outlines, tokenize("rare common"), 5)
	if len(got) != 4 {
		t.Fatalf("got %d results, want 4", len(got))
	}
	scores := make(map[int]float64)
	for _, result := range got {
		scores[result.Line] = result.Score
	}
	if scores[2] <= scores[1] {
		t.Errorf("rare score %g should exceed common score %g", scores[2], scores[1])
	}
	if scores[1] <= scores[3] {
		t.Errorf("short score %g should exceed long score %g", scores[1], scores[3])
	}
	if scores[1] != scores[4] {
		t.Errorf("identical documents should tie, got %g and %g", scores[1], scores[4])
	}
}

func TestRetrieveRanking(t *testing.T) {
	outlines := []*Outline{
		{File: "z.go", Nodes: []*Node{{Line: 1, Text: "find"}}},
		{File: "a.go", Nodes: []*Node{
			{Line: 8, Text: "find"},
			{Line: 4, Text: "find"},
			{Line: 9, Text: "find find"},
		}},
	}
	want := []struct {
		file string
		line int
	}{{"a.go", 9}, {"a.go", 4}, {"a.go", 8}, {"z.go", 1}}
	for _, top := range []int{2, 5} {
		got := retrieve(outlines, tokenize("find missing"), top)
		if len(got) != min(top, len(want)) {
			t.Fatalf("top %d returned %d results, want %d", top, len(got), min(top, len(want)))
		}
		for i, result := range got {
			if result.Rank != i+1 || result.File != want[i].file || result.Line != want[i].line {
				t.Errorf("top %d result %d = %+v, want rank %d at %+v", top, i, result, i+1, want[i])
			}
		}
	}
	if got := retrieve(outlines, tokenize("missing"), 5); len(got) != 0 {
		t.Errorf("unmatched query returned %+v", got)
	}
}

func TestRetrieveFromExtractor(t *testing.T) {
	ext, ok := ExtractorByLanguage("python")
	if !ok {
		t.Fatal("no python extractor")
	}
	src := "class Manager:\n    def check(self):\n        return authenticate_user()\n"
	outline, err := ext.Extract("auth.py", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	outlines := []*Outline{outline}
	got := retrieve(outlines, tokenize("authenticate user"), 5)
	if len(got) != 1 {
		t.Fatalf("got %+v, want one match", got)
	}
	result := got[0]
	if result.File != "auth.py" || result.Line != 3 || result.Text != "return authenticate_user()" {
		t.Errorf("top result = %+v, want auth.py line 3 with the authenticate_user call", result)
	}
	if want := []string{"class Manager:", "def check(self):"}; !slices.Equal(result.Parents, want) {
		t.Errorf("parents = %q, want %q", result.Parents, want)
	}

	got = retrieve(outlines, tokenize("manager"), 5)
	if len(got) != 1 || len(got[0].Parents) != 0 {
		t.Errorf("manager query returned %+v, want one top-level match", got)
	}
}
