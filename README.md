# practical-ai-golab-2026
Ultimate Practical AI Workshop @ GoLab 2026

## Service

| Route                  | Description                                  |
| ---------------------- | -------------------------------------------- |
| `GET /health`          | `200` with an empty body                     |
| `GET /v1/hello/{user}` | `{"message":"Hello, {user}"}`; blank → `400` |
| `GET /v1/users`        | every user as a JSON array                   |

### Run locally

```sh
make dev-up    # Postgres 18 via docker compose on localhost:5434
make run       # applies pending goose migrations, then serves on :3000
make seed      # (other terminal) load sample users
curl localhost:3000/v1/users
```

Migrations live in `zarf/schemas/db` (goose format), are embedded in the
binary and applied on startup under a Postgres advisory lock, so several
instances starting together do not race.

### Configuration

`golab serve --help` lists every flag. Each flag can also come from an
environment variable (`--db.host` → `GOLAB_DB_HOST`) or a config file passed
with `--config`. Precedence: flag > env > config file > default.

### Tests

```sh
make test        # unit + Postgres integration tests (needs Docker)
make test-short  # unit tests only
```
