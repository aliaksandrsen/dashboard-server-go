# Project Rules & Agent Guidelines (Go)

## 1. Overview & Tech Stack

- **Language**: Go (1.27+)
- **HTTP Router**: `github.com/go-chi/chi/v5` (`net/http` compatible)
- **Database**: PostgreSQL with `github.com/jackc/pgx/v5/pgxpool`
- **Cache**: Redis / Valkey with `github.com/redis/go-redis/v9`
- **Payments**: Stripe via `github.com/stripe/stripe-go/v82`
- **Cron Scheduler**: `github.com/robfig/cron/v3`
- **Logging**: stdlib `log/slog` + `gopkg.in/natefinch/lumberjack.v2`

---

## 2. Commands (Makefile)

- **Development**: `make dev` (`go run ./cmd/server`)
- **Build**: `make build` (`go build -o bin/dashboard-server ./cmd/server`)
- **Format**: `make fmt` / `make fmt-check`
- **Lint**: `make lint` (`go vet ./...`)
- **Unit Tests**: `make test` (`go test -race -v ./internal/...`)
- **E2E Tests**: `make test-e2e` (`go test -race -v -tags=e2e ./internal/server/...`)
- **Migrations**: `make migrate` (or `make migrate-baseline` for existing Prisma DB)
- **Seed**: `make seed`

---

## 3. Database Schema Compatibility

- Tables and columns preserve exact Prisma naming (`"User"`, `"Order"`, `"Product"`, `"_OrderToProduct"` with camelCase quoted identifiers) so this service is a drop-in replacement for the Node.js `dashboard-server`.
- Always quote identifiers in raw SQL (`"User"`, `"createdAt"`, `"stripeProductId"`, etc.).
- Use parameterized queries (`$1`, `$2`, ...) exclusively; never concatenate user input into SQL.

---

## 4. Architecture & HTTP Conventions

- Handlers parse/validate HTTP input (`httputil.DecodeStrictJSON`, `httputil.ValidateAllowedQueryParams`), delegate to services, and write responses via `httputil.WriteJSON` / `httputil.WriteError`.
- Errors use unified `ErrorResponseBody` (`statusCode`, `error`, `message`, `path`, `timestamp`, `requestId`).
- Nullable JSON fields (`stripeProductId`, `stripePriceId`, `stripeInvoiceId`, `userId`, `Order.user`) must always serialize as `null` when unset (do not use `omitempty` on them).
- Slice fields in API responses (`orders`, `products`) must serialize as `[]` rather than `null`.

---

## 5. Testing & Quality Standards

- Co-locate unit tests (`*_test.go`) inside each package under `internal/`.
- Run `make fmt-check`, `make lint`, and `make test` before finalizing changes.
- Preserve existing comments and docstrings unrelated to your changes.
