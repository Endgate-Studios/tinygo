// Command browser runs a TinyGo wasm program in headless Chrome and prints
// its output: on a cross-origin isolated local page (so SharedArrayBuffer
// works), with the cores glue when -cores is set. It exits with the
// program's exit code.
//
//	go run ./tests/wasmcores/browser -cores=4 prog.wasm
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

//go:embed page
var page embed.FS

var verbose bool

func main() {
	cores := flag.Int("cores", 0, "run with the cores glue and this many cores (0: plain wasm_exec.js)")
	timeout := flag.Duration("timeout", 60*time.Second, "how long to wait for the program to end")
	chrome := flag.String("chrome", `C:\Program Files\Google\Chrome\Application\chrome.exe`, "the Chrome to run headless")
	targets := flag.String("targets", "targets", "the TinyGo targets directory (wasm_exec.js and the cores glue)")
	flag.BoolVar(&verbose, "v", false, "print the page's URL and Chrome's own output")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: browser [-cores=N] prog.wasm")
		os.Exit(2)
	}
	code, err := run(flag.Arg(0), *targets, *chrome, *cores, *timeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "browser:", err)
		os.Exit(2)
	}
	os.Exit(code)
}

func run(wasm, targets, chrome string, cores int, timeout time.Duration) (int, error) {
	static, err := fs.Sub(page, "page")
	if err != nil {
		return 0, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()

	var mu sync.Mutex
	done := make(chan int, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /test.wasm", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/wasm")
		http.ServeFile(w, r, wasm)
	})
	for _, name := range []string{"wasm_exec.js", "wasm_exec_cores.js"} {
		mux.HandleFunc("GET /"+name, func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(targets, name))
		})
	}
	mux.Handle("GET /", http.FileServerFS(static))
	mux.HandleFunc("POST /log", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		os.Stdout.Write(append(b, '\n'))
		mu.Unlock()
	})
	mux.HandleFunc("POST /done", func(w http.ResponseWriter, r *http.Request) {
		code, _ := strconv.Atoi(r.URL.Query().Get("code"))
		if r.URL.Query().Get("idle") != "" {
			fmt.Fprintln(os.Stderr, "browser: the program is idle (main returned)")
		}
		select {
		case done <- code:
		default:
		}
	})
	srv := &http.Server{Handler: isolated(mux)}
	go srv.Serve(ln)
	defer srv.Shutdown(context.Background())

	profile, err := os.MkdirTemp("", "browser-profile")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(profile)
	ctx, cancel := context.WithCancel(context.Background())
	url := fmt.Sprintf("http://%s/?cores=%d", ln.Addr(), cores)
	cmd := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-first-run",
		"--no-default-browser-check", "--user-data-dir="+profile, url)
	if verbose {
		fmt.Fprintln(os.Stderr, "browser:", url)
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return 0, err
	}
	// Chrome stays up after the page is done: stop it, then wait.
	defer func() {
		cancel()
		cmd.Wait()
	}()

	select {
	case code := <-done:
		return code, nil
	case <-time.After(timeout):
		return 0, fmt.Errorf("no result after %v", timeout)
	}
}

// isolated makes every response cross-origin isolated, which a page needs
// for SharedArrayBuffer.
func isolated(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}
