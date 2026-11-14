.PHONY: db test dev-dns dev-api dev-front up down
db:        ; docker compose up -d postgres redis
test:      ; cd backend && go mod tidy && go test ./...
dev-dns:   ; cd backend && set -a && . ../.env && set +a && go run ./cmd/dns
dev-api:   ; cd backend && set -a && . ../.env && set +a && go run ./cmd/api
dev-front: ; cd frontend && npm install && npm run dev
up:        ; docker compose --profile full up -d --build
down:      ; docker compose --profile full down
