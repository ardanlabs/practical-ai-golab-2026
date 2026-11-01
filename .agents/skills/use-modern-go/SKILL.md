---
name: use-modern-go
description: Apply modern Go syntax guidelines based on the project's Go version. Use whenever writing, editing, or reviewing Go (.go) code — not only when explicitly asked for guidelines.
---

# Modern Go Guidelines

## Detected Go Version

!`grep -rh "^go " --include="go.mod" . 2>/dev/null | cut -d' ' -f2 | sort | uniq -c | sort -nr | head -1 | xargs | cut -d' ' -f2 | grep . || echo unknown`

## How to Use

Use ONLY the version above — do not detect it yourself. Everything under **Always available** assumes the Go 1.22 floor and is safe on any currently supported Go version; gate anything newer on the detected version.

- **Version detected:** Say "This project is using Go X.XX, so I'll use modern Go features up to that version. Let me know to target a different one." Do not list features or ask for confirmation.
- **"unknown":** Say "Could not detect Go version." Then AskUserQuestion: "Which Go version should I target?" → [1.22] / [1.23] / [1.24] / [1.25] / [1.26] / [1.27].

When writing Go: use every feature up to the target, prefer modern built-ins/packages (`slices`, `maps`, `cmp`) over legacy patterns, never use features newer than the target.

## Modernizing Existing Files

If the **installed toolchain** is Go 1.26+, run `go fix` on the target package(s) **before reading any file** — it mechanically applies most rewrites below (range-over-int, `min`/`max`, `any`, `strings.Cut`, `maps.*`, `errors.AsType`, `wg.Go`, `new(expr)`). Go 1.27 adds `atomictypes`, `embedlit`, `slicesbackward`, `unsafefuncs`; it dropped `fmtappendf` and renamed `waitgroup` → `waitgroupgo`.

```bash
go version                       # gate on installed toolchain
go fix -diff ./path/...          # preview (unflagged form writes in place)
go fix ./path/...                # apply; run twice — one fix enables another
go fix ./path/...
go build ./path/... && go test ./path/...   # verify
```

Then read the post-fix files and apply remaining manual modernizations. Do not Read a file before `go fix` runs on its package.

Caveats:

- Only fixes when the declared Go version permits it (`go 1.X` in `go.mod` / `//go:build go1.X`).
- Skips generated files (per `//go:generate`) — fix the generator.
- Analyzes one build config; re-run per `GOOS`/`GOARCH` combo for tagged code.
- Resolves syntactic conflicts, not semantic ones (e.g. now-unused vars) — post-fix compile errors are normal and need manual fixes.
- `go tool fix help` lists fixers; disable one with `-name=false` (e.g. `-minmax=false`).
- Library migrations (`encoding/json/v2`, stdlib `uuid`, `httptest.NewTestServer`) are never automated — they are manual and can change behavior; see Go 1.27+.
- When committing, keep automated `go fix` changes in their own commit, separate from manual modernizations.

If the installed toolchain is older than Go 1.26, skip `go fix` and apply the features below manually.

---

## Features

### Always available (Go 1.22 floor)

Standard library:
- `time.Since(start)` / `time.Until(deadline)` over manual `.Sub`.
- `errors.Is(err, target)` over `==` (handles wrapping); `errors.Join(e1, e2)` to combine.
- `any` over `interface{}`.
- `strings.Cut` / `bytes.Cut`; `strings.CutPrefix` / `CutSuffix`.
- `strings.Clone` / `bytes.Clone`.
- `fmt.Appendf(buf, ...)` when appending to an existing buffer. Don't mechanically rewrite `[]byte(fmt.Sprintf(...))` — Go 1.27 dropped that fixer on style grounds.
- Type-safe atomics: `atomic.Bool`, `atomic.Int64`, `atomic.Pointer[T]` over `atomic.StoreInt32` etc.
- `context.WithCancelCause` + `context.Cause(ctx)`; `context.AfterFunc`, `context.WithTimeoutCause`, `context.WithDeadlineCause`.

Built-ins & generics:
- `min(a, b)` / `max(a, b)` over if/else.
- `clear(m)` / `clear(s)` to empty a map / zero a slice.
- `for i := range n` over `for i := 0; i < n; i++`. Loop variables are per-iteration — safe to capture in goroutines.
- `cmp.Or(a, b, "default")` returns first non-zero — e.g. `name := cmp.Or(os.Getenv("NAME"), "default")`.
- `reflect.TypeFor[T]()` over `reflect.TypeOf((*T)(nil)).Elem()`.

`slices`:
- `Contains`, `Index`, `IndexFunc` over manual loops.
- `Sort` / `SortFunc(s, func(a, b T) int { return cmp.Compare(a.X, b.X) })`.
- `Max`, `Min`, `Reverse`, `Compact` (dedupes consecutive), `Clip`, `Clone`.

`maps`: `Clone`, `Copy(dst, src)`, `DeleteFunc(m, pred)`.

`sync`: `sync.OnceFunc(fn)`, `sync.OnceValue(fn)` over `sync.Once` + wrapper.

`net/http`: method+path patterns — `mux.HandleFunc("GET /api/{id}", h)`, `r.PathValue("id")`.

### Go 1.23+

- `maps.Keys(m)` / `maps.Values(m)` return iterators.
- `slices.Collect(iter)` to build a slice; `slices.Sorted(iter)` to collect+sort.

```go
keys := slices.Collect(maps.Keys(m))
sortedKeys := slices.Sorted(maps.Keys(m))
for k := range maps.Keys(m) { process(k) }
```

- `time.Tick` is safe to use freely — the GC reclaims unreferenced tickers without `Stop`. No reason to prefer `NewTicker` when `Tick` will do.

### Go 1.24+

- `t.Context()` over `context.WithCancel(context.Background())` in tests. Always.
- `omitzero` over `omitempty` in JSON tags — always for `time.Duration`, `time.Time`, structs, slices, maps (`omitempty` doesn't work for these).
- `for b.Loop()` over `for i := 0; i < b.N; i++` in benchmarks.
- `strings.SplitSeq` / `FieldsSeq` (and `bytes.*`) over `strings.Split` when iterating in a for-range.

```go
for part := range strings.SplitSeq(s, ",") { process(part) }
```

### Go 1.25+

- `wg.Go(fn)` over `wg.Add(1)` + `go func() { defer wg.Done(); ... }()`.

```go
var wg sync.WaitGroup
for _, item := range items {
    wg.Go(func() { process(item) })
}
wg.Wait()
```

### Go 1.26+

- `new(val)` over `x := val; &x` — returns a pointer to any value. Type inferred: `new(0)` → `*int`, `new(true)` → `*bool`, `new(T{})` → `*T`. No redundant casts (`new(0)`, not `new(int(0))`). Common for pointer struct fields.

```go
cfg := Config{Timeout: new(30), Debug: new(true)}
```

- `errors.AsType[T](err)` over `errors.As(err, &target)`.

```go
if pathErr, ok := errors.AsType[*os.PathError](err); ok { handle(pathErr) }
```

### Go 1.27+

Language:
- Generic methods: a method may declare its own type parameters. Use them for helpers that belong to one type instead of package-level generic funcs taking the receiver as an argument. Interface methods cannot have type parameters, and generic methods never satisfy an interface.

```go
func (s *Set[T]) Map[U comparable](f func(T) U) *Set[U] { ... }
n := r.N(10 * time.Second) // math/rand/v2: (*Rand).N[Int] now a method
```

- Struct literal keys may name promoted fields: `User{ID: 1, Email: e}` where `ID` comes from embedded `Base`. Qualified keys (`User{Base.ID: 1}`) are still invalid. `go fix` (`embedlit`) applies it.
- Generic function inference works in any assignment/conversion to a function type: `var sumInts func([]int) int = Sum` — no `Sum[int]`.

Standard library:
- `uuid` (stdlib) over `github.com/google/uuid` / `gofrs/uuid`. `uuid.New()` (v4), `uuid.NewV7()` (time-ordered — prefer for DB primary keys), `Parse`, `MustParse`, `Nil()`, `Compare`; comparable with `==`; implements `TextMarshaler`/`TextUnmarshaler`. `database/sql` `Scan` accepts `*uuid.UUID` (from string or 16-byte source). It has **no** `driver.Valuer`: pgx v5 accepts it as a query argument (it is a `[16]byte`); for other drivers verify, or pass `id.String()`.
- `encoding/json/v2` (+ `encoding/json/jsontext`) for new code: `json.Marshal(v, opts...)`, `json.MarshalWrite(w, v)`, `json.UnmarshalRead(r, &v)`. v1 is now backed by v2 and stays supported — migrating is optional and **changes wire output**:
  - nil slice/map → `[]` / `{}` (v1: `null`). No more "never return nil" guards just for JSON.
  - No HTML escaping by default (v1 emits `\u003c`). Pass `jsontext.EscapeForHTML(true)` to keep v1 bytes.
  - Unmarshal is case-sensitive, rejects duplicate names and invalid UTF-8.
  - `time.Duration` has no default representation (error) — convert explicitly.
  - `omitempty` means "omit if encodes as null/\"\"/{}/[]"; prefer `omitzero`.
- `strings.CutLast` / `bytes.CutLast` over `strings.LastIndex` + manual slicing.

```go
dir, file, ok := strings.CutLast(path, "/")
```

- `(*url.URL).Clone()` / `url.Values.Clone()` over `u2 := *u` (shallow — shares `User`) or hand-rolled copies.
- `math/big`: `z.Divide(x, y, mode)` for quotient+remainder with `Trunc`/`Floor`/`Round`/`Ceil`.

Testing:
- `httptest.NewTestServer(t, h)` + `srv.Client()` over `httptest.NewServer(h)` + `t.Cleanup(srv.Close)` + `http.DefaultClient`. In-memory network (no ports, works with `testing/synctest`), auto-cleanup, fails the test if the handler panics. Requests may use any host (`http://example.com/path`); `srv.URL` is not needed.

```go
srv := httptest.NewTestServer(t, handler)
resp, err := srv.Client().Get("http://example.com/health")
```

- `synctest.Sleep(d)` over `time.Sleep(d); synctest.Wait()`.

Tooling & runtime:
- `go vet` flags `fmt.Errorf("...: %w", p)` when `p` is `*E` and `E` (the value type) implements `error` — wrap the value, not a pointer to it.
- `go test` runs the `stdversion` vet check: using a stdlib symbol newer than `go.mod`'s `go` line fails tests.
- `go mod tidy` merges `require` blocks into one direct + one indirect block.
- `asynctimerchan` GODEBUG is gone: remove `//go:debug asynctimerchan=1` / `godebug asynctimerchan=1` (old value now fails the build).
- Goroutine leaks: `pprof.Lookup("goroutineleak")` / `/debug/pprof/goroutineleak` is GA.
