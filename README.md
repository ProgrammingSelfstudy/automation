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

# 填一个登录密码（至少 8 位），换成你自己想要的密码
sed -i '' "s|^BOOTSTRAP_ADMIN_PASSWORD=.*|BOOTSTRAP_ADMIN_PASSWORD=你自己的密码|" .env

make db-up  
# 起本地 MySQL，首次启动自动建好所有表
make run                   # 编译并启动后端，监听 :8080


newuser
SuperSecret123

```

> 也可以不用上面这两条 `sed`，手动打开 `.env` 把 `make key` 打印出来的值粘进 `ACCOUNT_ENCRYPTION_KEY=` 后面、把密码填进 `BOOTSTRAP_ADMIN_PASSWORD=` 后面——`make run` 报 `ACCOUNT_ENCRYPTION_KEY is required` 或 `BOOTSTRAP_ADMIN_PASSWORD is required` 就是这两项没填。

另开一个终端起前端：

```bash
cd web
cp .env.example .env           # 默认 VITE_API_BASE_URL=http://localhost:8080，本地一般不用改
npm install
npm run dev                    # 监听 :5173
```

浏览器打开 `http://localhost:5173`。

## 首次登录

后端启动时会用 `.env` 里的 `BOOTSTRAP_ADMIN_USERNAME`/`BOOTSTRAP_ADMIN_PASSWORD` 幂等创建一个初始账号（已存在就跳过，不会重复创建，也不会覆盖）。这个平台强制账号密码 + TOTP 二次验证，没有跳过 2FA 的入口：

1. 用 bootstrap 的用户名密码登录，系统会提示还没设置 2FA
2. 页面展示二维码，用 Google Authenticator / Authy 之类的 App 扫码
3. 输入 App 生成的 6 位验证码确认
4. 页面会展示一批**一次性**备用恢复码——现在就保存好，之后只有这一次能看到明文，用来应对手机丢失/换机的情况

登录之后可以在页面右上角"备用码"入口随时作废旧码、生成新的一批。

新增登录用户没有单独的注册页面，走 `POST /api/auth/users`（需要已登录），或者在 `.env` 改用户名重新跑一次 bootstrap（同一个用户名已存在的话不会重复创建）。

### 手动创建登录账号（不走 `BOOTSTRAP_ADMIN_PASSWORD`）

不想用环境变量走 bootstrap 的话，也可以直接往 `user` 表插一行。**不能直接写明文密码**——`password_hash` 这一列存的是 bcrypt 哈希，MySQL 没有内置函数能现算，所以先用仓库自带的小工具生成哈希：

```bash
go run ./cmd/hashpw "你的密码"
# 输出类似：$2a$10$B7/spSFupDgjcpqhshNVIudTirr7Xhc3dD/MhK2.uNe4pk806aT4m
```

再拿这个哈希拼 SQL（`id` 随便填一个 UUID，Linux/macOS 可以用 `uuidgen` 或 `python3 -c "import uuid; print(uuid.uuid4())"` 生成）：

```sql
INSERT INTO `user` (id, username, password_hash, totp_secret, totp_enabled)
VALUES (
  '<uuid>',
  'admin',
  '<go run ./cmd/hashpw 输出的哈希>',
  NULL,
  0
);
```

`totp_secret` 留 `NULL`、`totp_enabled` 留 `0`——这样插入之后，用这个用户名密码登录时系统照样会走"还没设置 2FA"的正常流程（扫码、备用码），不会因为是手动插入的就绕过 2FA。

连本地 docker-compose 起的 MySQL 执行这条 SQL：

```bash
docker exec -it interface-load-test-mysql-1 mysql -uroot -pdevpassword loadtest
```

（用户名密码对应 `.env` 里的 `MYSQL_ROOT_PASSWORD`/`MYSQL_DATABASE`，默认值就是 `devpassword`/`loadtest`）。

## 环境变量说明

`.env.example` 里都有注释，几个容易忽略的点单独说一下：

| 变量 | 说明 |
|---|---|
| `ACCOUNT_ENCRYPTION_KEY` | base64 编码的 32 字节 AES-256 key，用来加密存储的是"被测系统"的账号密码（不是登录密码）。`make key` 生成 |
| `BOOTSTRAP_ADMIN_USERNAME`/`PASSWORD` | 首次启动创建的登录账号，只在这个用户名不存在时生效 |
| `COOKIE_SECURE` | 本地开发（`localhost:5173` ↔ `localhost:8080`）保持 `false` 就行——两个 `localhost` 端口在 SameSite cookie 的判定里算"同站"，不需要 HTTPS。真要跨域名部署（前后端不同域名）才需要设 `true`，同时那种部署方式必须是 HTTPS |
| `ALLOWED_ORIGINS` | CORS 白名单，逗号分隔。前端换了端口/域名要记得加进来 |
| `HTTP_SHUTDOWN_TIMEOUT`/`TASK_SHUTDOWN_TIMEOUT` | 优雅停机超时（`time.ParseDuration` 格式，如 `10s`），不设就用代码里的默认值 |

## 常用命令

```bash
make db-up      # 起本地 MySQL
make db-down    # 停止但保留数据
make db-reset   # 停止并清空数据卷——改了 schema.sql 之后要用这个重新初始化
make db-logs    # 看 MySQL 日志
make run        # 编译并启动后端
make test       # go test ./... -race
make web-dev    # 启动前端 dev server

cd web && npm run build   # 前端类型检查 + 生产构建
cd web && npm run lint    # 前端 lint
```

## 关于数据库表结构

每个 `internal/*store` 包各自维护自己的 `schema.sql`，`docker-compose.yml` 把它们挂载进 MySQL 容器的 `docker-entrypoint-initdb.d/`，**只在数据卷第一次初始化时执行一次**。改了某个 `schema.sql` 之后，本地已有的容器不会自动应用变更，需要 `make db-reset` 清空数据卷重新初始化（会丢本地测试数据，正式环境不要这么干，正式环境的表结构变更需要手动写迁移）。

## 已知限制

- 优雅停机时 WebSocket 连接（`/ws/tasks/{id}/progress`）会被直接切断，不是优雅关闭——Go 标准库的 `http.Server.Shutdown` 不追踪已升级的 WS 连接
- 结果查询接口（`GET /api/tasks/{id}/results`）目前不分页，单任务结果量很大时会全量返回
- 暴力破解限流目前只覆盖登录接口，按来源 IP 记，`internal/authstore` 的 `login_attempt` 表没有过期清理，记录会一直保留（不影响功能，只是会占用一些存储空间）
