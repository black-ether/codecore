package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestMeasureCorpusIndex emits descriptive measurements when explicitly run
// with the pinned corpus. The ordinary suite needs no external checkout.
func TestMeasureCorpusIndex(t *testing.T) {
	corpus := os.Getenv("CODECORE_EXPERIMENT_CORPUS")
	if corpus == "" {
		t.Skip("set CODECORE_EXPERIMENT_CORPUS to measure corpus token counts")
	}
	type frequencies struct {
		Legacy   int `json:"legacy"`
		Original int `json:"identifier_original"`
		Expanded int `json:"identifier_expanded"`
	}
	stats := struct {
		Files      int                     `json:"eligible_files"`
		Nodes      int                     `json:"documents"`
		Length     frequencies             `json:"tokens"`
		Categories map[string]int          `json:"file_categories"`
		Terms      map[string]*frequencies `json:"unboosted_exact_term_frequency"`
	}{Categories: make(map[string]int), Terms: make(map[string]*frequencies)}
	for _, term := range []string{"checksum", "include", "tasknamewithnamespace", "collectkeys", "up", "http", "read", "interrupt", "output", "lines"} {
		stats.Terms[term] = new(frequencies)
	}
	files, err := collectFiles([]string{corpus}, false)
	if err != nil {
		t.Fatal(err)
	}
	stats.Files = len(files)
	var walk func([]*Node)
	walk = func(nodes []*Node) {
		for _, node := range nodes {
			stats.Nodes++
			text := node.Text
			if len(node.Comments) > 0 {
				text += " " + strings.Join(node.Comments, " ")
			}
			legacy := tokenize(text)
			parts, originals := identifierTerms(text)
			stats.Length.Legacy += len(legacy)
			stats.Length.Original += len(originals)
			stats.Length.Expanded += len(parts)
			for _, term := range legacy {
				if count := stats.Terms[term]; count != nil {
					count.Legacy++
				}
			}
			for _, term := range originals {
				if count := stats.Terms[term]; count != nil {
					count.Original++
				}
			}
			for _, term := range parts {
				if count := stats.Terms[term]; count != nil {
					count.Expanded++
				}
			}
			walk(node.Children)
		}
	}
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		stats.Categories[sourceCategory(file, src)]++
		ext, _ := ExtractorFor(file)
		outline, err := ext.Extract(file, src)
		if err != nil {
			t.Fatal(err)
		}
		attachComments(outline, src)
		walk(outline.Nodes)
	}
	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if dest := os.Getenv("CODECORE_EXPERIMENT_INDEX_OUTPUT"); dest != "" {
		if err := os.WriteFile(dest, append(data, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Log(string(data))
	}
}
