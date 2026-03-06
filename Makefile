ifneq (,$(wildcard ./.env))
include .env
export
endif

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

compose-up:
	docker compose up -d

compose-down:
	docker compose down -v

build:
	go build ./...