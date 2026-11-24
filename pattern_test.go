package main

import (
	"slices"
	"strings"
	"testing"
)

func TestTokenizeCode(t *testing.T) {
	src := "α_2(0x1F, 3.14) == \"a\\\"b\" // hidden\n" +
		"`raw\nvalue` /* hidden\nmore */ # hidden\n" +
		"'''one\ntwo''' -- hidden\n\"\"\"three\"\"\" . ... =/* hidden */name 'x' 1e10"
	want := []token{
		{tokIdent, "α_2", 1}, {tokBracket, "(", 1}, {tokNumber, "0x1F", 1},
		{tokOperator, ",", 1}, {tokNumber, "3.14", 1}, {tokBracket, ")", 1},
		{tokOperator, "==", 1}, {tokString, "\"a\\\"b\"", 1},
		{tokString, "`raw\nvalue`", 2}, {tokString, "'''one\ntwo'''", 5},
		{tokString, "\"\"\"three\"\"\"", 7}, {tokOperator, ".", 7},
		{tokOperator, ".", 7}, {tokOperator, ".", 7}, {tokOperator, ".", 7},
		{tokOperator, "=", 7}, {tokIdent, "name", 7},
		{tokString, "'x'", 7}, {tokNumber, "1e10", 7},
	}
	if got := tokenizeCode([]byte(src)); !slices.Equal(got, want) {
		t.Fatalf("tokens = %#v, want %#v", got, want)
	}
}

func mustParsePattern(t *testing.T, pat string) []atom {
	t.Helper()
	atoms, err := parsePattern(pat)
	if err != nil {
		t.Fatal(err)
	}
	return atoms
}

func TestParsePattern(t *testing.T) {
	pat := "... ... ANY KEY TYP OP NUM OCT HEX STR LIT _ _1 NOT STR A OR B OR C _ANY _NOT _OR __1 __ _other"
	atoms := mustParsePattern(t, pat)
	want := []atomKind{atomEllipsis, atomAny, atomKeyword, atomNativeType,
		atomOperator, atomNumber, atomOct, atomHex, atomString, atomLiteral,
		atomPlaceholder, atomPlaceholder, atomNot, atomOr,
		atomExact, atomExact, atomExact, atomExact, atomExact, atomExact}
	var kinds []atomKind
	for _, a := range atoms {
		kinds = append(kinds, a.kind)
	}
	if !slices.Equal(kinds, want) {
		t.Fatalf("atom kinds = %v, want %v", kinds, want)
	}
	if atoms[10].text != "" || atoms[11].text != "1" || atoms[12].inner.kind != atomString {
		t.Fatalf("unexpected placeholders or NOT: %#v", atoms[10:13])
	}
	or := atoms[13]
	if or.left.text != "A" || or.right.kind != atomOr || or.right.left.text != "B" || or.right.right.text != "C" {
		t.Fatalf("OR is not right-associative: %#v", or)
	}
	for i, text := range []string{"ANY", "NOT", "OR", "_1", "_", "_other"} {
		a := atoms[14+i]
		if a.text != text || a.tok.text != text {
			t.Errorf("escaped atom = %#v, want %q", a, text)
		}
	}

	for _, pat := range []string{"", "  ", "// comment"} {
		if _, err := parsePattern(pat); err == nil {
			t.Errorf("parsePattern(%q) succeeded", pat)
		}
	}
	for _, pat := range []string{"NOT", "NOT ...", "OR A", "A OR", "... OR B", "A OR ..."} {
		atoms := mustParsePattern(t, pat)
		if !slices.ContainsFunc(atoms, func(a atom) bool {
			return a.kind == atomExact && (a.text == "NOT" || a.text == "OR")
		}) {
			t.Errorf("parsePattern(%q) lost its literal combinator", pat)
		}
	}
}

func TestMatchAtoms(t *testing.T) {
	for _, tc := range []struct {
		pat, src string
		ok       bool
	}{
		{"foo = 3", "foo = 3", true},
		{"foo", "Foo", false},
		{"ANY ANY ANY ANY ANY", "a 'b' 1 + (", true},
		{"_", "name", true},
		{"_", "123", false},
		{"_1 = _1", "x = x", true},
		{"_1 = _1", "x = y", false},
		{"NOT STR", "123", true},
		{"NOT STR", "'word'", false},
		{"NOT NOT _1 _1", "x y", true},
		{"_1 NOT _1", "x y", true},
		{"_1 NOT _1", "x x", false},
		{"A OR B", "A", true},
		{"A OR B", "B", true},
		{"A OR B", "C", false},
		{"_1 OR ANY _1", "x y", false},
		{"NOT _1 OR _ YES _1", "x YES y", true},
		{"KEY", "func", false},
		{"TYP", "int", false},
		{"OCT HEX LIT OP NUM", "078 0Xff 'hi' += 42", true},
		{"OCT", "0x12", false},
		{"' hi '", "`	hi`", true},
		{"'hi'", "\"\"\"hi\"\"\"", true},
		{"'hi'", "hi", false},
	} {
		t.Run(tc.pat+"/"+tc.src, func(t *testing.T) {
			_, ok := matchAtoms(mustParsePattern(t, tc.pat), tokenizeCode([]byte(tc.src)), 0, nil)
			if ok != tc.ok {
				t.Fatalf("matched = %v, want %v", ok, tc.ok)
			}
		})
	}
}

func TestMatchEllipsis(t *testing.T) {
	for _, tc := range []struct {
		pat, src string
		end      int // -1 means no match
	}{
		{"f ( ... )", "f(a, b)", 6},
		{"f ( ... )", "f(g(x), y)", 9},
		{"f ( ... )", "f(a); g(b)", 4},
		{"f ( ... )", "f(a))", 4},
		{"f ( ... ) mark", "f(a); g(b) mark", -1},
		{"f ( ... ) mark", "f(a)) mark", -1},
		{"f ( ... )", "f(a; b)", -1},
		{"f ( ... )", "f(g(a; b))", 9},
		{"f ...", "f", 1},
		{"f ...", "f x", 1},
		{"f ... _1 = _1", "f x = y z = z", 7},
		{"f ... x", "f " + strings.Repeat("a ", 64) + "x", 66},
		{"f ... x", "f " + strings.Repeat("a ", 65) + "x", -1},
	} {
		t.Run(tc.pat+"/"+tc.src, func(t *testing.T) {
			end, ok := matchAtoms(mustParsePattern(t, tc.pat), tokenizeCode([]byte(tc.src)), 0, nil)
			if ok != (tc.end >= 0) || ok && end != tc.end {
				t.Fatalf("end, matched = %d, %v; want end %d (-1 for failure)", end, ok, tc.end)
			}
		})
	}
}

func TestFindPatternMatches(t *testing.T) {
	const file = "sample.txt"
	src := []byte("  x = x\nx = y\nz =\n  z\n// a = a\n")
	tokens := tokenizeCode(src)
	got := findPatternMatches(file, src, mustParsePattern(t, "_1 = _1"), tokens)
	want := []patternResult{{file, 1, 1, "  x = x"}, {file, 3, 4, "z =\n  z"}}
	if !slices.Equal(got, want) {
		t.Fatalf("matches = %#v, want %#v", got, want)
	}
	for _, pat := range []string{"missing", "..."} {
		if got := findPatternMatches(file, src, mustParsePattern(t, pat), tokens); len(got) != 0 {
			t.Errorf("pattern %q matched %#v", pat, got)
		}
	}
	src = []byte("x = x = x")
	if got := findPatternMatches(file, src, mustParsePattern(t, "_1 = _1"), tokenizeCode(src)); len(got) != 2 {
		t.Errorf("overlapping matches = %#v, want two", got)
	}
}

func TestPatternFlag(t *testing.T) {
	for _, args := range [][]string{
		{"--pattern", "ANY", "--ask", "sample", "testdata/sample.go"},
		{"--pattern", "ANY", "--ask=", "testdata/sample.go"},
		{"--pattern=", "testdata/sample.go"},
		{"--pattern", "  ", "testdata/sample.go"},
		{"--pattern", "// comment", "testdata/sample.go"},
	} {
		if got := run(args); got != 2 {
			t.Errorf("run(%q) = %d, want 2", args, got)
		}
	}
	if got := run([]string{"--pattern", "missing", "--top", "0", "--per-file", "--def-only", "testdata/sample.go"}); got != 0 {
		t.Errorf("run with --ask options = %d, want 0", got)
	}
}
