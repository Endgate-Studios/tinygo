# runtime/memory: notes from the TinyGo maintainers

Upstream TinyGo does not accept arena allocators or a manual `free` for Go
objects. This file collects what the maintainers said and why, with links.
For each point, it also gives what this package does about it.

This package stays in the fork. See `plans/upstream.md`.

## Summary

| Concern | Source | What this package does |
|---|---|---|
| An outside pointer into a freed arena becomes a dangling pointer | aykevl, #4434 | Not prevented. `arena.Heap.Free` panics on a double or invalid free. The `memory_debug` tag poisons freed memory. `misuse_test.go` shows the failures |
| The GC does not see pointers in an arena | aykevl, #5518. digitalentity, #5518. deadprogram, #5554 | `gcshim.AddRoot` scans a `Region` conservatively. `tools/arenacheck` fails when an arena type holds GC pointers. `gc_test.go` and `gcsuite` show which memory the GC reads |
| The API must be unsafe or return `[]byte` | aykevl, #5518 | `memory.New[T]` and `memory.MakeSlice[T]` return typed values from `unsafe` memory. The types are not checked at run time, only by `arenacheck` |
| Misuse gives memory corruption, not a panic | aykevl, #5518 | True here too. Only a double or invalid free panics |
| `runtime.Free(interface{})` can allocate | aykevl, #3182 | `free.Free[T]` takes `*T` and `free.FreeSlice[T]` takes `[]T`. No interface |
| What does a free release in a map, a channel or a struct? | aykevl, #3182 | One block only. Nothing that it points to |
| The compiler decides if a value is on the heap | aykevl, #3182 | Not solved. `free` is safe only for memory that the program knows is a heap block |
| `runtime.free` takes only manual allocations | Upstream commit `02021b58` | `free.Free` does nothing unless the build has a real free (`gc.custom` or `memory_realfree`) |
| `-gc=custom` has no stable API | aykevl, #3302 | gcshim uses `-gc=custom`, so each upstream merge can break it. Run `make memory-test` after each merge |
| A custom GC must mark the stack | aykevl, #3302 | gcshim calls `markRoots` for the stacks and `markCurrentGoroutineStack` with `-scheduler=cores` |

## Notes by thread

### #4434 Adding Arena Allocator? (closed, 2024)

<https://github.com/tinygo-org/tinygo/issues/4434>

aykevl:

> I don't see how an arena allocator can possibly be used safely in Go. Go is
> a garbage collected language, so if any outside objects have a pointer to an
> object inside the arena that will become a dangling pointer after freeing the
> arena.
>
> I can't prevent anybody from shooting themselves in the foot, but I don't
> think this is something we're going to add to TinyGo to be honest.
>
> A better solution in my opinion would be to improve the GC itself.

aykevl pointed to the Boehm GC work (#4385, merged). For an explicit `free` of
some allocations, aykevl asked for the use case: "Is it perhaps large areas of
pointer-free memory that are slow?" The use case was WebAssembly, where the GC
cannot scan the stack.

### #3182 About memory management tinygo (closed, 2022)

<https://github.com/tinygo-org/tinygo/issues/3182>

aykevl, on a `runtime.Free(interface{})`:

> - `interface{}` can itself be a heap object, therefore calling `runtime.Free`
>   can actually cause a heap allocation.
> - What exactly should it deallocate? For container objects like maps and
>   channels, should it also deallocate the values stored in there [...]?
> - How do you even know a value is heap allocated? The compiler decides how
>   values are allocated (even `new(int)` could be stack allocated, or not
>   allocated at all [...]).
> - [...] a `runtime.Free` is unsafe. It results in a high risk for dangling
>   pointers if not used correctly.
>
> So in summary I do not think a manual free function is a good fit for the Go
> language and is certainly hard to use correctly.

On reference counting, aykevl wants it "some day". Go allows interior pointers, so
it needs fat pointers and large compiler and runtime changes.

### #5518 Support discontinuous memory (open, 2026)

<https://github.com/tinygo-org/tinygo/issues/5518>

The issue asks for memory outside the main RAM range, such as the STM32H7 D2
and D3 SRAM, and for DMA buffers in special memory.

deadprogram has the same problem on the ESP32. The radio blobs use memory
ranges for DMA "right in the middle of everything".

digitalentity proposed a bump arena and wrote that "GC is unaware of these
regions and will not consider anything referenced from here as used". digitalentity later
proposed `AllocIn[T any](region Region) *T`, compiled like `new` with a GC
layout, so that the objects are visible to the GC.

aykevl:

> It looks like this issue is about two different things:
>
> - Discontinuous memory. This can be implemented without any user facing
>   changes, just by updating the GC to work over multiple address ranges.
> - NUMA memory, where some memory is special in some way.
>
> At the moment the assumption is that all memory used in DMA operations (such
> as `spi.Tx` on many chips) can be any heap allocated memory. We could change
> this, and maybe we need to, but if we do that's a breaking change [...].
>
> While arenas are a possibility, I would strongly advise against it. They
> don't play well with the GC (unless we make the GC also scan those areas at
> which point it's easier to just make the GC support multiple ranges) and an
> API would either have to be inherently unsafe or just return `[]byte`. If
> they are misused, it's very difficult to catch bugs since it will lead to
> memory corruption instead of a panic for example.

dgryski thinks that WASIp3 can also need discontinuous memory.

### #5554 ESP32-S3 PSRAM support (open)

<https://github.com/tinygo-org/tinygo/pull/5554>

deadprogram, review item 1:

> `.psram` isn't in the GC root set (`_globals_start`/`_globals_end` =
> `_sbss`/`_edata`), so a `//go:section .psram` variable holding a pointer or
> slice lets the GC free what it points at.

`testGCPointerOnlyInArena` in `src/examples/gcsuite` shows this on the board.

### #3302 Add -gc=custom option (merged, 2023)

<https://github.com/tinygo-org/tinygo/pull/3302>

aykevl:

> I don't think we will want to promise any kind of API stability here [...].
> The reason is that promising anything like that will make this feature a much
> bigger maintenance burden for a feature that shouldn't be used much anyway
> (if it is used a lot, support for that GC should be implemented upstream
> instead).

And on the stack:

> This is unsafe. The GC _must_ call `markStack` which will in turn call
> `markRoots`. Otherwise the stack won't be scanned for objects.

On WebAssembly, the shadow stack is necessary, because values on the VM stack
cannot be read from outside the function.

### Commit 02021b58 runtime: preserve malloc allocations until free (2026)

Upstream added `allocManual` and `freeManual` for C `malloc` memory. This
memory is pointer-free and stays allocated until `free`. The block GC keeps it
marked. `runtime.free` now accepts only this memory, and Go finalizers on it
are rejected. Because of this change, `free.Free` on a normal GC object
crashed with `free: invalid pointer` until the fork commit `edfd4d53`.

## What upstream wants instead

- A better GC, such as Boehm (`-gc=boehm`, #4385).
- A GC that works over more than one address range (#5518).
- Possibly reference counting, at a later time (#3182).
- `-gc=custom` for special cases, with no stable API (#3302).
