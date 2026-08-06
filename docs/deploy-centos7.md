# 部署文档：CentOS 7.6 (64-bit) 公网部署

## 架构

```
浏览器 → nginx (80/443，唯一公网入口)
           ├─ /            → 前端静态文件（web/dist）
           ├─ /api/*       → 反向代理到 127.0.0.1:8080（后端 Go 服务）
           └─ /ws/*        → 反向代理到 127.0.0.1:8080（WebSocket，走进度推送）
                                  │
                                  ▼
                          后端 Go 服务（只监听 127.0.0.1，不直接暴露公网）
                                  │
                                  ▼
                              MySQL 8.0（本机）
```

前端和后端放在同一个域名/端口下（nginx 统一入口，反代 `/api`、`/ws`），浏览器发出的请求全部是同源请求——不用配置 CORS，Cookie 也不用处理跨站 SameSite 问题，比本地开发时前后端分跑在 5173/8080 两个端口简单。

本文档只覆盖部署这台服务器本身的步骤，不包含域名解析、CDN、云厂商安全组这些外部配置——那些请按你实际用的服务商自行操作，本文档里提到的地方会标注需要提前确认好。

## 0. 前置条件

- CentOS 7.6 (64-bit) 服务器一台，能用 root 或有 sudo 权限的账号登录
- 云服务商安全组（如果是云主机）已经放行 80、443（如果用 HTTPS）
- 如果要上 HTTPS：一个已经解析到这台服务器公网 IP 的域名。没有域名也能部署，只是只能用 HTTP + IP 访问，跳过第 12 步即可

## 1. 系统更新与基础工具

```bash
yum update -y
yum install -y wget git firewalld
systemctl enable firewalld --now
```

## 2. 安装 Go

CentOS 7 yum 源里的 Go 版本太老，这个项目 `go.mod` 要求 `go 1.25.10`，直接装官方二进制包：

```bash
cd /tmp
wget https://go.dev/dl/go1.25.10.linux-amd64.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf go1.25.10.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
source /etc/profile.d/go.sh
go version   # 应该打印 go1.25.10
```

## 3. 安装 Node.js（编译前端用）

```bash
curl -fsSL https://rpm.nodesource.com/setup_20.x | bash -
yum install -y nodejs
node --version   # 20.x 即可，本地开发用的是 22.x，20 LTS 编译这个项目没问题
```

## 4. 安装 MySQL 8.0

CentOS 7 默认装的是 MariaDB，`internal/environmentstore` 这张表用了 `JSON` 列类型（MariaDB 10.2+ 也支持，但为了跟本地开发环境用的镜像一致，装官方 MySQL 8.0）：

```bash
yum install -y https://dev.mysql.com/get/mysql80-community-release-el7-9.noarch.rpm
yum install -y mysql-community-server
systemctl enable mysqld --now

# 首次启动会在日志里生成一个随机初始密码
grep 'temporary password' /var/log/mysqld.log

# 用这个初始密码登录后强制改密码 + 关掉一些不安全的默认配置
mysql_secure_installation
```

建库、建一个专门跑这个应用的账号（不要让应用直接用 root 连库）：

```bash
mysql -uroot -p
```
```sql
CREATE DATABASE loadtest CHARACTER SET utf8mb4;
CREATE USER 'loadtest'@'localhost' IDENTIFIED BY '换成你自己的强密码';
GRANT ALL PRIVILEGES ON loadtest.* TO 'loadtest'@'localhost';
FLUSH PRIVILEGES;
EXIT;
```

## 5. 建应用专用系统用户和目录

不要用 root 直接跑后端服务：

```bash
useradd --system --home-dir /opt/interface-load-test --shell /sbin/nologin interface-load-test
mkdir -p /opt/interface-load-test /var/log/interface-load-test
chown -R interface-load-test:interface-load-test /opt/interface-load-test /var/log/interface-load-test
```

## 6. 拉代码、编译后端

```bash
su - interface-load-test -s /bin/bash
cd /opt/interface-load-test
git clone https://github.com/ProgrammingSelfstudy/automation.git app
cd app
go build -o /opt/interface-load-test/bin/server ./cmd/server
exit   # 退回 root，后面装系统服务、配 nginx 需要 root 权限
```

## 7. 初始化数据库表结构

每个 `internal/*store` 包各自维护自己的 `schema.sql`，本地开发是靠 docker-compose 挂载自动执行；生产环境没有这个机制，手动按顺序执行一遍（顺序参考 `docker-compose.yml` 里的文件编号）：

```bash
cd /opt/interface-load-test/app
for f in \
  internal/accountstore/schema.sql \
  internal/resultstore/schema.sql \
  internal/taskmanager/schema.sql \
  internal/scenariostore/schema.sql \
  internal/authstore/schema.sql \
  internal/interfacestore/schema.sql \
  internal/environmentstore/schema.sql \
; do
  echo "applying $f"
  mysql -uloadtest -p loadtest < "$f"
done
```

以后代码更新加了新的 `schema.sql`（比如以后新加一个 `internal/xxxstore`），照这个方式手动补一条 `mysql -uloadtest -p loadtest < 新文件路径` 就行；改了已有表结构要手写 `ALTER TABLE`（生产环境不能像本地 `make db-reset` 那样直接清库重建）。

## 8. 配置 `.env`

```bash
cp /opt/interface-load-test/app/.env.example /opt/interface-load-test/.env
vi /opt/interface-load-test/.env
```

改成生产值：

```bash
MYSQL_DSN=loadtest:你在第4步设的数据库密码@tcp(127.0.0.1:3306)/loadtest?parseTime=true

# 生成一个新的，不要沿用本地开发用的那个：
# openssl rand -base64 32
ACCOUNT_ENCRYPTION_KEY=<生成的值>

# 只监听本地，公网只能通过 nginx 反代访问，不要写成 :8080（那样等于直接暴露公网端口）
LISTEN_ADDR=127.0.0.1:8080

# 前后端同域走 nginx 反代，浏览器不会把这当跨域请求，CORS 允许列表这里
# 填不填其实都不影响功能——但注意 cmd/server 里 ALLOWED_ORIGINS 留空会
# 触发默认值 fallback 成 http://localhost:5173（envOrDefault 把空字符串
# 当成未设置），不会是真的空列表。为了配置清楚、不留一个没用的 localhost
# 兜底值，直接写你自己的域名：
ALLOWED_ORIGINS=https://your-domain.com

# 留空，账号走 cmd/enrolluser 手动建（见第 9 步）
BOOTSTRAP_ADMIN_USERNAME=
BOOTSTRAP_ADMIN_PASSWORD=

# 生产用 HTTPS 才能设 true；如果这台服务器暂时只能用 HTTP + IP 访问（没有域名/证书），
# 先保持 false，等第 12 步配好 HTTPS 之后再回来改成 true 并重启服务
COOKIE_SECURE=false

HTTP_SHUTDOWN_TIMEOUT=10s
TASK_SHUTDOWN_TIMEOUT=30s
```

```bash
chown interface-load-test:interface-load-test /opt/interface-load-test/.env
chmod 600 /opt/interface-load-test/.env
```

## 9. 建第一个登录账号

```bash
su - interface-load-test -s /bin/bash
cd /opt/interface-load-test/app
go run ./cmd/enrolluser <你的用户名> <你的密码>
```

终端会依次打印二维码、otpauth 链接/密钥、10 个备用码、最后是可以直接执行的 SQL——照着 README「手动创建登录账号」那节操作：扫码/保存备用码，然后把打印出来的 SQL 粘进 `mysql -uloadtest -p loadtest` 执行。执行完 `exit` 退回 root。

## 10. systemd 服务

`/etc/systemd/system/interface-load-test.service`：

```ini
[Unit]
Description=接口压测平台后端
After=network.target mysqld.service

[Service]
Type=simple
User=interface-load-test
Group=interface-load-test
WorkingDirectory=/opt/interface-load-test
EnvironmentFile=/opt/interface-load-test/.env
ExecStart=/bin/bash -c '/opt/interface-load-test/bin/server >> /var/log/interface-load-test/server.log 2>&1'
Restart=on-failure
RestartSec=3
KillSignal=SIGTERM
TimeoutStopSec=40

[Install]
WantedBy=multi-user.target
```

几个非显而易见的点：

- `ExecStart` 用 `/bin/bash -c '... >> file 2>&1'` 做日志重定向，没用较新版本 systemd 才支持的 `StandardOutput=append:...`——CentOS 7.6 自带的 systemd 是 219，太老不支持这个写法，bash 重定向兼容性更好，任何 systemd 版本都能用。这样日志只进文件、不进 journald，`journalctl` 里看不到这个服务的输出是正常的，看文件就行
- `KillSignal=SIGTERM`：systemd 默认发的就是 SIGTERM，跟 `cmd/server` 里的优雅停机逻辑（收到 SIGTERM 先关 HTTP 再等在跑的压测任务收尾）对得上，不用改
- `TimeoutStopSec=40`：要留够 `HTTP_SHUTDOWN_TIMEOUT`（10s）+ `TASK_SHUTDOWN_TIMEOUT`（30s）的余量，不然 systemd 会在优雅停机跑完之前就直接 SIGKILL，等于白配了优雅停机

```bash
systemctl daemon-reload
systemctl enable interface-load-test --now
systemctl status interface-load-test
tail -f /var/log/interface-load-test/server.log
```

看到 `listening on 127.0.0.1:8080` 就是起来了。

## 11. 后端日志切割（logrotate）

`/etc/logrotate.d/interface-load-test`：

```
/var/log/interface-load-test/server.log {
    daily
    rotate 14
    compress
    delaycompress
    missingok
    notifempty
    copytruncate
}
```

用 `copytruncate` 而不是默认的 move+create：后端进程通过 bash 重定向一直拿着这个文件的文件描述符在写，普通 logrotate 把文件移走再建一个新的，后端进程不会自动重新打开新文件（这个服务没实现收到信号后重开日志文件），会继续写在那个已经被移走、只是还没被回收的旧文件里，新文件永远是空的。`copytruncate` 是复制一份出来再把原文件截断清零，同一个 fd 继续写不受影响，最省事。

## 12. 编译前端、配置 nginx

```bash
yum install -y nginx
systemctl enable nginx --now
```

编译前端：

```bash
su - interface-load-test -s /bin/bash
cd /opt/interface-load-test/app/web
cp .env.example .env
vi .env
```

把 `VITE_API_BASE_URL` 改成这台服务器对外的完整地址（协议 + 域名 + 端口，前后端同域，就是下面 nginx `server_name`/`listen` 配的那个）：

```
VITE_API_BASE_URL=https://your-domain.com:9527
```

（还没上 HTTPS 就先写 `http://your-domain.com:9527` 或者 `http://服务器公网IP:9527`，等第 13 步配完证书后回来改成 https 再重新 `npm run build` 一次）

```bash
npm install
npm run build   # 产物在 web/dist
exit
```

nginx 配置 `/etc/nginx/conf.d/interface-load-test.conf`：

```nginx
server {
    listen 9527;
    server_name your-domain.com;   # 没有域名就写服务器的公网 IP

    root /opt/interface-load-test/app/web/dist;
    index index.html;

    access_log /var/log/nginx/interface-load-test.access.log;
    error_log  /var/log/nginx/interface-load-test.error.log;

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /ws/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 3600s;   # WS 长连接，默认超时太短会被 nginx 主动断开
    }

    # 前端是 SPA（React Router），刷新任意路由都要落到 index.html，交给前端路由处理
    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

```bash
nginx -t   # 检查配置语法
systemctl reload nginx
```

`/ws/` 那个 block 里 `proxy_set_header Upgrade`/`Connection "upgrade"` 这两行是 WebSocket 能不能连上的关键，漏了的话浏览器控制台会看到 WS 连接直接失败或者一直 pending。

用的是非标准端口 9527，不是 80/443，浏览器访问要带上端口：`http://your-domain.com:9527`（配完 HTTPS 之后是 `https://your-domain.com:9527`）——不写端口默认走的是 80/443，会连不上。

前端日志就是 `/var/log/nginx/interface-load-test.access.log`（每一次请求，包括页面加载和所有 `/api`、`/ws` 转发过去之前经过 nginx 这一层的记录）和 `interface-load-test.error.log`（nginx 自己的报错，比如后端连不上）——CentOS 的 nginx 包自带了 `/etc/logrotate.d/nginx`，会自动做日志切割，不用再额外配置。

## 13.（可选）HTTPS

前提：有一个域名已经解析到这台服务器的公网 IP，且第 12 步 `server_name` 填的是这个域名（不是 IP，Let's Encrypt 用 IP 签不了证书）。

**因为站点用的是 9527 这种非标准端口，不能直接用 `certbot --nginx` 一键搞定**——Let's Encrypt 默认的 HTTP-01 验证方式必须能通过标准的 80 端口访问到 `http://你的域名/.well-known/acme-challenge/...`，跟你实际网站监听哪个端口没关系，所以还是得单独留一个 80 端口专门应付证书验证：

```bash
yum install -y epel-release
yum install -y certbot

mkdir -p /var/www/certbot
```

在 `/etc/nginx/conf.d/interface-load-test.conf` 里现有的 `server { listen 9527; ... }` 上面，加一个专门处理证书验证的 80 端口 block（其它请求直接跳转到 9527，不承载业务）：

```nginx
server {
    listen 80;
    server_name your-domain.com;

    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }

    location / {
        return 301 https://$host:9527$request_uri;
    }
}
```

```bash
nginx -t && systemctl reload nginx

certbot certonly --webroot -w /var/www/certbot -d your-domain.com
```

跟着交互式提示走（邮箱、同意条款）。这条命令只签证书、不改 nginx 配置，签完之后手动把证书接进 9527 那个 `server` block（改成同时监听 9527 明文和 9527 走 ssl 不太合理，一般做法是把原来的 `listen 9527;` 改成 `listen 9527 ssl;` 直接强制 HTTPS）：

```nginx
server {
    listen 9527 ssl;
    server_name your-domain.com;

    ssl_certificate     /etc/letsencrypt/live/your-domain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;

    root /opt/interface-load-test/app/web/dist;
    # ... 其余 location /api/、location /ws/、location / 不变
}
```

```bash
nginx -t && systemctl reload nginx
```

证书 90 天到期，certbot 装的时候会自带一个自动续期的定时任务（`systemctl list-timers | grep certbot` 能看到），到期前会自动重新走一遍 80 端口那个 webroot 验证，不用手动管，只要 80 端口那个跳转 block 一直留着别删。

配完之后回去做两件事：

1. `.env` 里 `COOKIE_SECURE` 改成 `true`，`systemctl restart interface-load-test`
2. `web/.env` 里 `VITE_API_BASE_URL` 改成 `https://` 开头，重新 `npm run build` 一次

## 14. 防火墙

```bash
firewall-cmd --permanent --add-port=9527/tcp
firewall-cmd --permanent --add-service=http   # 只用来跳转 + 走证书验证，见第 13 步；完全不打算上 HTTPS 就不用开
firewall-cmd --reload
```

**不要**对公网开放 3306（MySQL）和 8080（后端监听的是 127.0.0.1，本身也没法从外面直接连）。

## 常用运维命令

```bash
# 后端
systemctl status interface-load-test
systemctl restart interface-load-test
tail -f /var/log/interface-load-test/server.log
journalctl -u interface-load-test -n 50   # 只能看到 systemd 自己的启动/退出事件，业务日志在上面那个文件里

# nginx / 前端
systemctl status nginx
nginx -t && systemctl reload nginx
tail -f /var/log/nginx/interface-load-test.access.log
tail -f /var/log/nginx/interface-load-test.error.log

# 更新部署（代码有更新之后）
su - interface-load-test -s /bin/bash
cd /opt/interface-load-test/app
git pull
go build -o /opt/interface-load-test/bin/server ./cmd/server
cd web && npm install && npm run build
exit
systemctl restart interface-load-test
# 前端是静态文件，nginx 直接读新的 web/dist，不用重启 nginx；如果改了 schema.sql 记得手动执行新增的建表/改表语句
```

## 已知限制

- 优雅停机时 WebSocket 连接会被直接切断，不是优雅关闭（Go 标准库 `http.Server.Shutdown` 不追踪已升级的 WS 连接），跟本地开发环境的限制一样
- 数据库表结构变更需要手动 `ALTER TABLE`，没有自动迁移机制——上线前确认清楚有没有新增/改动过的 `schema.sql`
- 本文档没有覆盖 MySQL 备份策略，生产环境建议至少配一个每日 `mysqldump` 定时任务，存到这台服务器之外的地方
