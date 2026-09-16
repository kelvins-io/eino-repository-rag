.PHONY: deps up down run tidy web web-install web-build

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

web-install:
	cd web && npm install

web:
	cd web && npm run dev

web-build:
	cd web && npm run build
