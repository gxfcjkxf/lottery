.PHONY: infra migrate seed api check build
infra:
	docker compose up -d postgres
migrate:
	cd backend && go run ./cmd/platform migrate
seed:
	cd backend && go run ./cmd/platform seed
api:
	cd backend && go run ./cmd/platform serve
check:
	cd backend && go vet ./... && go test -race ./...
	pnpm typecheck
	pnpm test
build:
	cd backend && go build -buildvcs=false ./cmd/platform
	pnpm build
