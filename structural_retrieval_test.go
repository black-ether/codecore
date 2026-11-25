package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestStructuralRetrieval(t *testing.T) {
	src := []byte("package sample\nfunc (x *Alpha[T]) Find() {\n if enabled {\n findNeedle()\n findNeedle()\n }\n}\nfunc (x *Beta) Find() {\n findNeedle()\n}\n")
	ext, _ := ExtractorFor("sample.go")
	outline, err := ext.Extract("sample.go", src)
	if err != nil {
		t.Fatal(err)
	}
	opts := retrievalOptions{groupStructure: true}
	terms := []string{"findneedle"}
	got := opts.retrieve([]*Outline{outline}, terms, 2, false, false, true)
	if len(got) != 2 || got[0].Line != 4 || got[1].Line != 9 {
		t.Fatalf("grouped evidence = %+v", got)
	}
	for i, want := range []structuralUnit{
		{Kind: "method", Name: "(*Alpha).Find", Start: 2, End: 7},
		{Kind: "method", Name: "(*Beta).Find", Start: 8, End: 10},
	} {
		if got[i].Unit == nil || *got[i].Unit != want || len(got[i].Parents) != 0 {
			t.Fatalf("result %d unit = %+v, want %+v", i, got[i].Unit, want)
		}
	}
	if got[0].Score != got[1].Score {
		t.Fatalf("repeated evidence changed the group score: %+v", got)
	}
}

func TestNestedStructuralUnits(t *testing.T) {
	class := enclosingUnit("python", &Node{Line: 1, Text: "class Reader:"}, nil, 0)
	method := enclosingUnit("python", &Node{Line: 3, Text: "async def read(self):"}, class, 1)
	nested := enclosingUnit("python", &Node{Line: 4, Text: "def fetch():", Children: []*Node{{Line: 6}}}, method, 3)
	if nested == nil || nested.Name != "Reader.read.fetch" || nested.Start != 4 || nested.End != 0 || nested.OutlineEnd != 6 {
		t.Fatalf("nested unit = %+v", nested)
	}
	statement := enclosingUnit("python", &Node{Line: 5, Text: "return response"}, nested, 4)
	if statement != nested {
		t.Fatalf("statement attributed to %+v, want %+v", statement, nested)
	}
	jsClass := enclosingUnit("typescript", &Node{Line: 1, Text: "export class Reader {"}, nil, 0)
	jsMethod := enclosingUnit("typescript", &Node{Line: 2, Text: "private async read() {"}, jsClass, 1)
	if jsMethod == nil || jsMethod.Name != "Reader.read" {
		t.Fatalf("TypeScript method = %+v", jsMethod)
	}
}

func TestGroupedMultilineGoDeclaration(t *testing.T) {
	src := []byte("package p\nfunc Compute(\n value int,\n) int {\n return value\n}\n")
	ext, _ := ExtractorFor("sample.go")
	outline, err := ext.Extract("sample.go", src)
	if err != nil {
		t.Fatal(err)
	}
	opts := retrievalOptions{groupStructure: true}
	terms := []string{"value"}
	// Both the parameter and return statement match. The return remains
	// outside the parameter-list unit in the existing outline tree.
	before := opts.retrieve([]*Outline{outline}, terms, 5, false, false, false)

	applyGoDeclarationRanges(outline, src)
	after := opts.retrieve([]*Outline{outline}, terms, 5, false, false, false)
	if len(after) != len(before) || len(after) != 2 {
		t.Fatalf("matches before = %+v, after = %+v", before, after)
	}
	for i, row := range after {
		if row.Line == 3 && (row.Unit == nil || row.Unit.Start != 2 || row.Unit.End != 6) {
			t.Errorf("parameter's enclosing range = %+v, want lines 2–6", row.Unit)
		}
		if row.Line == 5 && row.Unit != nil {
			t.Errorf("return moved to a different unit: %+v", row)
		}
		row.Unit = before[i].Unit
		if !reflect.DeepEqual(row, before[i]) {
			t.Errorf("ranking or evidence changed at rank %d: before %+v, after %+v", i+1, before[i], row)
		}
	}
}

func TestGroupedRangeLabels(t *testing.T) {
	for _, tc := range []struct {
		name, src, label string
		end, outlineEnd  int
		unknown          bool
	}{
		{"sample.ts", "function target() {\n return evidence();\n}\n", "[source L:1-3]", 3, 0, false},
		{"imports.go", "package sample\nimport (\n \"evidence\"\n)\n", "[source L:2-4]", 4, 0, false},
		{"sample.py", "def target():\n    # A comment\n    return evidence()\n", "[outline L:1-3]", 0, 3, false},
		{"broken.go", "func target() {\n evidence()\n", "[source L:1-?]", 0, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outline := mustExtractSource(t, tc.name, tc.src)
			rows := (retrievalOptions{groupStructure: true}).retrieve([]*Outline{outline}, []string{"evidence"}, 5, false, false, true)
			if len(rows) != 1 || rows[0].Unit == nil {
				t.Fatalf("want grouped evidence, got %+v", rows)
			}
			var output bytes.Buffer
			if err := formatResults(&output, "text", "evidence", rows); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), tc.label) || !strings.Contains(output.String(), "match L:") {
				t.Errorf("text output = %q, want %s and an evidence line", output.String(), tc.label)
			}
			output.Reset()
			if err := formatResults(&output, "json", "evidence", rows); err != nil {
				t.Fatal(err)
			}
			var decoded []struct {
				Unit map[string]any
			}
			if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
				t.Fatal(err)
			}
			unit := decoded[0].Unit
			for field, want := range map[string]int{"end": tc.end, "outlineEnd": tc.outlineEnd} {
				got, present := unit[field]
				if present != (want > 0) || (want > 0 && got != float64(want)) {
					t.Errorf("%s = %v, want %d (omitted when zero)", field, got, want)
				}
			}
			if flag, present := unit["sourceEndUnknown"]; present != tc.unknown || (present && flag != true) {
				t.Errorf("sourceEndUnknown = %v, want %v (omitted when false)", flag, tc.unknown)
			}
		})
	}
}
