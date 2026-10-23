.PHONY: infra migrate seed api check build test-backend-integration
infra:
	docker compose up -d postgres
migrate:
	cd backend && go run ./cmd/platform migrate
seed:
	cd backend && go run ./cmd/platform seed
api:
	cd backend && go run ./cmd/platform serve
check:
	cd backend && go vet ./... && go test -race -count=1 -p 2 -parallel 4 -timeout=20m ./...
	pnpm typecheck
	pnpm test
build:
	cd backend && go build -buildvcs=false ./cmd/platform
	pnpm build

# Explicit PostgreSQL target; bounded harness parallelism keeps schema migrations
# within local lock resources. Concurrency tests retain their own goroutines.
test-backend-integration:
	@test -n "$$TEST_DATABASE_URL" || { echo "TEST_DATABASE_URL is required" >&2; exit 1; }
	cd backend && go test -race -count=1 -p 2 -parallel 4 -timeout=20m ./...
