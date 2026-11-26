#!/bin/sh
# The wasm-cores checks in headless Chrome (cross-origin isolated, Web
# Workers as the cores), for a target: wasm-cores (default) or wasm64-cores.
# The prototypes that start cores by hand stay in run.sh (Node).
set -e
cd "$(dirname "$0")"
root=$(cd ../.. && pwd)
TINYGO=${TINYGO:-$root/build/tinygo.exe}
TARGET=${1:-wasm-cores}
export TINYGOROOT=$root
export GOFLAGS=-buildvcs=false
out=$(mktemp -d)
go build -o "$out/browser.exe" ./browser
browser() { # cores wasm
	timeout 180 "$out/browser.exe" -cores="$1" -timeout 120s -targets "$root/targets" "$2" 2>&1
}
build() { "$TINYGO" build -target="$TARGET" -o "$out/$1.wasm" "./$2"; }

build spread spread
got=$(browser 5 "$out/spread.wasm")
echo "$got" | awk '/cores used/ { if ($5 >= 2 && $NF >= 15) { print "ok goroutines across cores, " $NF/10 "x faster"; ok = 1 } } END { exit !ok }' || { echo "FAIL spread: $got"; exit 1; }
for p in hellocores migrate v_chan v_wg v_mutex v_defer coreid wgonly; do
	build $p $p
	browser 3 "$out/$p.wasm" > "$out/$p.txt" || { echo "FAIL $p"; cat "$out/$p.txt"; exit 1; }
done
echo "ok goroutines, channels, mutexes, wait groups and defers on two cores"
build panics panics
for i in 1 2 3; do
	got=$(browser 5 "$out/panics.wasm") || { echo "FAIL panics: $got" | head -5; exit 1; }
	echo "$got" | grep -q "wrong results 0 recovered 800 of 800" || { echo "FAIL panics: $got"; exit 1; }
done
echo "ok panics, recover and returned values across cores (3 runs)"
build stress stress
for n in 2 3 5 8; do
	got=$(browser $n "$out/stress.wasm") || { echo "FAIL stress cores=$n: $got" | head -5; exit 1; }
	echo "$got" | grep -q "^stress ok" || { echo "FAIL stress cores=$n: $got"; exit 1; }
done
echo "ok mutex, rwmutex, channels, select, cond, once, atomics, timers on 1, 2, 4 and 7 cores"
build allocdefer allocdefer
build gcmin gcmin
build gcstress gcstress
for n in 2 3 5 9; do
	got=$(browser $n "$out/allocdefer.wasm") || { echo "FAIL allocdefer cores=$n: $got" | head -5; exit 1; }
	got=$(browser $n "$out/gcmin.wasm") || { echo "FAIL gcmin cores=$n: $got" | head -5; exit 1; }
	echo "$got" | grep -q "gcmin kept 100" || { echo "FAIL gcmin cores=$n: $got"; exit 1; }
	got=$(browser $n "$out/gcstress.wasm") || { echo "FAIL gcstress cores=$n: $got" | head -5; exit 1; }
	echo "$got" | awk '/gcstress bad 0 collections/ { if ($5 >= 5) ok = 1 } END { exit !ok }' || { echo "FAIL gcstress cores=$n: $got"; exit 1; }
done
echo "ok collections with every core allocating, on 1, 2, 4 and 8 cores"
for p in reflectcall reflectmethod; do
	build $p ../$p
	for n in 2 5; do
		got=$(browser $n "$out/$p.wasm") || { echo "FAIL $p cores=$n: $got" | head -5; exit 1; }
		echo "$got" | grep -q "^$p ok" || { echo "FAIL $p cores=$n: $got"; exit 1; }
	done
done
echo "ok reflect Call, Method and MethodByName on 1 and 4 cores"
echo "all ok in Chrome on $TARGET"
