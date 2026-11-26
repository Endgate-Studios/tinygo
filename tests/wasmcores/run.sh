#!/bin/sh
# The wasm-cores checks, in order. TINYGO is the worktree's compiler.
set -e
cd "$(dirname "$0")"
root=$(cd ../.. && pwd)
TINYGO=${TINYGO:-$root/build/tinygo.exe}
export TINYGOROOT=$root
export GOFLAGS=-buildvcs=false
out=$(mktemp -d)
"$TINYGO" build -target=wasm-cores -o "$out/hello.wasm" ./hello.go
node shared_memory.js "$out/hello.wasm"
"$TINYGO" build -target=wasm-cores -scheduler=asyncify -o "$out/twocores.wasm" ./twocores
got=$(timeout 60 node "$root/targets/wasm_exec_cores_node.js" "$out/twocores.wasm")
echo "$got"
echo "$got" | grep -q "hello from core 1" && echo "$got" | grep -q "core 1 wrote 42" && echo "ok two cores, one memory"
"$TINYGO" build -target=wasm-cores -scheduler=asyncify -o "$out/tls.wasm" ./tls 2>/dev/null
got=$(timeout 60 node "$root/targets/wasm_exec_cores_node.js" "$out/tls.wasm")
echo "$got"
echo "$got" | grep -q "core 1 sees 7" && echo "$got" | grep -q "core 0 sees 0" && echo "ok each core its own thread-locals"
"$TINYGO" build -target=wasm-cores -scheduler=asyncify -o "$out/gotls.wasm" ./gotls
got=$(timeout 60 node "$root/targets/wasm_exec_cores_node.js" "$out/gotls.wasm")
echo "$got"
echo "$got" | grep -q "core 1 mine 7" && echo "$got" | grep -q "core 0 mine 0 shared 99" && echo "ok //go:threadlocal per core"
"$TINYGO" build -target=wasm-cores -scheduler=asyncify -o "$out/perstate.wasm" ./perstate
got=$(timeout 60 node "$root/targets/wasm_exec_cores_node.js" "$out/perstate.wasm")
echo "$got"
echo "$got" | grep -q "shared" || echo "ok the runtime's state per core"
"$TINYGO" build -target=wasm-cores -o "$out/spread.wasm" ./spread
got=$(WASMCORES_CORES=5 timeout 60 node "$root/targets/wasm_exec_cores_node.js" "$out/spread.wasm")
echo "$got"
echo "$got" | awk '/cores used/ { if ($5 >= 2 && $NF >= 15) print "ok goroutines across cores, " $NF/10 "x faster" }'
for p in hellocores migrate v_chan v_wg v_mutex v_defer coreid wgonly; do
	"$TINYGO" build -target=wasm-cores -o "$out/$p.wasm" ./$p
	WASMCORES_CORES=3 timeout 60 node "$root/targets/wasm_exec_cores_node.js" "$out/$p.wasm" > "$out/$p.txt" 2>&1 || { echo "FAIL $p"; cat "$out/$p.txt"; exit 1; }
done
echo "ok goroutines, channels, mutexes, wait groups and defers on two cores"
"$TINYGO" build -target=wasm-cores -o "$out/panics.wasm" ./panics
for i in 1 2 3 4 5; do
	got=$(WASMCORES_CORES=5 timeout 60 node "$root/targets/wasm_exec_cores_node.js" "$out/panics.wasm" 2>&1) || { echo "FAIL panics: $got" | head -5; exit 1; }
	echo "$got" | grep -q "wrong results 0 recovered 800 of 800" || { echo "FAIL panics: $got"; exit 1; }
done
echo "ok panics, recover and returned values across cores (5 runs)"
"$TINYGO" build -target=wasm-cores -o "$out/stress.wasm" ./stress
for n in 2 3 5 8; do
	for i in 1 2 3; do
		got=$(WASMCORES_CORES=$n timeout 60 node "$root/targets/wasm_exec_cores_node.js" "$out/stress.wasm" 2>&1) || { echo "FAIL stress cores=$n: $got" | head -5; exit 1; }
		echo "$got" | grep -q "^stress ok" || { echo "FAIL stress cores=$n: $got"; exit 1; }
	done
done
echo "ok mutex, rwmutex, channels, select, cond, once, atomics, timers on 1, 2, 4 and 7 cores (3 runs each)"
"$TINYGO" build -target=wasm-cores -o "$out/allocdefer.wasm" ./allocdefer
"$TINYGO" build -target=wasm-cores -o "$out/gcmin.wasm" ./gcmin
"$TINYGO" build -target=wasm-cores -o "$out/gcstress.wasm" ./gcstress
for n in 2 3 5 9; do
	got=$(WASMCORES_CORES=$n timeout 60 node "$root/targets/wasm_exec_cores_node.js" "$out/allocdefer.wasm" 2>&1) || { echo "FAIL allocdefer cores=$n: $got" | head -5; exit 1; }
	got=$(WASMCORES_CORES=$n timeout 60 node "$root/targets/wasm_exec_cores_node.js" "$out/gcmin.wasm" 2>&1) || { echo "FAIL gcmin cores=$n: $got" | head -5; exit 1; }
	echo "$got" | grep -q "gcmin kept 100" || { echo "FAIL gcmin cores=$n: $got"; exit 1; }
	for i in 1 2 3; do
		got=$(WASMCORES_CORES=$n timeout 60 node "$root/targets/wasm_exec_cores_node.js" "$out/gcstress.wasm" 2>&1) || { echo "FAIL gcstress cores=$n: $got" | head -5; exit 1; }
		echo "$got" | awk '/gcstress bad 0 collections/ { if ($5 >= 5) ok = 1 } END { exit !ok }' || { echo "FAIL gcstress cores=$n: $got"; exit 1; }
	done
done
echo "ok collections with every core allocating, on 1, 2, 4 and 8 cores (3 runs each)"
for p in reflectcall reflectmethod; do
	"$TINYGO" build -target=wasm-cores -o "$out/$p.wasm" ../$p
	for n in 2 5; do
		got=$(WASMCORES_CORES=$n timeout 60 node "$root/targets/wasm_exec_cores_node.js" "$out/$p.wasm" 2>&1) || { echo "FAIL $p cores=$n: $got" | head -5; exit 1; }
		echo "$got" | grep -q "^$p ok" || { echo "FAIL $p cores=$n: $got"; exit 1; }
	done
done
echo "ok reflect Call, Method and MethodByName on 1 and 4 cores"
