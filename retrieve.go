package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"unicode"
)

type retrievalResult struct {
	Rank    int      `json:"rank"`
	File    string   `json:"file"`
	Line    int      `json:"line"`
	Text    string   `json:"text"`
	Score   float64  `json:"score"`
	Parents []string `json:"parents,omitempty"`
}

func tokenize(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

type retrievalDocument struct {
	file    string
	node    *Node
	length  int
	parents []string
}

type termOccurrence struct {
	document  int
	frequency int
}

func retrieve(outlines []*Outline, terms []string, top int) []retrievalResult {
	var documents []retrievalDocument
	postings := make(map[string][]termOccurrence)
	totalLength := 0
	var walk func(string, []*Node, []string)
	walk = func(file string, nodes []*Node, parents []string) {
		for _, node := range nodes {
			tokens := tokenize(node.Text)
			id := len(documents)
			documents = append(documents, retrievalDocument{file, node, len(tokens), parents})
			totalLength += len(tokens)
			frequencies := make(map[string]int)
			for _, token := range tokens {
				frequencies[token]++
			}
			for term, frequency := range frequencies {
				postings[term] = append(postings[term], termOccurrence{id, frequency})
			}
			if len(node.Children) > 0 {
				// Give each branch its own path so siblings cannot overwrite it.
				walk(file, node.Children, append(parents[:len(parents):len(parents)], strings.TrimSpace(node.Text)))
			}
		}
	}
	for _, outline := range outlines {
		walk(outline.File, outline.Nodes, nil)
	}

	results := make([]retrievalResult, 0)
	if totalLength == 0 {
		return results
	}
	const k1, b = 1.2, 0.75
	n := float64(len(documents))
	avgdl := float64(totalLength) / n
	scores := make([]float64, len(documents))
	// Add scores in query order to keep floating-point sums deterministic.
	for _, term := range terms {
		matches := postings[term]
		if len(matches) == 0 {
			continue
		}
		df := float64(len(matches))
		idf := math.Log(1 + (n-df+0.5)/(df+0.5))
		for _, match := range matches {
			tf := float64(match.frequency)
			dl := float64(documents[match.document].length)
			scores[match.document] += idf * (tf * (k1 + 1)) / (tf + k1*(1-b+b*dl/avgdl))
		}
	}
	for i, doc := range documents {
		if scores[i] > 0 {
			results = append(results, retrievalResult{
				File: doc.file, Line: doc.node.Line, Text: doc.node.Text,
				Score: scores[i], Parents: doc.parents,
			})
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		a, b := results[i], results[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	if len(results) > top {
		results = results[:top]
	}
	for i := range results {
		results[i].Rank = i + 1
	}
	return results
}

func formatResults(w io.Writer, format, query string, results []retrievalResult) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}
	if len(results) == 0 {
		_, err := fmt.Fprintf(w, "no matches for %q\n", query)
		return err
	}
	for _, result := range results {
		context := ""
		if len(result.Parents) > 0 {
			context = " · in " + strings.Join(result.Parents, " > ")
		}
		if _, err := fmt.Fprintf(w, "  %d. %s [L:%d] %s\n     score %.2f%s\n",
			result.Rank, result.File, result.Line, result.Text, result.Score, context); err != nil {
			return err
		}
	}
	return nil
}
