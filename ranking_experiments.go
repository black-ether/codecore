package main

import (
	"slices"
	"strings"
)

type rankingEvidence struct {
	Line  int      `json:"line"`
	Text  string   `json:"text"`
	Terms []string `json:"terms"`
}

func uniqueRankingTerms(terms []string) []string {
	seen := make(map[string]bool, len(terms))
	distinct := make([]string, 0, len(terms))
	for _, term := range terms {
		if !seen[term] {
			seen[term] = true
			distinct = append(distinct, term)
		}
	}
	return distinct
}

func (opts retrievalOptions) contextMatches(unit *structuralUnit, header *Node, terms []string, wordOnly bool) []int {
	text := unit.Name + " " + strings.Join(header.Comments, " ")
	var tokens []string
	if opts.splitIdentifiers {
		tokens, _ = identifierTerms(text)
	} else {
		tokens = tokenize(text)
	}
	var matches []int
	for i, term := range terms {
		if slices.ContainsFunc(tokens, func(token string) bool {
			return token == term || (!wordOnly && len(term) >= 3 && strings.HasPrefix(token, term))
		}) {
			matches = append(matches, i)
		}
	}
	return matches
}

func isImportEvidence(language, text string, unit *structuralUnit) bool {
	if unit != nil && unit.Kind == "declarations" && unit.Name == "import" {
		return true
	}
	text = strings.TrimSpace(text)
	switch language {
	case "go", "javascript", "typescript":
		return strings.HasPrefix(text, "import ")
	case "python":
		return strings.HasPrefix(text, "import ") || (strings.HasPrefix(text, "from ") && strings.Contains(text, " import "))
	case "powershell":
		return strings.HasPrefix(strings.ToLower(text), "import-module ")
	}
	return false
}

// Both context sources can add at most two legacy whole-word bonuses.
// The bonus rewards new query terms and depends neither on their
// frequency nor on the number of lines in the scope.
const contextBudget = 6.0

func (opts retrievalOptions) applyRankingExperiment(results []retrievalResult, terms []string, contexts map[*structuralUnit][]int) []retrievalResult {
	switch opts.rankMode {
	case "coverage":
		for i := range results {
			row := &results[i]
			multiplier := 1 + float64(len(row.matchedTerms)-1)/float64(max(1, len(terms)-1))
			row.adjustRanking(row.Score*(multiplier-1), nil)
		}
	case "context":
		for i := range results {
			row := &results[i]
			extra := additionalRankingTerms(terms, contexts[row.Unit], row.matchedTerms)
			row.adjustRanking(contextBudget*float64(len(extra))/float64(len(terms)), extra)
		}
	case "unit":
		return rankCompactUnits(results, terms)
	case "category":
		broader := slices.ContainsFunc(results, func(row retrievalResult) bool {
			return !row.importEvidence && len(row.matchedTerms) >= 2
		})
		if broader {
			for i := range results {
				row := &results[i]
				if row.importEvidence && len(row.matchedTerms) == 1 {
					row.adjustRanking(-0.35*row.Score, nil)
				}
			}
		}
	}
	if !opts.groupStructure {
		for i := range results {
			results[i].Unit = nil
		}
	}
	return results
}

func (row *retrievalResult) adjustRanking(delta float64, extra []string) {
	row.Score += delta
	if row.Explanation != nil {
		row.Explanation.Adjustment = delta
		row.Explanation.SupportingTerms = extra
	}
}

func additionalRankingTerms(terms []string, candidates, seen []int) []string {
	var extra []string
	for _, term := range candidates {
		if !slices.Contains(seen, term) {
			extra = append(extra, terms[term])
		}
	}
	return extra
}

func rankCompactUnits(results []retrievalResult, terms []string) []retrievalResult {
	const maxSupportingRows = 3
	positions := make(map[*structuralUnit]int)
	kept := results[:0]
	for _, row := range results {
		if row.Unit == nil {
			kept = append(kept, row)
			continue
		}
		index, exists := positions[row.Unit]
		if !exists {
			positions[row.Unit] = len(kept)
			row.Parents = nil
			kept = append(kept, row)
			continue
		}
		representative := &kept[index]
		if len(representative.Support) == maxSupportingRows {
			continue
		}
		extra := additionalRankingTerms(terms, row.matchedTerms, representative.matchedTerms)
		if len(extra) == 0 {
			continue
		}
		representative.Support = append(representative.Support, rankingEvidence{row.Line, row.Text, extra})
		for _, term := range row.matchedTerms {
			if !slices.Contains(representative.matchedTerms, term) {
				representative.matchedTerms = append(representative.matchedTerms, term)
			}
		}
	}
	for i := range kept {
		row := &kept[i]
		var extra []string
		for _, support := range row.Support {
			extra = append(extra, support.Terms...)
		}
		row.adjustRanking(contextBudget*float64(len(extra))/float64(len(terms)), extra)
	}
	return kept
}
