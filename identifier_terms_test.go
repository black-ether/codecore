package main

import (
	"reflect"
	"testing"
)

func TestIdentifierTerms(t *testing.T) {
	terms, whole := identifierTerms("readFile ParseHTTP2Response read_read caféÉté readRead read")
	wantWhole := []string{"readfile", "parsehttp2response", "read_read", "caféété", "readread", "read"}
	wantTerms := []string{"readfile", "read", "file", "parsehttp2response", "parse", "http", "2", "response",
		"read_read", "read", "caféété", "café", "été", "readread", "read", "read"}
	if !reflect.DeepEqual(whole, wantWhole) || !reflect.DeepEqual(terms, wantTerms) {
		t.Fatalf("identifier terms = %q, originals = %q", terms, whole)
	}
}

func TestIdentifierRanking(t *testing.T) {
	outlines := []*Outline{{File: "example.go", Nodes: []*Node{
		{Line: 1, Text: "func readXML2File() {"},
		{Line: 2, Text: "func read_XML2_file() {"},
		{Line: 3, Text: "func readFile() {"},
		{Line: 4, Text: "func read_read_file() {"},
		{Line: 5, Text: "func READ_read_file() {"},
	}}}
	opts := retrievalOptions{splitIdentifiers: true}
	for query, want := range map[string]int{"readXML2File": 1, "read_XML2_file": 2, "readFile": 3, "read_read_file": 4} {
		got := opts.retrieve(outlines, wholeIdentifierTerms(query), 5, false, false, true)
		if len(got) == 0 || got[0].Line != want {
			t.Errorf("%s = %+v, want line %d", query, got, want)
		}
	}
	got := opts.retrieve(outlines, []string{"xml"}, 5, false, false, true)
	if len(got) != 2 || got[0].Score != got[1].Score {
		t.Fatalf("xml = %+v, want equivalent component matches", got)
	}
	got = opts.retrieve(outlines, []string{"read"}, 5, false, false, true)
	if len(got) != 5 || got[0].Score != got[4].Score {
		t.Fatalf("read = %+v, want no length or frequency increase from extra aliases", got)
	}
}
