ifneq (,$(wildcard ./.env))
include .env
export
endif

.PHONY: db-up db-down db-reset db-logs key build run test web-dev

## 起本地 MySQL（首次启动会自动跑三个 schema.sql）
db-up:
	docker compose up -d mysql

## 停止但保留数据（下次 db-up 数据还在）
db-down:
	docker compose down

## 停止并清空数据卷，下次 db-up 会重新执行 schema.sql——改了表结构之后用这个
db-reset:
	docker compose down -v

db-logs:
	docker compose logs -f mysql

## 生成一个可以填进 .env 的 ACCOUNT_ENCRYPTION_KEY
key:
	@openssl rand -base64 32

build:
	go build -o bin/server ./cmd/server

## 依赖 .env 里的 MYSQL_DSN / ACCOUNT_ENCRYPTION_KEY / LISTEN_ADDR
run: build
	./bin/server

test:
	go test ./... -race

web-dev:
	cd web && npm run dev
