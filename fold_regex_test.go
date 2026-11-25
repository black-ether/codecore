package main

import (
	"strings"
	"testing"
)

func TestJavaScriptSlashContexts(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{"escaped closing slash", `(/\//);`},
		{"opaque pattern and flags", `const re = /[(){}[\]/*]/dgi; const other = /[//]/u;`},
		{"escaped character class closer", `const re = /[\]}/]/g; const escaped = /\\\//;`},
		{"division and chained division", `const total = value / (divisor / scale) / factor; const n = i++ / (scale / 2);`},
		{"division after operands", `const a = x[0] / (y / z); const b = x.return() / (y / z); const c = ({}) / (y / z);`},
		{"division across lines", "const total = value\n  / (divisor / scale);"},
		{"division after a regex or template", "const a = /[}]/g / (y / z); const b = `text` / (y / z);"},
		{"regex after division operator", `const total = value / /[}]/.test(text);`},
		{"line and block comments", "const a = /* )} [ / */ /[}]/; // [ } (\n/* [ /\n} (\n*/"},
		{"assignment and return", `const a = /[}]/; function make() { return /[)]/; }`},
		{"control conditions", `if (ready(value)) /^\/docs\/reference\//.test(path); while (ready()) /[}]/.test(path);`},
		{"multiline control condition", "if /* comment */ (\n  ready()\n)\n  /[)]/.test(text);"},
		{"template interpolation", "const a = `before ${/[}/*]/u.test(text)} after ${`inner ${/\\//.test(path)}`}`;"},
	}
	for _, file := range []string{"example.js", "example.ts"} {
		t.Run(file, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					next := strings.Count(tc.source, "\n") + 2
					outline := mustExtractSource(t, file, tc.source+"\nfunction next() {\n  return 1;\n}\n")
					if outline.Unbalanced || len(outline.Nodes) == 0 {
						t.Fatalf("want a balanced outline, got %+v", outline)
					}
					node := outline.Nodes[len(outline.Nodes)-1]
					if node.Line != next || node.SourceEnd != next+2 {
						t.Fatalf("last top-level node = %+v, want next function at lines %d–%d", node, next, next+2)
					}
				})
			}
		})
	}
}

func TestJavaScriptRegexDoesNotHideOpenBlock(t *testing.T) {
	outline := mustExtractSource(t, "example.js", `function open() { return /[}]/;`)
	if !outline.Unbalanced || len(outline.Nodes) != 1 || outline.Nodes[0].SourceEnd != -1 {
		t.Fatalf("want the unclosed block to remain visible, got %+v", outline)
	}
}
