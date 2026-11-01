# ABC-1234 — Feature Diagrams

How the golab API (health, hello, list users) **should** work. Each diagram depicts intended behavior; where the code at `4eee246` diverges, the divergence is called out with the finding id from `finding.md` (e.g. **F1**).

## 1. System context

```mermaid
flowchart LR
    client["HTTP client<br/>(curl, browser, service)"]
    probe["Orchestrator probe<br/>(k8s / LB)"]

    subgraph svc["golab-api process (cmd/golab-api)"]
        mux["http.ServeMux"]
        sdk["app/sdk/api<br/>WriteJSON / WriteText"]
        userapp["app/domain/userapp<br/>Handlers"]
        userbus["business/domain/userbus<br/>Service"]
        userdb["userbus/stores/userdb<br/>Store"]
        migr["goose migrator<br/>(startup)"]
    end

    pg[("PostgreSQL<br/>users, goose_db_version")]
    files["zarf/schemas/db/*.sql"]

    client -- "GET /hello/{user}<br/>GET /users" --> mux
    probe -- "GET /health" --> mux
    mux --> userapp --> userbus --> userdb -- "sqlx + pgx" --> pg
    userapp --> sdk
    mux --> sdk
    files --> migr -- "DDL" --> pg
```

## 2. Client-facing HTTP contract

```mermaid
flowchart TB
    subgraph health["GET /health"]
        h1["200, empty body<br/>liveness only"]
    end

    subgraph hello["GET /hello/{user}"]
        he1["200 text/plain<br/>Hello, {user}"]
        he2["404 when segment empty<br/>(/hello/)"]
    end

    subgraph users["GET /users"]
        u1["200 application/json<br/>{ users: [ {id, name, created_at} ] }"]
        u2["200 { users: [] }<br/>empty table"]
        u3["500 application/json<br/>{ error: failed to fetch users }"]
    end

    note1["Intended: 500 on marshal failure.<br/>Current: implicit 200 + empty body (F2)"]
    note2["Intended: bounded + ordered by created_at, id.<br/>Current: unbounded, created_at only (F5)"]
    note3["Intended: readiness = separate DB-pinging endpoint (F11)"]

    u3 -.-> note1
    u1 -.-> note2
    h1 -.-> note3
```

## 3. Client interaction — sequence

```mermaid
sequenceDiagram
    autonumber
    actor C as Client
    participant M as ServeMux
    participant H as userapp.Handlers
    participant B as userbus.Service
    participant S as userdb.Store
    participant DB as PostgreSQL

    C->>M: GET /health
    M-->>C: 200 (no body)

    C->>M: GET /hello/alice
    M->>M: r.PathValue("user") = "alice"
    M-->>C: 200 text/plain "Hello, alice"

    C->>M: GET /users
    M->>H: ListUsers(w, r)
    H->>B: ListUsers(ctx)
    B->>S: FetchUsers(ctx)
    S->>DB: SELECT id, name, created_at FROM users ORDER BY created_at, id
    DB-->>S: rows
    loop each row
        S->>S: toBusUser(dbUser) — validates name
    end
    alt all rows valid
        S-->>B: []User
        B-->>H: []User
        H->>H: fromBusUsersResponse(users)
        H-->>C: 200 {"users":[...]}
    else query error or invalid row
        S-->>B: error
        B-->>H: "listing users: ..." (wrapped)
        H->>H: slog.ErrorContext(...)
        H-->>C: 500 {"error":"failed to fetch users"}
    end

    Note over S,DB: Current: no LIMIT, no tiebreaker (F5).<br/>One invalid row fails the whole list (F1).
```

## 4. Data types across layers

```mermaid
classDiagram
    direction LR

    namespace App_userapp {
        class listUsersResponse {
            +[]userResponse Users  json:"users"
        }
        class userResponse {
            +uuid.UUID ID  json:"id"
            +string Name  json:"name"
            +time.Time CreatedAt  json:"created_at"
        }
        class Handlers {
            -service *userbus.Service
            +Routes(mux)
            +ListUsers(w, r)
        }
    }

    namespace Business_userbus {
        class User {
            +uuid.UUID ID
            +types.UserName Name
            +time.Time CreatedAt
        }
        class Store {
            <<interface>>
            +FetchUsers(ctx) []User, error
        }
        class Service {
            -store Store
            +ListUsers(ctx) []User, error
        }
    }

    namespace Business_types {
        class UserName {
            <<strong type>>
            1..100 runes, trimmed
            +NewUserName(string) UserName, error
        }
    }

    namespace Storage_userdb {
        class dbUser {
            +uuid.UUID ID  db:"id"
            +string Name  db:"name"
            +time.Time CreatedAt  db:"created_at"
        }
        class userdbStore {
            -db *sqlx.DB
            +FetchUsers(ctx) []User, error
        }
    }

    listUsersResponse *-- userResponse
    Handlers --> Service
    Service --> Store
    userdbStore ..|> Store
    User *-- UserName
    userdbStore ..> dbUser : scans
    dbUser ..> User : toBusUser (can fail)
    User ..> userResponse : fromBusUserResponse (cannot fail)
```

Notes:
- `UserName` should be opaque (struct with unexported field) so only `NewUserName` can construct it; it is currently `type UserName string` (**F4**).
- Converter names should be `toBusUser` (Storage) and `fromBusUserResponse` (App, per item) (**F10**).
- Whether `uuid.UUID` / `time.Time` count as edge primitives in `userResponse` is an open team decision (**F10**).

## 5. Persistence model

```mermaid
erDiagram
    USERS {
        uuid id PK "app-generated (uuid.NewV7 recommended)"
        text name "NOT NULL, CHECK 1..100 chars, no leading/trailing whitespace"
        timestamptz created_at "NOT NULL DEFAULT now()"
    }
    GOOSE_DB_VERSION {
        int id PK
        bigint version_id
        bool is_applied
        timestamp tstamp
    }
```

Intended index: `(created_at, id)` to back ordering/pagination. Current migration has no CHECK on `name` (**F1**), no index (**F5**), and uses `IF NOT EXISTS` (**F8**).

## 6. Type-boundary crossings (read path)

```mermaid
flowchart LR
    row[("users row<br/>id uuid · name text · created_at timestamptz")]
    db["dbUser<br/>(native DB types)"]
    bus["userbus.User<br/>(strong types)"]
    resp["userResponse<br/>(primitives)"]
    json["JSON body"]
    err["error<br/>converting user[id]: ..."]

    row -- "sqlx.SelectContext scan" --> db
    db -- "toBusUser<br/>NewUserName(name)" --> bus
    db -. "invalid name" .-> err
    bus -- "fromBusUserResponse<br/>(cannot fail)" --> resp
    resp -- "json/v2 Marshal" --> json

    classDef fail fill:#fdd,stroke:#c33;
    class err fail;
```

Crossing *into* Business can fail (validation); crossing *out* cannot. Serialization (`json.Marshal`) can still fail and must surface as 500, not 200 (**F2**).

## 7. `GET /users` — handler decision flow

```mermaid
flowchart TD
    A["GET /users"] --> B["service.ListUsers(ctx)<br/>with per-query timeout"]
    B -->|error| E["log with distinct message<br/>500 {error}"]
    B -->|ok| C["fromBusUsersResponse"]
    C --> D["json.Marshal"]
    D -->|error| E2["log<br/>500 {error}"]
    D -->|ok| F["set Content-Type<br/>WriteHeader 200<br/>Write body"]
    F -->|write error| G["log only<br/>(headers already sent)"]
    F -->|ok| H["done"]
```

## 8. Corrupt-row policy (decision required)

```mermaid
flowchart TD
    R["row fails toBusUser"] --> Q{"policy?<br/>(product decision)"}
    Q -->|fail-closed<br/>current behavior| FC["FetchUsers returns error<br/>GET /users → 500 for everyone"]
    Q -->|skip-and-log| SL["log row id + reason<br/>omit row, continue<br/>GET /users → 200 partial"]
    P["Prevention (both paths):<br/>DB CHECK mirrors NewUserName"] -.-> R
```

## 9. Startup and shutdown lifecycle

```mermaid
sequenceDiagram
    autonumber
    participant OS as OS / orchestrator
    participant Main as main.run()
    participant Cfg as loadConfig
    participant Goose as goose Provider
    participant DB as PostgreSQL
    participant Srv as http.Server

    OS->>Main: start
    Main->>Cfg: read env DATABASE_URL, ADDR, MIGRATIONS_PATH
    alt DATABASE_URL unset (intended)
        Cfg-->>Main: error → exit 1
    end
    Note over Cfg: Current: silently falls back to<br/>postgres:postgres@localhost, sslmode=disable (F6)
    Main->>DB: sqlx.ConnectContext("pgx") + ping
    Main->>Goose: NewProvider(db, embed.FS, session locker)
    Goose->>DB: acquire advisory lock
    Goose->>DB: apply pending migrations
    Goose->>DB: release lock
    Note over Goose,DB: Current: legacy global API, second pool,<br/>no lock, CWD-relative path (F7)
    Main->>Srv: ListenAndServe(ADDR)
    Srv-->>OS: serving
    OS->>Main: SIGINT / SIGTERM
    Main->>Srv: Shutdown(10s)
    alt timeout
        Main->>Srv: Close()
    end
    Main->>DB: db.Close()
    Main-->>OS: exit 0
```
