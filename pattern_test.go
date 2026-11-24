package main

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestTokenizeCodePreprocessor(t *testing.T) {
	// A # at the start of a line followed by a preprocessor keyword
	// is code (C/C++), not a comment. A # anywhere else, or followed
	// by other text, is a comment (Python, Ruby, shell).
	src := "#include <stdio.h>\n  #define FOO 1\n# TODO note\nx = 1 # trailing\n"
	want := []token{
		{tokOperator, "#", 1}, {tokIdent, "include", 1},
		{tokOperator, "<", 1}, {tokIdent, "stdio", 1},
		{tokOperator, ".", 1}, {tokIdent, "h", 1},
		{tokOperator, ">", 1},
		{tokOperator, "#", 2}, {tokIdent, "define", 2},
		{tokIdent, "FOO", 2}, {tokNumber, "1", 2},
		{tokIdent, "x", 4}, {tokOperator, "=", 4}, {tokNumber, "1", 4},
	}
	if got := tokenizeCode([]byte(src), ""); !slices.Equal(got, want) {
		t.Fatalf("tokens = %#v, want %#v", got, want)
	}
}

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
	if got := tokenizeCode([]byte(src), ""); !slices.Equal(got, want) {
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
			_, ok := matchAtoms(mustParsePattern(t, tc.pat), tokenizeCode([]byte(tc.src), ""), 0, nil)
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
			end, ok := matchAtoms(mustParsePattern(t, tc.pat), tokenizeCode([]byte(tc.src), ""), 0, nil)
			if ok != (tc.end >= 0) || ok && end != tc.end {
				t.Fatalf("end, matched = %d, %v; want end %d (-1 for failure)", end, ok, tc.end)
			}
		})
	}
}

func TestFindPatternMatches(t *testing.T) {
	const file = "sample.txt"
	src := []byte("  x = x\nx = y\nz =\n  z\n// a = a\n")
	tokens := tokenizeCode(src, "")
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
	if got := findPatternMatches(file, src, mustParsePattern(t, "_1 = _1"), tokenizeCode(src, "")); len(got) != 2 {
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

func TestTokenizeCodeKeywords(t *testing.T) {
	const goSrc = "func main() { var x int = 0; if x == 0 { return } }"
	const pythonSrc = "def f(x): if x is None: return True"
	for _, tc := range []struct {
		lang, src string
		want      map[string]tokenKind
	}{
		{"GO", goSrc, map[string]tokenKind{
			"func": tokKeyword, "var": tokKeyword, "if": tokKeyword, "return": tokKeyword,
			"int": tokNativeType, "main": tokIdent, "x": tokIdent, "0": tokNumber,
		}},
		{"", goSrc, map[string]tokenKind{
			"func": tokIdent, "var": tokIdent, "if": tokIdent, "return": tokIdent,
			"int": tokIdent, "main": tokIdent, "x": tokIdent, "0": tokNumber,
		}},
		{"python", pythonSrc, map[string]tokenKind{
			"def": tokKeyword, "if": tokKeyword, "is": tokKeyword, "return": tokKeyword,
			"None": tokKeyword, "True": tokKeyword, "f": tokIdent, "x": tokIdent,
		}},
		{"python", "true print int", map[string]tokenKind{
			"true": tokIdent, "print": tokIdent, "int": tokNativeType,
		}},
		{"sql", "select * from t where x = 1", map[string]tokenKind{
			"select": tokKeyword, "from": tokKeyword, "where": tokKeyword,
			"t": tokIdent, "x": tokIdent, "1": tokNumber,
		}},
		{"sql", "Select * FrOm t", map[string]tokenKind{
			"Select": tokKeyword, "FrOm": tokKeyword, "t": tokIdent,
		}},
		{"json", "true null", map[string]tokenKind{"true": tokIdent, "null": tokIdent}},
		{"unknown", "if int", map[string]tokenKind{"if": tokIdent, "int": tokIdent}},
	} {
		got := make(map[string]tokenKind)
		for _, tok := range tokenizeCode([]byte(tc.src), tc.lang) {
			if tok.kind != tokOperator && tok.kind != tokBracket {
				got[tok.text] = tok.kind
			}
		}
		if !maps.Equal(got, tc.want) {
			t.Errorf("tokenizeCode(%q, %q) word and number kinds = %v, want %v", tc.src, tc.lang, got, tc.want)
		}
	}
}

func TestKeywordTableLanguages(t *testing.T) {
	for _, lang := range Languages() {
		if _, ok := keywordTable[lang]; !ok {
			t.Errorf("missing keyword table entry for %q", lang)
		}
	}
}

// TestPatternMatchesDedup: a single-token pattern matches at every
// token position, so a line with two keywords (type and func) would
// yield the same line twice. Identical spans are reported once.
func TestPatternMatchesDedup(t *testing.T) {
	src := []byte("type Handler func(w http.ResponseWriter, r *http.Request)")
	got := findPatternMatches("sample.go", src, mustParsePattern(t, "KEY"), tokenizeCode(src, "go"))
	if len(got) != 1 {
		t.Errorf("KEY matches = %d, want 1 (got %#v)", len(got), got)
	}
	// A line with one keyword still yields one match, and a line with
	// three type tokens yields one match too.
	src = []byte("func Map[T any, U any](in []T, f func(T) U) []U {")
	got = findPatternMatches("sample.go", src, mustParsePattern(t, "TYP"), tokenizeCode(src, "go"))
	if len(got) != 1 {
		t.Errorf("TYP matches = %d, want 1 (got %#v)", len(got), got)
	}
}

func TestMatchKeywordType(t *testing.T) {
	for _, tc := range []struct {
		pat, src, lang string
		want           bool
	}{
		{"KEY", "if", "go", true},
		{"KEY", "x", "go", false},
		{"KEY", "int", "go", false},
		{"KEY", "if", "", false},
		{"TYP", "int", "go", true},
		{"TYP", "x", "go", false},
		{"TYP", "if", "go", false},
		{"TYP", "int", "", false},
	} {
		_, got := matchAtoms(mustParsePattern(t, tc.pat), tokenizeCode([]byte(tc.src), tc.lang), 0, nil)
		if got != tc.want {
			t.Errorf("pattern %q against %q (%s) matched = %v, want %v", tc.pat, tc.src, tc.lang, got, tc.want)
		}
	}
}

func TestMatchExactCrossKind(t *testing.T) {
	for _, tc := range []struct {
		pat, src string
		want     bool
	}{
		{"if", "if", true},
		{"int", "int", true},
		{"If", "if", false},
		{"if", "int", false},
		{"'if'", "if", false},
		{"if", "'if'", false},
		{"_", "if", true},
		{"_1 _1", "int int", true},
		{"_1 _1", "int if", false},
	} {
		_, got := matchAtoms(mustParsePattern(t, tc.pat), tokenizeCode([]byte(tc.src), "go"), 0, nil)
		if got != tc.want {
			t.Errorf("pattern %q against Go source %q matched = %v, want %v", tc.pat, tc.src, got, tc.want)
		}
	}
}

func TestPatternFlagLang(t *testing.T) {
	const goFile = "testdata/sample.go"
	dir := t.TempDir()
	pythonFile := filepath.Join(dir, "source.py")
	unknownFile := filepath.Join(dir, "source.unknown")
	for _, file := range []string{pythonFile, unknownFile} {
		if err := os.WriteFile(file, []byte("def"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		pat       string
		wantFiles []string
	}{
		{"KEY", []string{pythonFile, goFile}},
		{"TYP", []string{goFile}},
		{"def", []string{pythonFile, unknownFile}},
	} {
		t.Run(tc.pat, func(t *testing.T) {
			out, err := os.CreateTemp(dir, "stdout")
			if err != nil {
				t.Fatal(err)
			}
			defer out.Close()
			stdout := os.Stdout
			os.Stdout = out
			defer func() { os.Stdout = stdout }()

			if code := run([]string{"--format", "json", "--pattern", tc.pat, goFile, pythonFile, unknownFile}); code != 0 {
				t.Fatalf("exit code = %d, want 0", code)
			}
			if _, err := out.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			var matches []patternResult
			if err := json.NewDecoder(out).Decode(&matches); err != nil {
				t.Fatal(err)
			}
			var files []string
			for _, match := range matches {
				if !slices.Contains(files, match.File) {
					files = append(files, match.File)
				}
			}
			if !slices.Equal(files, tc.wantFiles) {
				t.Errorf("matched files = %q, want %q", files, tc.wantFiles)
			}
		})
	}
}
