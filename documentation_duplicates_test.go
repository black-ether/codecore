package main

import (
	"strings"
	"testing"
)

func TestMarkdownPassages(t *testing.T) {
	src := strings.Join([]string{
		"---",
		"identical front matter is excluded in each copy",
		"---",
		"",
		"# Identical headings are excluded in each copy",
		"",
		"A long line starts a searchable paragraph.",
		"This second line has a separate offset.",
		"",
		"````go",
		"identical quoted code is excluded in each copy",
		"```",
		"this is still inside the longer fence",
		"````",
		"",
		"    identical indented code is excluded in each copy",
		"",
		"Short reused fragment.",
		"",
		"The passage after the fence is also searchable.",
	}, "\r\n")
	keys := markdownPassages([]byte(src))
	if len(keys) != 3 || keys[7].text != keys[8].text || keys[7].offset != 0 ||
		keys[8].offset != 1 || keys[20].text == "" {
		t.Fatalf("passage keys = %+v", keys)
	}
}

func TestDuplicateDocsRetrieval(t *testing.T) {
	p := "A checksum example searches this shared passage.\nSecond line is distinct evidence within the same passage.\n"
	different := strings.Replace(p, "Second line", "A different line", 1)
	sources := map[string]string{
		"a.md": p + "\n" + p,
		"b.md": p,
		"c.md": different,
		"a.go": "const checksum = 7",
		"b.go": "const checksum = 7",
	}
	var outlines []*Outline
	opts := retrievalOptions{docPassages: make(map[string]map[int]passageLine)}
	for file, source := range sources {
		ext, _ := ExtractorFor(file)
		outline, err := ext.Extract(file, []byte(source))
		if err != nil {
			t.Fatal(err)
		}
		outlines = append(outlines, outline)
		if isMarkdownFile(file) {
			opts.docPassages[file] = markdownPassages([]byte(source))
		}
	}
	got := opts.retrieve(outlines, []string{"checksum"}, 20, false, false, true)
	observed := make(map[string]int)
	for _, result := range got {
		observed[result.File]++
	}
	if len(got) != 5 || observed["a.md"] != 2 || observed["b.md"] != 0 ||
		observed["c.md"] != 1 || observed["a.go"] != 1 || observed["b.go"] != 1 {
		t.Fatalf("results = %+v", got)
	}
}
