package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeTree creates files (with empty content) under root, creating
// parent directories as needed.
func writeTree(t *testing.T, root string, rels []string) {
	t.Helper()
	for _, rel := range rels {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCollectFilesRecursive(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, []string{
		"a.py",
		"sub/b.go",
		"sub/deep/c.py",
		"sub/deep/d.txt",        // text parser: included
		"README.md",             // text parser: included
		"node_modules/pkg/e.js", // junk dir: skipped
		".git/hooks/f.py",       // junk dir: skipped
		"build/g.go",            // junk dir: skipped
	})
	got, err := collectFiles([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(root, "README.md"),
		filepath.Join(root, "a.py"),
		filepath.Join(root, "sub", "b.go"),
		filepath.Join(root, "sub", "deep", "c.py"),
		filepath.Join(root, "sub", "deep", "d.txt"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("collectFiles(%q) = %v, want %v", root, got, want)
	}
}

func TestCollectFilesDirectFileKept(t *testing.T) {
	root := t.TempDir()
	// A file passed directly is kept even without a registered
	// extension; the extractor reports it when the file is read.
	p := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := collectFiles([]string{p})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{p}) {
		t.Errorf("collectFiles(%q) = %v, want [%v]", p, got, p)
	}
}

func TestCollectFilesRootSkipDirKept(t *testing.T) {
	// A directory passed directly is walked even when its own name is
	// in skipDirs; only subdirectories are skipped.
	root := t.TempDir()
	build := filepath.Join(root, "build")
	writeTree(t, build, []string{"a.go", "sub/b.go"})
	got, err := collectFiles([]string{build})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(build, "a.go"),
		filepath.Join(build, "sub", "b.go"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("collectFiles(%q) = %v, want %v", build, got, want)
	}
}

func TestCollectFilesSorted(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, []string{"z.py", "a.py", "m/b.py"})
	// Passing the directories in reverse order still yields a sorted list.
	got, err := collectFiles([]string{filepath.Join(root, "m"), root})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(root, "a.py"),
		filepath.Join(root, "m", "b.py"),
		filepath.Join(root, "z.py"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("collectFiles = %v, want %v", got, want)
	}
}
