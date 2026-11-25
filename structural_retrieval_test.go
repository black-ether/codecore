package main

import (
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
		{Kind: "method", Name: "(*Alpha).Find", Start: 2, End: 5},
		{Kind: "method", Name: "(*Beta).Find", Start: 8, End: 9},
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
	class := enclosingUnit("python", &Node{Line: 1, Text: "class Reader:"}, 10, nil, 0)
	method := enclosingUnit("python", &Node{Line: 3, Text: "async def read(self):"}, 9, class, 1)
	nested := enclosingUnit("python", &Node{Line: 4, Text: "def fetch():"}, 6, method, 3)
	if nested == nil || nested.Name != "Reader.read.fetch" || nested.Start != 4 || nested.End != 6 {
		t.Fatalf("nested unit = %+v", nested)
	}
	statement := enclosingUnit("python", &Node{Line: 5, Text: "return response"}, 5, nested, 4)
	if statement != nested {
		t.Fatalf("statement attributed to %+v, want %+v", statement, nested)
	}
	jsClass := enclosingUnit("typescript", &Node{Line: 1, Text: "export class Reader {"}, 10, nil, 0)
	jsMethod := enclosingUnit("typescript", &Node{Line: 2, Text: "private async read() {"}, 4, jsClass, 1)
	if jsMethod == nil || jsMethod.Name != "Reader.read" {
		t.Fatalf("TypeScript method = %+v", jsMethod)
	}
}
