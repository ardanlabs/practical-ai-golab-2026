# Service DiffGuard Report — ABC-1234

Scope: user-listing service at `4eee246` — `app/`, `business/`, `cmd/golab-api/`, `zarf/schemas/db/`, `go.mod`, `go.sum` (feature commit `45a0899` + go.sum follow-ups `a4b4995`, `4eee246`). Excludes `.agents/`, `prompt-review.txt`.
Criteria source: Jira ABC-1234 was not accessible (unauthenticated `acli`, anonymous 404). Acceptance criteria below are the **proxy criteria** in the repo's `prompt.txt:5-9` (the original feature request) plus the repo's normative architecture skills. Re-run the AC pass once the ticket is readable.
Baseline: `go build`, `go vet`, `staticcheck`, `go fix -diff`, `go mod tidy -diff`, `go test -race ./...`, `govulncheck ./...` — all clean. Coverage: `types` 100%, `userapp` 92.3%; `userbus`, `userdb`, `api`, `cmd/golab-api` have no tests.

Overall Gate: **G3 — Repair**

## Run matrix

| Lens               | Result | Notes                                                                                    |
|--------------------|--------|------------------------------------------------------------------------------------------|
| Security Sentinel  | G2     | No secrets/injection/SSRF. Unauthenticated unbounded list; fail-open superuser DSN.      |
| Spec Cartographer  | G3     | "All users" not delivered (one bad row fails all); "any GET" ambiguous. Spec gaps S1–S5. |
| Service Steward    | G2     | Schema/domain mismatch, unbounded query, unlocked startup migrations.                    |
| Error Tripwire     | G3     | Marshal failure returns 200 + empty body; `IF NOT EXISTS` hides schema drift.            |
| Doc Drift Check    | G2     | "validated" `UserName`, "returns all users", `WriteJSON` failure modes overstate.        |
| Harness Map        | G3     | Storage layer and migration entirely untested; wire contract not pinned.                 |
| Boundary Keeper    | G2     | `UserName` invariant bypassable at compile time; converter naming off-skill.             |
| Clone Hunter       | G2     | Two DB pools/driver spellings for one DSN; minor test/helper duplication.                |
| Straight-Line Pass | G2     | Extract per-item converter; tighten test assertions; goose `NewProvider`.                |

## Acceptance-criteria coverage

Proxy criteria (`prompt.txt`), not the Jira ticket.

| #  | Criterion                                                                 | Verdict | Sites                                              | Proof / gap                                                                                                     |
|----|---------------------------------------------------------------------------|---------|----------------------------------------------------|-----------------------------------------------------------------------------------------------------------------|
| P1 | Three routes: healthcheck, Hello, List Users                              | Met     | `main.go:72-73`, `userapp.go:55`, `main.go:110-121` | All three registered on the mux.                                                                                |
| P2 | Healthcheck replies 200 to any GET                                        | Partial | `main.go:111-113`                                  | `GET /health` → 200, but `GET /` and any other path → 404, so the literal "any GET request" is unmet. Likely intent is a single health path (S1) — confirm. Untested. |
| P3 | Hello: GET, replies `Hello, {USER}`, user from the route                  | Met     | `main.go:115-120`, `api.go:30-39`                  | `r.PathValue("user")`, `text/plain`. Untested. Empty segment (`/hello/`) → 404 via ServeMux.                     |
| P4 | List Users: Postgres via sqlx, fetch **all** users, render as JSON        | Partial | `userdb.go:47-69` → `userbus.go:37-44` → `userapp.go:60-73` | Implemented, but one row failing `NewUserName` fails the whole request with 500 (F1). "All" is not delivered under realistic data. |
| P5 | Schema in `zarf/schemas/db` as a goose migration                          | Met     | `20261101000000_create_users.sql`, `main.go:57,124-140` | Applied at startup; goose maps `"postgres"` → `pgx` driver, which is imported.                                   |
| R1 | Every layer crossing via a named converter (`layered-architecture-types`) | Partial | `userdb.go:24`, `userapp.go:27-41`                 | Read path present; names deviate (`toBus` ≠ `toBusUser`; no `fromBusUserResponse`) (F10). Write path out of scope (no create in P1–P5). |
| R2 | Strong types in Business are always valid                                 | Partial | `business/types/user.go:16`                        | Validation exists, but the type can be constructed without it (F4).                                              |

## Spec/AC weaknesses

- **S1 — Health scope.** "any GET request" could mean any path. Tighten: "`GET /health` returns 200 with an empty body; liveness only, no dependency checks. Readiness is `GET /ready` (DB ping, 503 on failure)."
- **S2 — Hello input.** Unstated whether `{USER}` is a domain user (validate/lookup) or free text; no length/encoding bound. Tighten: "`GET /hello/{user}` returns `text/plain` `Hello, <user>`; `user` is the decoded path segment, 1–100 runes, else 400. No lookup."
- **S3 — List Users contract.** JSON shape, ordering, empty-table response, corrupt-row behavior, error body, auth, and the bound on "for now" are unspecified. Tighten: "Returns 200 `{"users":[{"id","name","created_at"}]}` ordered by `created_at, id` ascending; empty table → `{"users":[]}`; store error → 500 `{"error":"…"}`; invalid stored rows are <skipped and logged | fail the request>; pagination deferred to <ticket>; internal-network only until <auth ticket>."
- **S4 — Schema invariants.** Unstated that DB constraints must mirror Business rules (name 1–100 runes, trimmed), whether `id` has a DB default, and who sets `created_at`.
- **S5 — Migration execution.** Unstated whether the app migrates at boot or a separate job does, and how many replicas start concurrently.

## Fix queue

### G4 Stop

None.

### G3 Repair

- **[Spec, Steward, Boundary, Error, Harness, Doc] F1 — One invalid stored row fails `GET /users` for every caller; DB does not enforce the domain rule** — `zarf/schemas/db/20261101000000_create_users.sql:5`, `business/domain/userbus/stores/userdb/userdb.go:60-64`, `business/types/user.go:20-28`
    - Proof: column is `name text NOT NULL` with no CHECK, so Postgres accepts `''`, `'   '`, and >100-rune names. `toBus` → `NewUserName` rejects them; `FetchUsers` returns `nil, err` on the first failure; handler returns 500 (`userapp.go:61-67`). The PR has no write path, so every row arrives via manual SQL/seed/other writers — exactly the path that bypasses `NewUserName`. Additionally `NewUserName` trims, so a stored `"  Alice"` is served as `"Alice"` (lossy read); a future update path would silently rewrite data. Doc comments claim "returns all the users" (`userdb.go:52`, `userbus.go:36`) and "validates … and converts" (`user.go:18`) without mentioning fail-closed or trimming.
    - Patch direction: (1) migration `CHECK (char_length(name) BETWEEN 1 AND 100 AND name !~ '^\s|\s$')` — store canonical form; `btrim` alone strips only spaces while `strings.TrimSpace` strips all Unicode whitespace, so either align or make the DB stricter. (2) Decide and document fail-closed vs skip-and-log for legacy bad rows. (3) Make Storage `toBus` reject untrimmed input rather than normalize, or document the normalization.
    - Verify: inserting `''`, `' x'`, 101 runes fails at the DB; `toBus` table test covers valid/blank/too-long/untrimmed rows; chosen bad-row behavior asserted.

- **[Error, Doc] F2 — `WriteJSON` marshal failure returns 200 OK with an empty body** — `app/sdk/api/api.go:13-16`, `app/domain/userapp/userapp.go:70-72`
    - Proof: `WriteJSON` marshals before `WriteHeader`; on error it returns having written nothing. `ListUsers` only logs, so net/http sends an implicit 200 with no Content-Type and 0-length body. json/v2 fails on `time.Time` with year > 9999 (Postgres `timestamptz` allows up to 294276) and on invalid UTF-8 (`NewUserName` does not check `utf8.ValidString`; reachable only if the DB encoding is not UTF8). Low likelihood for this endpoint today, but `WriteJSON` is shared SDK code and every future caller inherits a silent-success contract; dashboards and clients record 2xx for a failed request.
    - Patch direction: on marshal error, `WriteJSON` writes a 500 itself (headers not yet committed) and returns the error; document that a write error means headers were already sent. Add `utf8.ValidString` to `NewUserName`.
    - Verify: handler test with fake store returning `CreatedAt: time.Date(10000, …)` asserts 500; `NewUserName("a\xff")` errors.

- **[Harness] F3 — Storage layer and migration have zero tests; no integration harness** — `business/domain/userbus/stores/userdb/userdb.go:24-69`, `zarf/schemas/db/20261101000000_create_users.sql`
    - Proof: `go test ./...` reports `[no test files]` for `userdb`. Nothing exercises `toBus` (including its error wrap with the row ID), the SQL/column tags vs schema, ordering, the pgx-stdlib → stdlib `uuid.UUID` scan, or migration up/down. No Makefile / integration target exists. A `db:` tag typo, SQL typo, or schema drift breaks production with no failing test.
    - Patch direction: (a) DB-free table test for `toBus`; (b) integration test (build tag or `testing.Short`) against throwaway Postgres: goose up/down, insert rows, `FetchUsers`, assert values/order/bad-row behavior.
    - Verify: `go test ./business/domain/userbus/stores/userdb/...` (unit) and the new integration target.

### G2 Tighten

- **[Boundary, Doc] F4 — `types.UserName` invariant is bypassable at compile time** — `business/types/user.go:15-16`
    - Proof: `type UserName string`, so `types.UserName("")`, untyped constants, and the zero value `userbus.User{}` all yield an invalid name without `NewUserName`. Tests already rely on it (`userapp_test.go:36-37`, `:102`). The doc says "represents a validated user name" — a guarantee only convention provides. No production path builds an invalid value today (the only producer is Storage `toBus`, which validates); the first write path or `toDBUser` will inherit the hole.
    - Patch direction: `type UserName struct{ value string }` + `String()`; `NewUserName` the only constructor; test helper `MustUserName`. `fromBus…` uses `user.Name.String()`. Do this before any write path lands.
    - Verify: `userbus.User{Name: "x"}` and `types.UserName("")` no longer compile.

- **[Security, Steward] F5 — `GET /users` is unauthenticated, unbounded, and can exhaust the DB** — `userapp.go:55`, `userdb.go:47-50`, `main.go:38,61`
    - Proof: no auth on the mux; `SELECT … FROM users ORDER BY created_at` without LIMIT, materialized three times then marshaled to one `[]byte`; default `ADDR=":3000"` binds all interfaces; `sqlx.ConnectContext` with no `SetMaxOpenConns` (unlimited); no per-query timeout (`WriteTimeout` does not cancel `r.Context()`). N concurrent requests ⇒ N full-table loads and N connections, up to Postgres `max_connections` for every tenant of that DB. `ORDER BY created_at` has no tiebreaker (`DEFAULT now()` is identical within a transaction), so order is unstable and keyset pagination impossible. `prompt.txt` says "big JSON for now" — so unpaginated may be deliberate, but nothing in code or docs records it, and names are plausibly PII.
    - Patch direction: `ORDER BY created_at, id` + index `(created_at, id)`; capped keyset pagination; `SetMaxOpenConns/SetMaxIdleConns/SetConnMaxLifetime`; `context.WithTimeout` per query; auth or loopback bind until auth exists. If deferring, document it on the handler and in the ticket.
    - Verify: store test with rows > limit asserts page size and stable order; load test confirms connection cap.

- **[Security, Error] F6 — Missing `DATABASE_URL` silently falls back to a superuser DSN with TLS off; runtime role needs DDL** — `cmd/golab-api/main.go:36`, `:57`, `:124-135`
    - Proof: `cmp.Or(os.Getenv("DATABASE_URL"), "postgres://postgres:postgres@localhost:5432/golab?sslmode=disable")`. A misnamed/missing env var starts the service as superuser against whatever answers on localhost (sidecar, shared host) and runs migrations there. The effective target is never logged. `migrate` reuses the same DSN, so the serving credential must hold CREATE/DROP.
    - Patch direction: fail startup when unset (default only behind an explicit dev flag); log host/db (never credentials); separate migration DSN/job with DDL role, runtime role limited to DML; `sslmode=verify-full` outside dev.
    - Verify: `loadConfig` test errors on empty env; role grants reviewed in deploy config.

- **[Steward, Clone, Straight-Line, Doc] F7 — Startup migrations use goose's global API: no lock, second pool, CWD-relative path** — `cmd/golab-api/main.go:37`, `:124-139`
    - Proof: `goose.OpenDBWithDriver("postgres", …)` opens a second `*sql.DB` (different driver spelling, no ping) alongside `sqlx.ConnectContext(ctx, "pgx", …)` at `:61`; future pgx/TLS/PgBouncer options will reach one and not the other. Legacy `goose.RunContext` + `SetBaseFS` mutate package globals and take no session lock (locking exists only via `Provider` + `WithSessionLocker`), so concurrent replicas race on `goose_db_version`. Default `MigrationsPath` `zarf/schemas/db` resolves against CWD; started elsewhere it fails with goose's opaque ". directory does not exist".
    - Patch direction: connect once with sqlx; `goose.NewProvider(goose.DialectPostgres, db.DB, fsys, goose.WithSessionLocker(lock.NewPostgresSessionLocker()))` + `p.Up(ctx)`, or a separate one-shot migration job; embed migrations via `embed.FS`.
    - Verify: two instances started concurrently against an empty DB both come up, one row per version; `rg 'OpenDBWithDriver|ConnectContext' cmd/` → one hit.
    - Assumption: severity of the lock gap depends on concurrent replicas (S5).

- **[Error, Spec] F8 — `CREATE TABLE IF NOT EXISTS` / `DROP TABLE IF EXISTS` hide schema drift** — `zarf/schemas/db/20261101000000_create_users.sql:3,12`
    - Proof: if `users` already exists with another shape, goose records the version as applied with no error; the mismatch surfaces at the first `GET /users` as a 500 (`column … does not exist`). Versioned migrations should fail loudly.
    - Patch direction: plain `CREATE TABLE` / `DROP TABLE`.
    - Verify: pre-create `users(id int)`; startup fails with `running migrations: …`.

- **[Harness, Steward, Straight-Line, Spec] F9 — Tests do not pin the public contract** — `app/domain/userapp/userapp_test.go:58-76,95-97,100-114`, `business/types/user_test.go:29-40`, `cmd/golab-api/main.go:110-121`
    - Proof: `TestListUsers` decodes into the production `listUsersResponse` (tag changes move encode and decode together) and builds `CreatedAt` into `want` but compares only `ID`/`Name`; length check hard-codes `2`. `TestListUsersStoreFailure` asserts status only — a change that writes `err.Error()` into the body (leak) passes. `NewUserName` has no exactly-100 case, no multibyte case (swapping `RuneCountInString` for `len` passes), and the over-length input derives from `maxUserNameLen`, so an accidental limit change still passes. `/health`, `/hello/{user}`, `WriteText`, and empty-table `{"users":[]}` are untested.
    - Patch direction: decode into a test-local struct or compare golden JSON; compare whole elements with `CreatedAt.Equal`; assert failure body equals `{"error":"failed to fetch users"}` and does not contain the internal error; table test for names with literal boundaries (100/101 ASCII, 100/101 `é`, padded 100); `httptest.NewTestServer` tests for health/hello; empty-store case.
    - Verify: changing the `created_at` tag, dropping `CreatedAt` in the converter, or leaking `err.Error()` each fails a test.

- **[Boundary, Steward, Straight-Line] F10 — App/Storage crossings deviate from `layered-architecture-types`** — `business/domain/userbus/stores/userdb/userdb.go:24`, `app/domain/userapp/userapp.go:15-41`
    - Proof: Storage converter is `toBus`, skill requires `toBusUser`. App has no per-type `fromBusUserResponse`; mapping is inlined in the list converter's loop, so the next single-user endpoint will copy it. Response struct is `userResponse`, skill specifies `<Type>Response`. `userResponse` carries `uuid.UUID` and `time.Time`, assigned straight from Business values; the skill says "the edges use primitives".
    - Patch direction: rename to `toBusUser`; add `fromBusUserResponse(userbus.User) UserResponse` and delegate from `fromBusUsersResponse`; if the team reads "primitives" strictly, use `ID string` (`user.ID.String()`, same wire bytes) and `CreatedAt string` (`Format(time.RFC3339)`).
    - Verify: grep shows `toBusUser`/`fromBusUserResponse`; no response-struct literals outside converters.
    - Assumption: whether stdlib value types count as edge primitives. Read literally, the field types are a boundary violation (G4); read as "no `business/types` at edges", only naming is off.

- **[Error, Spec] F11 — `/health` is green regardless of DB state** — `cmd/golab-api/main.go:111-113`
    - Proof: unconditional `WriteHeader(200)`; never touches `db`. If Postgres drops after startup, `/health` stays 200 while every `/users` call is 500.
    - Patch direction: keep `/health` as liveness; add a readiness endpoint with `db.PingContext` (short timeout, 503 on error).
    - Verify: stop Postgres; readiness → 503.
    - Assumption: `/health` will back a readiness/LB probe (no probe config in repo).

### G1 Polish

- `app/domain/userapp/userapp.go:64-66` — `api.WriteJSON` result ignored on the 500 path (checked at `:70`); anonymous error struct invites copy-paste — add `api.WriteError` / named `ErrorResponse` in `app/sdk`.
- `app/domain/userapp/userapp.go:63,71` — identical log message `"userapp: ListUsers"` for store failure vs write failure; make them distinguishable.
- `business/types/user.go:22-27` — `fmt.Errorf` without verbs → `errors.New` or exported sentinels (lets App map to 400 via `errors.Is`); `RuneCountInString` computed twice — single `switch n := …` per `branching-logic-flow`; "characters" in the message means runes.
- `cmd/golab-api/main.go:110-121` — health/hello handlers live in `main`, outside the App layer (`app/sdk/*` or an `app/domain/*app`), so they are untestable without the binary.
- `app/sdk/api/api.go:18-25,31-38` — `WriteJSON`/`WriteText` duplicate the header/status/write tail; extract a private `write`.
- `cmd/golab-api/main.go:84,100-102` — "service started" logged before bind (use `net.Listen` then `Serve`); on `Shutdown` timeout `srv.Close()` is not called before deferred `db.Close()`.
- `userapp_test.go` — tests lack `t.Parallel()` (unlike `types`); server/mux setup duplicated between two tests; mixed `"Alice"` vs `types.UserName("Alice")` literal styles.

## Open checks

- **Ticket.** All AC verdicts are against `prompt.txt`, not ABC-1234. Re-run the Spec Cartographer once the ticket is readable (`acli jira auth login`).
- **Corrupt-row policy (F1).** Fail-closed vs skip-and-log is a product decision; confirm before patching.
- **"Primitives" at the App edge (F10).** Confirm whether `uuid.UUID` / `time.Time` are allowed in `*Response` structs.
- **Replicas (F7).** Confirm whether multiple instances start concurrently; sets the migration-lock severity.
- **Probe usage (F11).** Confirm whether `/health` backs readiness.
- **Name content.** `NewUserName` accepts control, bidi-override, and zero-width runes — spoofing risk only if names are rendered in a UI or used as identifiers.
- **Startup logs.** pgx's DSN password redaction in parse/connect errors (`main.go:46`) is best-effort; keep startup logs within the same trust boundary as the secret.
- **Extension seam.** `userbus` uses `Service`/`New`/`Store`; `business-layer-extensions` expects `Business`/`NewBusiness`/`Storer`/`ExtBusiness`. Not required until a cross-cutting concern is added, but `userapp.Handlers` depends on concrete `*userbus.Service`, so adding the seam will change the App signature.
- **Not verified live.** sqlx scan of `uuid` → stdlib `uuid.UUID` and goose `"postgres"` → `pgx` mapping are verified from source (`database/sql/convert.go`, goose `db.go`), not against a running Postgres.
- **No secret scanner / SAST available.** Secrets sweep was manual; the only credential literal is the dev default DSN (`main.go:36`, F6).

## Suggested verification

- `go build ./... && go vet ./... && go test -race ./...`
- `go mod tidy -diff` (expect empty) and `govulncheck ./...`
- New: `go test ./business/domain/userbus/stores/userdb/...` (unit) plus an integration target against throwaway Postgres covering migration up/down, `FetchUsers`, and the corrupt-row behavior.
- Manual: start against local Postgres, `curl :3000/health`, `curl :3000/hello/alice`, `curl :3000/users`, send SIGTERM, confirm clean shutdown.
- Re-run Security Sentinel, Spec Cartographer, and Harness Map after fixes.
