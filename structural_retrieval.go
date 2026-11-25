package main

import (
	"regexp"
	"strings"
)

type structuralUnit struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

const unitIdentifier = `[\p{L}_$][\p{L}\p{N}_$]*`

var (
	goFunction = regexp.MustCompile(`^func\s+(?:\(\s*(?:` + unitIdentifier + `\s+)?(\*?` + unitIdentifier + `)(?:\[[^\]]*\])?\s*\)\s*)?(` + unitIdentifier + `)\s*[\[(]`)
	goBlock    = regexp.MustCompile(`^(import|const|var|type)\s+\(`)
	goType     = regexp.MustCompile(`^type\s+(` + unitIdentifier + `)(?:\[[^\]]*\])?\s+(?:struct|interface)\s*\{`)
	namedUnits = map[string]*regexp.Regexp{
		"python":     regexp.MustCompile(`^(?:async\s+)?(def|class)\s+(` + unitIdentifier + `)(?:[\s\[(:]|$)`),
		"javascript": regexp.MustCompile(`^(?:(?:export|default|async)\s+)*(function|class)\s*\*?\s+(` + unitIdentifier + `)`),
		"powershell": regexp.MustCompile(`(?i)^(function|filter)\s+([` + "-:" + `\p{L}\p{N}_]+)`),
		"rust":       regexp.MustCompile(`^(?:(?:pub(?:\([^)]*\))?|async|unsafe|const)\s+)*(fn|struct|enum|trait)\s+(` + unitIdentifier + `)`),
	}
	jsMethod = regexp.MustCompile(`^(?:(?:public|protected|private|static|async|get|set|override)\s+)*([#]?` + unitIdentifier + `)\s*(?:<[^>]*>)?\s*\(`)
)

func enclosingUnit(language string, node *Node, end int, parent *structuralUnit, parentLine int) *structuralUnit {
	text := strings.TrimSpace(node.Text)
	var kind, name string
	switch language {
	case "go":
		if match := goFunction.FindStringSubmatch(text); match != nil {
			kind, name = "function", match[2]
			if match[1] != "" {
				kind, name = "method", "("+match[1]+")."+name
			}
		} else if match := goType.FindStringSubmatch(text); match != nil {
			kind, name = "type", match[1]
		} else if match := goBlock.FindStringSubmatch(text); match != nil {
			kind, name = "declarations", match[1]
		}
	default:
		if language == "typescript" {
			language = "javascript"
		}
		if pattern := namedUnits[language]; pattern != nil {
			if match := pattern.FindStringSubmatch(text); match != nil {
				kind, name = strings.ToLower(match[1]), match[2]
			}
		}
		if kind == "" && language == "javascript" && parent != nil && parent.Kind == "class" && parent.Start == parentLine &&
			(len(node.Children) > 0 || strings.Contains(text, "{")) {
			if match := jsMethod.FindStringSubmatch(text); match != nil {
				kind, name = "method", match[1]
			}
		}
		if name != "" && parent != nil {
			name = parent.Name + "." + name
		}
	}
	if name == "" {
		return parent
	}
	return &structuralUnit{Kind: kind, Name: name, Start: node.Line, End: end}
}

// Matches arrive in rank order. Keeping the best line leaves the group's
// score independent of the number of matching lines.
func groupStructuralMatches(results []retrievalResult) []retrievalResult {
	type location struct {
		file string
		line int
	}
	seen := make(map[location]bool)
	kept := results[:0]
	for _, result := range results {
		if result.Unit != nil {
			key := location{result.File, result.Unit.Start}
			if seen[key] {
				continue
			}
			seen[key] = true
			result.Parents = nil
		}
		kept = append(kept, result)
	}
	return kept
}
