# 接口压测平台

多账号并发压测平台：接口编排 + 变量提取 + 公式计算，按账号分组存储结果、按账号分 sheet 导出 Excel。技术方案见 [docs/design.md](docs/design.md)。

- 后端：Go + MySQL（`cmd/server`，业务代码在 `internal/`）
- 前端：Vite + React + TypeScript（`web/`）
- 本地开发环境：docker-compose 起 MySQL

## 前置依赖

- Go（版本见 [go.mod](go.mod)）
- Node.js 20+
- Docker（起本地 MySQL 用）

## 快速开始

```bash
cp .env.example .env

# 生成 key 并直接写进 .env（macOS/BSD sed；Linux 把 sed -i '' 换成 sed -i）
KEY=$(make key)
sed -i '' "s|^ACCOUNT_ENCRYPTION_KEY=.*|ACCOUNT_ENCRYPTION_KEY=$KEY|" .env

# 推荐用 cmd/enrolluser 手动创建已配置 2FA 的登录账号，清空 bootstrap
sed -i '' "s|^BOOTSTRAP_ADMIN_USERNAME=.*|BOOTSTRAP_ADMIN_USERNAME=|" .env

make db-up
# 起本地 MySQL，首次启动自动建好所有表

# 生成登录账号所需的二维码、备用码和 SQL；按输出提示先保存备用码，再复制 SQL
go run ./cmd/enrolluser admin "你自己的密码"

# 打开 MySQL，把上一步最后打印的 SQL 粘进去执行
docker exec -it interface-load-test-mysql-1 mysql -uroot -pdevpassword loadtest

./scripts/start-server.sh  # 编译并启动后端，监听 :8080（Ctrl+C 走优雅停机）
```

> 也可以不用上面的 `sed`——`ACCOUNT_ENCRYPTION_KEY`/`BOOTSTRAP_ADMIN_USERNAME` 留空的话 `scripts/start-server.sh` 会自己补：key 是空的就现场生成一个写进 `.env`，`BOOTSTRAP_ADMIN_USERNAME` 填了但密码是空的会自动清空跳过 bootstrap。

另开一个终端起前端：

```bash
cd web
cp .env.example .env           # 默认 VITE_API_BASE_URL=http://localhost:8080，本地一般不用改
npm install
npm run dev                    # 监听 :5173
```

浏览器打开 `http://localhost:5173`。

## 首次登录

这个平台强制账号密码 + TOTP 二次验证，没有跳过 2FA 的入口。登录账号建议在创建时就用 `cmd/enrolluser` 把 TOTP 密钥、二维码和备用码准备好：

1. 运行 `go run ./cmd/enrolluser <用户名> <密码>`
2. 用 Google Authenticator / Authy 之类的 App 扫终端二维码
3. 保存终端打印的 10 个**一次性**备用恢复码，之后只有这一次能看到明文
4. 把最后打印的 SQL 粘进 MySQL 执行
5. 打开前端，用账号密码 + App 生成的 6 位验证码登录

登录之后可以在页面右上角"备用码"入口随时作废旧码、生成新的一批。

前端登录页不再生成二维码；如果某个账号还没有配置 2FA，页面会提示联系管理员创建。`POST /api/auth/totp/setup` / `POST /api/auth/totp/confirm` 和 bootstrap 创建账号的机制仍保留，但这类账号需要额外补完 TOTP 后才能通过前端登录。

### 手动创建登录账号（不走 `BOOTSTRAP_ADMIN_PASSWORD`）

直接运行仓库自带的小工具：

```bash
go run ./cmd/enrolluser <用户名> <密码>
```

跑完之后终端会依次打印：二维码（用认证器 App 扫）→ otpauth 链接/密钥（扫不了时手动输入用）→ 10 个备用码（只显示这一次，请立刻保存）→ 最后是可以直接复制粘贴执行的 SQL。SQL 里已经包含 `user.id`、bcrypt 密码哈希、TOTP secret 和 10 个备用码哈希，`totp_enabled` 会直接写成 `1`，形状大致是这样（把下面的 `<...>` 换成终端实际打印出来的值，10 个备用码哈希要跟终端打印的顺序一一对应）：

```sql
INSERT INTO `user` (id, username, password_hash, totp_secret, totp_enabled)
VALUES ('<uuid>', '<用户名>', '<bcrypt 密码哈希>', '<TOTP secret>', 1);

INSERT INTO backup_code (user_id, code_hash) VALUES
  ('<uuid>', '<备用码哈希 1>'),
  ('<uuid>', '<备用码哈希 2>'),
  ...
  ('<uuid>', '<备用码哈希 10>');
```

连本地 docker-compose 起的 MySQL 执行这条 SQL：

```bash
docker exec -it interface-load-test-mysql-1 mysql -uroot -pdevpassword loadtest
```

（用户名密码对应 `.env` 里的 `MYSQL_ROOT_PASSWORD`/`MYSQL_DATABASE`，默认值就是 `devpassword`/`loadtest`）。

`cmd/hashpw` 仍然保留，适合只想单独改某个已有账号密码、不动 TOTP 的时候生成新的 bcrypt 哈希。

## 环境变量说明

`.env.example` 里都有注释，几个容易忽略的点单独说一下：

| 变量 | 说明 |
|---|---|
| `ACCOUNT_ENCRYPTION_KEY` | base64 编码的 32 字节 AES-256 key，用来加密存储的是"被测系统"的账号密码（不是登录密码）。`make key` 生成 |
| `BOOTSTRAP_ADMIN_USERNAME`/`PASSWORD` | 旧 bootstrap 入口，只在用户名不存在时创建未预配置 TOTP 的账号；新账号推荐用 `cmd/enrolluser` |
| `COOKIE_SECURE` | 本地开发（`localhost:5173` ↔ `localhost:8080`）保持 `false` 就行——两个 `localhost` 端口在 SameSite cookie 的判定里算"同站"，不需要 HTTPS。真要跨域名部署（前后端不同域名）才需要设 `true`，同时那种部署方式必须是 HTTPS |
| `ALLOWED_ORIGINS` | CORS 白名单，逗号分隔。前端换了端口/域名要记得加进来 |
| `HTTP_SHUTDOWN_TIMEOUT`/`TASK_SHUTDOWN_TIMEOUT` | 优雅停机超时（`time.ParseDuration` 格式，如 `10s`），不设就用代码里的默认值 |

## 常用命令

```bash
make db-up      # 起本地 MySQL
make db-down    # 停止但保留数据
make db-reset   # 停止并清空数据卷——改了 schema.sql 之后要用这个重新初始化
make db-logs    # 看 MySQL 日志
make test       # go test ./... -race
make web-dev    # 启动前端 dev server

./scripts/start-server.sh # 编译并启动后端，监听 :8080（Ctrl+C 走优雅停机，见下方说明）

cd web && npm run build   # 前端类型检查 + 生产构建
cd web && npm run lint    # 前端 lint
```

> 启动后端不要用 `make run`：这台机器上 Xcode 自带的 `make` 不会把 `Ctrl+C`/`SIGTERM` 转发给它起的子进程 `./bin/server`，优雅停机代码根本收不到信号。`scripts/start-server.sh` 会补全 `.env` 缺的项、等 MySQL healthy、编译，再用 `exec` 把自己替换成 `./bin/server`，这样信号能直接送到位。`make run` 还留着，纯粹给不关心优雅关闭、只想跑一下看看的场景用。

## 关于数据库表结构

每个 `internal/*store` 包各自维护自己的 `schema.sql`，`docker-compose.yml` 把它们挂载进 MySQL 容器的 `docker-entrypoint-initdb.d/`，**只在数据卷第一次初始化时执行一次**。改了某个 `schema.sql` 之后，本地已有的容器不会自动应用变更，需要 `make db-reset` 清空数据卷重新初始化（会丢本地测试数据，正式环境不要这么干，正式环境的表结构变更需要手动写迁移）。

## 已知限制

- 优雅停机时 WebSocket 连接（`/ws/tasks/{id}/progress`）会被直接切断，不是优雅关闭——Go 标准库的 `http.Server.Shutdown` 不追踪已升级的 WS 连接
- 结果查询接口（`GET /api/tasks/{id}/results`）目前不分页，单任务结果量很大时会全量返回
- 暴力破解限流目前只覆盖登录接口，按来源 IP 记，`internal/authstore` 的 `login_attempt` 表没有过期清理，记录会一直保留（不影响功能，只是会占用一些存储空间）
