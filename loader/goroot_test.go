package loader

import (
	"bufio"
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tinygo-org/tinygo/goenv"
)

func TestFilesToOverride(t *testing.T) {
	testCases := []struct {
		name          string
		goMinor       int
		sharedLibrary bool
		expected      map[string]string
	}{
		{
			name:     "Program",
			goMinor:  26,
			expected: map[string]string{},
		},
		{
			name:          "SharedLibrary",
			goMinor:       26,
			sharedLibrary: true,
			expected:      map[string]string{"syscall/rlimit.go": "syscall/goroot/rlimit.go"},
		},
		{
			name:          "SharedLibraryGo125",
			goMinor:       25,
			sharedLibrary: true,
			expected:      map[string]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			files := filesToOverride(tc.goMinor, tc.sharedLibrary)
			if !reflect.DeepEqual(files, tc.expected) {
				t.Errorf("expected %v, got %v", tc.expected, files)
			}
		})
	}
}

func TestListGorootMergeLinksFiles(t *testing.T) {
	goroot := t.TempDir()
	tinygoroot := t.TempDir()
	for _, name := range []string{
		filepath.Join(goroot, "src", "syscall", "rlimit.go"),
		filepath.Join(goroot, "src", "syscall", "syscall.go"),
		filepath.Join(goroot, "src", "syscall", "js", "js.go"),
		filepath.Join(goroot, "src", "time", "time.go"),
		filepath.Join(tinygoroot, "src", "syscall", "syscall.go"),
		filepath.Join(tinygoroot, "src", "syscall", "goroot", "rlimit.go"),
	} {
		if err := os.MkdirAll(filepath.Dir(name), 0777); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte("package x\n"), 0666); err != nil {
			t.Fatal(err)
		}
	}
	goSrc := filepath.Join(goroot, "src")
	tinygoSrc := filepath.Join(tinygoroot, "src")
	overrides := map[string]bool{"": true}

	merge, err := listGorootMergeLinks(goroot, tinygoroot, overrides, nil)
	if err != nil {
		t.Fatal(err)
	}
	if merge[filepath.Join("src", "syscall")] != filepath.Join(goSrc, "syscall") {
		t.Errorf("without file overrides, syscall must be linked as a directory: %v", merge)
	}

	files := map[string]string{"syscall/rlimit.go": "syscall/goroot/rlimit.go"}
	mergeFiles, err := listGorootMergeLinks(goroot, tinygoroot, overrides, files)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{
		filepath.Join("src", "syscall", "rlimit.go"):  filepath.Join(tinygoSrc, "syscall", "goroot", "rlimit.go"),
		filepath.Join("src", "syscall", "syscall.go"): filepath.Join(goSrc, "syscall", "syscall.go"),
		filepath.Join("src", "syscall", "js"):         filepath.Join(goSrc, "syscall", "js"),
		filepath.Join("src", "time"):                  filepath.Join(goSrc, "time"),
		"bin":                                         filepath.Join(goroot, "bin"),
		"lib":                                         filepath.Join(goroot, "lib"),
		"pkg":                                         filepath.Join(goroot, "pkg"),
	}
	if !reflect.DeepEqual(mergeFiles, expected) {
		t.Errorf("expected %v, got %v", expected, mergeFiles)
	}

	// The cached GOROOT is keyed on the links, so the overrides must change them.
	if reflect.DeepEqual(merge, mergeFiles) {
		t.Error("file overrides do not change the merge links")
	}

	_, err = listGorootMergeLinks(goroot, tinygoroot, map[string]bool{"": true, "syscall/": true}, files)
	if err == nil {
		t.Error("expected an error for a directory override and a file override of the same directory")
	}
	_, err = listGorootMergeLinks(goroot, tinygoroot, overrides, map[string]string{"syscall/missing.go": "syscall/goroot/rlimit.go"})
	if err == nil {
		t.Error("expected an error for an override of a file that is not in GOROOT")
	}
}

// TestGorootFileOverrides compares each replacement file with its GOROOT file
// minus the init functions. If it fails, update the replacement from GOROOT.
func TestGorootFileOverrides(t *testing.T) {
	_, minor, err := goenv.GetGorootVersion()
	if err != nil {
		t.Fatal(err)
	}
	files := filesToOverride(minor, true)
	if len(files) == 0 {
		t.Skipf("no file overrides for Go 1.%d", minor)
	}
	goSrc := filepath.Join(goenv.Get("GOROOT"), "src")
	for name, src := range files {
		t.Run(name, func(t *testing.T) {
			goDecls, goBuild := gorootFileDecls(t, filepath.Join(goSrc, name), true)
			tinygoDecls, tinygoBuild := gorootFileDecls(t, filepath.Join("..", "src", src), false)
			if goBuild != tinygoBuild {
				t.Errorf("build constraint: GOROOT has %q, TinyGo has %q", goBuild, tinygoBuild)
			}
			if !reflect.DeepEqual(goDecls, tinygoDecls) {
				t.Errorf("declarations are not the same as GOROOT %s minus init\nGOROOT:\n%s\nTinyGo:\n%s",
					name, strings.Join(goDecls, "\n"), strings.Join(tinygoDecls, "\n"))
			}
		})
	}
}

// gorootFileDecls returns the package clause and declarations of a file without
// comments, and its //go:build line. With dropInit, it removes the init functions.
func gorootFileDecls(t *testing.T, filename string, dropInit bool) (decls []string, build string) {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); strings.HasPrefix(line, "//go:build ") {
			build = line
		}
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, data, 0)
	if err != nil {
		t.Fatal(err)
	}
	decls = append(decls, "package "+file.Name.Name)
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "init" {
			if !dropInit {
				t.Errorf("%s has an init function", filename)
			}
			continue
		}
		var buf bytes.Buffer
		if err := printer.Fprint(&buf, fset, decl); err != nil {
			t.Fatal(err)
		}
		decls = append(decls, buf.String())
	}
	return decls, build
}
