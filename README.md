# Dashboard Server (Go)

Backend API built with Go, `chi/v5`, `pgx/v5`, `go-redis/v9`, `stripe-go/v82`, and `robfig/cron/v3`. Drop-in compatible with the NestJS `dashboard-server` API, PostgreSQL schema, and `bruno_collection`.

---

## Getting Started

### 1. Environment Variables

Copy `.env.example` to `.env`:

```bash
cp .env.example .env
```

### 2. Start PostgreSQL & Valkey (Docker)

```bash
make docker-up
```

- **PostgreSQL**: `localhost:5432` (`postgres:postgres`, db: `dashboard`)
- **Valkey**: `localhost:6379`

### 3. Run Migrations & Seed

```bash
# Apply SQL migrations from migrations/
make migrate

# Or if attaching to a database already migrated by Prisma:
make migrate-baseline

# (Optional) Seed sample data (1 user, 5 products, 5 orders)
make seed
```

### 4. Run the Server

```bash
# Run directly
make dev

# Or build binary & run
make build && make run
```

---

## Testing & Quality

```bash
make fmt-check   # Verify gofmt formatting
make lint        # Run go vet
make test        # Run unit tests with -race
make test-e2e    # Run E2E tests against PostgreSQL & Valkey
make test-cov    # Generate coverage report
```

---

## API Endpoints

- `GET /health`, `GET /health/live`, `GET /health/ready` (no rate limiting)
- `POST /v1/users`, `GET /v1/users`, `GET /v1/users/{id}`, `PATCH /v1/users/{id}`, `DELETE /v1/users/{id}`
- `POST /v1/products`, `GET /v1/products`, `GET /v1/products/{id}`, `PATCH /v1/products/{id}`, `DELETE /v1/products/{id}`
- `POST /v1/orders`, `GET /v1/orders`, `GET /v1/orders/{id}`, `PATCH /v1/orders/{id}`, `DELETE /v1/orders/{id}`
- `POST /v1/stripe/payments`
- `POST /v1/jobs/sync-stripe`
