package main

import (
	"strings"
	"testing"
)

func TestFoldText(t *testing.T) {
	// Pure indentation folding, no language rules: a markdown
	// heading, a TOML section header, and a config key are all
	// just lines, nested by their indent.
	src := "# Title\n\n## Section\n  key = value\n  nested = true\n\n## Other\nplain\n"
	ext, ok := ExtractorByLanguage("text")
	if !ok {
		t.Fatal("no text extractor")
	}
	outline, err := ext.Extract("notes.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if outline.Language != "text" {
		t.Errorf("language = %q, want text", outline.Language)
	}
	// Top level: the two ## headings and the two # heading lines
	// that are not indented. "# Title" is a top-level node (headings
	// are not skipped as comments), "## Section" and "## Other" are
	// top-level too, and the indented keys nest under "## Section".
	if len(outline.Nodes) != 4 {
		t.Fatalf("top-level nodes = %d, want 4: %+v", len(outline.Nodes), outline.Nodes)
	}
	if outline.Nodes[0].Text != "# Title" || outline.Nodes[1].Text != "## Section" {
		t.Errorf("top-level = %q, %q, want # Title, ## Section", outline.Nodes[0].Text, outline.Nodes[1].Text)
	}
	section := outline.Nodes[1]
	if len(section.Children) != 2 || section.Children[0].Text != "key = value" || section.Children[1].Text != "nested = true" {
		t.Errorf("section children = %+v, want key = value, nested = true", section.Children)
	}
	// Blank lines produce no nodes.
	if outline.Nodes[3].Text != "plain" {
		t.Errorf("last node = %q, want plain", outline.Nodes[3].Text)
	}
}

func TestFoldTextRegistered(t *testing.T) {
	// The common text extensions route to the text parser, so they
	// are included in directory walks.
	for _, ext := range []string{".md", ".txt", ".toml", ".ini", ".log", ".rst"} {
		e, ok := ExtractorFor("file" + ext)
		if !ok || e.Language() != "text" {
			t.Errorf("ExtractorFor(file%s) = %v, %v, want text parser", ext, e, ok)
		}
	}
	// An unregistered extension has no extractor — the fallback for
	// a directly passed file lives in main.go, not in the registry,
	// so a directory walk still skips it.
	if _, ok := ExtractorFor("file.xyz"); ok {
		t.Errorf("ExtractorFor(file.xyz) returned an extractor, want none")
	}
}

func TestFoldTextRejectsBinary(t *testing.T) {
	ext, ok := ExtractorByLanguage("text")
	if !ok {
		t.Fatal("no text extractor")
	}
	src := []byte("hello\x00world\n")
	if _, err := ext.Extract("bin.dat", src); err == nil {
		t.Errorf("binary input was accepted, want an error")
	} else if !strings.Contains(err.Error(), "not a text file") {
		t.Errorf("error = %q, want a not-a-text-file error", err.Error())
	}
}

func TestFoldTextAsk(t *testing.T) {
	// --ask works on a plain-text file: the heading and the key are
	// outline nodes, so they are searchable.
	ext, ok := ExtractorByLanguage("text")
	if !ok {
		t.Fatal("no text extractor")
	}
	src := "# Deployment\n\n## Database\n  host = db.example.com\n  port = 5432\n"
	outline, err := ext.Extract("notes.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	got := (retrievalOptions{}).retrieve([]*Outline{outline}, tokenize("database"), 5, false, false, false)
	if len(got) != 1 || got[0].Text != "## Database" {
		t.Fatalf("database query = %+v, want the Database heading", got)
	}
	// The "host = ..." line is an assignment, so it carries a name
	// ("host") and ranks first for its own term.
	got = (retrievalOptions{}).retrieve([]*Outline{outline}, tokenize("host"), 5, false, false, false)
	if len(got) != 1 || got[0].Text != "host = db.example.com" {
		t.Fatalf("host query = %+v, want the host line", got)
	}
}
