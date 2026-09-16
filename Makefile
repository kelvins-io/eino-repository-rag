.PHONY: deps up down run tidy

deps:
	go mod tidy

up:
	docker compose up -d

down:
	docker compose down

run:
	go run ./cmd/server -config configs/config.yaml

tidy:
	go mod tidy
