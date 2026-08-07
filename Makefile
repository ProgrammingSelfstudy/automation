ifneq (,$(wildcard ./.env))
include .env
export
endif

.PHONY: db-up db-down db-reset db-logs key build run test web-dev perf-agent-build

## 起本地 MySQL（首次启动会自动跑各包的 schema.sql）
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

## 交叉编译本地采集 Agent（perf-rabbit/client/cmd/main.go——无嵌入前端、
## 不自动开浏览器的后端调试入口，不是 cmd/app 那个独立产品）到
## assets/perf-agent/，供 /api/perf/agent/downloads 下载接口读取。
## 部署时把这个目录下的产物 scp 到服务器上 PERF_AGENT_ASSETS_DIR 指向的位置。
perf-agent-build:
	mkdir -p assets/perf-agent
	cd perf-rabbit/client && GOOS=darwin GOARCH=arm64 go build -o ../../assets/perf-agent/perf-agent-darwin-arm64 ./cmd/main.go
	cd perf-rabbit/client && GOOS=darwin GOARCH=amd64 go build -o ../../assets/perf-agent/perf-agent-darwin-amd64 ./cmd/main.go
	cd perf-rabbit/client && GOOS=windows GOARCH=amd64 go build -o ../../assets/perf-agent/perf-agent-windows-amd64.exe ./cmd/main.go
