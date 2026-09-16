.PHONY: deps up down run tidy web web-install web-build test eval docker-build docker-up docker-down

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

test:
	go test ./...

# 需依赖已启动且样例文档已导入；按实际 kb/tenant 改 examples/eval/golden.jsonl
eval:
	go run ./cmd/eval -config configs/config.yaml -golden examples/eval/golden.jsonl -k 5

docker-build:
	docker build -t eino-repository-rag:latest .
	docker build -t eino-repository-rag-web:latest ./web

# 依赖 + 后端/前端镜像（需 .env 中 DEEPSEEK_API_KEY / EMBEDDING_API_KEY）
docker-up:
	docker compose --profile app up -d --build

docker-down:
	docker compose --profile app --profile milvus down

web-install:
	cd web && npm install

web:
	cd web && npm run dev

web-build:
	cd web && npm run build
