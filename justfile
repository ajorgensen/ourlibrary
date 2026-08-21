set dotenv-load := true

export DATABASE_URL := env("DATABASE_URL", "postgres://ourlibrary:ourlibrary@localhost:5432/ourlibrary?sslmode=disable")

_air_version := "v1.67.4"
_templ_version := "v0.3.1020"
_sqlc_version := "v1.31.1"
_goose_version := "v3.27.3"
_migrations := "internal/database/migrations"

# Show available commands.
default:
    @just --list

# Install project and development dependencies.
setup:
    npm install
    go install github.com/air-verse/air@{{_air_version}}
    go install github.com/a-h/templ/cmd/templ@{{_templ_version}}
    go install github.com/sqlc-dev/sqlc/cmd/sqlc@{{_sqlc_version}}
    go install github.com/pressly/goose/v3/cmd/goose@{{_goose_version}}
    just generate

# Generate database code, templates, and browser assets.
generate:
    sqlc generate
    templ generate
    npm run build

# Format source files.
fmt:
    gofmt -w cmd internal
    templ fmt internal/web/templates

# Run all tests and static checks.
check: generate
    go test ./...
    go vet ./...

# Build the server binary.
build: generate
    mkdir -p bin
    go build -o bin/ourlibrary ./cmd/server

# Run the server.
run:
    go run ./cmd/server

# Run the server with code generation and browser reloads.
dev: generate
    air

# Start PostgreSQL and wait until it is ready.
db-up:
    docker compose up -d --wait postgres

# Stop PostgreSQL.
db-down:
    docker compose down

# Apply all pending migrations.
migrate:
    goose -dir {{_migrations}} postgres "{{DATABASE_URL}}" up

# Roll back the latest migration.
migrate-down:
    goose -dir {{_migrations}} postgres "{{DATABASE_URL}}" down

# Show migration status.
migrate-status:
    goose -dir {{_migrations}} postgres "{{DATABASE_URL}}" status

# Create a new SQL migration.
new-migration name:
    goose -dir {{_migrations}} create "{{name}}" sql
