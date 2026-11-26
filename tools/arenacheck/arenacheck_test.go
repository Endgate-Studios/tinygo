// Package arenacheck checks that arena memory holds no GC pointers. The GC does not
// read some arena memory, so it can free an object whose only pointer is there.
package arenacheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/tinygo-org/tinygo/compileopts"
	"github.com/tinygo-org/tinygo/goenv"
	"github.com/tinygo-org/tinygo/loader"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// allowed lists the arena types with GC pointers that are safe, and why.
var allowed = map[string]string{
	"tinygo/crypto/aes.Cipher": "mem is a global strategy and rk comes from the same Memory",
	"tinygo/net/tls.hsState":   "the certificate slices point into the handshake buffer of the same Memory",
}

// target loads the radio stack, which is the main user of runtime/memory.
const target = "esp32s3-psram-octal"

func TestArenaTypesHaveNoGCPointers(t *testing.T) {
	root := goenv.Get("TINYGOROOT")
	pkgs := memoryUsers(t, filepath.Join(root, "src"))
	dir := t.TempDir()
	var main strings.Builder
	main.WriteString("package main\n\nimport (\n")
	for _, p := range pkgs {
		main.WriteString("\t_ " + strconv.Quote(p) + "\n")
	}
	main.WriteString(")\n\nfunc main() {}\n")
	write(t, filepath.Join(dir, "main.go"), main.String())
	write(t, filepath.Join(dir, "go.mod"), "module arenacheck\n\ngo 1.24\n")

	options := &compileopts.Options{Target: target, Directory: dir}
	spec, err := compileopts.LoadTarget(options)
	if err != nil {
		t.Fatal(err)
	}
	config := &compileopts.Config{Options: options, Target: spec}
	lprogram, err := loader.Load(config, ".", types.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := lprogram.Parse(); err != nil {
		t.Fatal(err)
	}
	prog := lprogram.LoadSSA()
	prog.Build()

	used := map[string]bool{}
	for fn := range ssautil.AllFunctions(prog) {
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				call, ok := instr.(ssa.CallInstruction)
				if !ok {
					continue
				}
				T, ok := arenaTypeArg(call.Common())
				if !ok || !hasPointers(T, nil) {
					continue
				}
				name := types.TypeString(T, nil)
				pos := prog.Fset.Position(call.Pos())
				if _, ok := allowed[name]; ok {
					used[name] = true
					continue
				}
				t.Errorf("%s: arena memory of type %s holds GC pointers", pos, name)
			}
		}
	}
	for name := range allowed {
		if !used[name] {
			t.Errorf("allowed has %s, but no arena allocation uses it", name)
		}
	}
}

// arenaTypeArg returns T of a call to memory.New[T] or memory.MakeSlice[T].
func arenaTypeArg(c *ssa.CallCommon) (types.Type, bool) {
	fn := c.StaticCallee()
	if fn == nil || fn.Origin() == nil || fn.Origin().Pkg == nil {
		return nil, false
	}
	origin := fn.Origin()
	if origin.Pkg.Pkg.Path() != "runtime/memory" || (origin.Name() != "New" && origin.Name() != "MakeSlice") {
		return nil, false
	}
	return fn.TypeArgs()[0], true
}

// hasPointers reports whether a value of type T can hold a pointer that the GC must see.
func hasPointers(T types.Type, seen map[types.Type]bool) bool {
	switch u := T.Underlying().(type) {
	case *types.Basic:
		return u.Kind() == types.String || u.Kind() == types.UnsafePointer
	case *types.Array:
		return u.Len() > 0 && hasPointers(u.Elem(), seen)
	case *types.Struct:
		if seen[T] {
			return false
		}
		if seen == nil {
			seen = map[types.Type]bool{}
		}
		seen[T] = true
		for i := range u.NumFields() {
			if hasPointers(u.Field(i).Type(), seen) {
				return true
			}
		}
		return false
	default:
		// Pointers, slices, maps, channels, functions and interfaces.
		return true
	}
}

// memoryUsers returns the packages outside runtime/memory and the examples whose
// non-test files import runtime/memory.
func memoryUsers(t *testing.T, src string) []string {
	set := map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "runtime/memory" || rel == "examples" || strings.HasPrefix(rel, "runtime/memory/") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil
		}
		if importsMemory(f) {
			set[filepath.ToSlash(filepath.Dir(rel))] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var pkgs []string
	for p := range set {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	return pkgs
}

func importsMemory(f *ast.File) bool {
	for _, imp := range f.Imports {
		if p, _ := strconv.Unquote(imp.Path.Value); p == "runtime/memory" {
			return true
		}
	}
	return false
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o666); err != nil {
		t.Fatal(err)
	}
}
