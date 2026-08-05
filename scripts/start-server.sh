#!/usr/bin/env bash
# 起本地后端：补全 .env 里必填的项、起 MySQL、等它 healthy、编译并启动 cmd/server。
# 前台运行（不是后台），Ctrl+C 走的就是优雅停机那条路径。
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

if [ ! -f .env ]; then
  echo "没有 .env，从 .env.example 复制一份"
  cp .env.example .env
fi

# ACCOUNT_ENCRYPTION_KEY 是硬性必填项（没有服务起不来），空的话现场生成一个写进去。
current_key=$(grep -E '^ACCOUNT_ENCRYPTION_KEY=' .env | cut -d= -f2-)
if [ -z "$current_key" ]; then
  echo "生成 ACCOUNT_ENCRYPTION_KEY 并写入 .env"
  key=$(openssl rand -base64 32)
  sed -i '' "s|^ACCOUNT_ENCRYPTION_KEY=.*|ACCOUNT_ENCRYPTION_KEY=$key|" .env
fi

# BOOTSTRAP_ADMIN_USERNAME 填了但密码没填的话，mustEnv 会直接拒绝启动——
# 与其到那一步才报错，这里先兜住：没填密码就干脆把用户名也清空，跳过 bootstrap
# （账号走手动插库那条路的话本来就用不上这个机制，参考 README「手动创建登录账号」）。
bootstrap_user=$(grep -E '^BOOTSTRAP_ADMIN_USERNAME=' .env | cut -d= -f2-)
bootstrap_pass=$(grep -E '^BOOTSTRAP_ADMIN_PASSWORD=' .env | cut -d= -f2-)
if [ -n "$bootstrap_user" ] && [ -z "$bootstrap_pass" ]; then
  echo "BOOTSTRAP_ADMIN_USERNAME 填了但 BOOTSTRAP_ADMIN_PASSWORD 是空的，跳过 bootstrap（清空 BOOTSTRAP_ADMIN_USERNAME）"
  sed -i '' "s|^BOOTSTRAP_ADMIN_USERNAME=.*|BOOTSTRAP_ADMIN_USERNAME=|" .env
fi

echo "起 MySQL..."
docker compose up -d mysql

echo -n "等 MySQL healthy"
for _ in $(seq 1 30); do
  status=$(docker inspect --format='{{.State.Health.Status}}' interface-load-test-mysql-1 2>/dev/null || echo "")
  if [ "$status" = "healthy" ]; then
    echo " 好了"
    break
  fi
  echo -n "."
  sleep 2
done
if [ "$status" != "healthy" ]; then
  echo
  echo "MySQL 30 次轮询之后还没 healthy，自己 docker compose logs mysql 看一下" >&2
  exit 1
fi

echo "编译后端..."
go build -o bin/server ./cmd/server

# 不走 `make run`：`make run` 是 make 起子进程跑 ./bin/server，Ctrl+C 发的信号
# 到 make 这层不一定会转发给子进程（实测这台机器上 Xcode 自带的 make 就不转发），
# 优雅停机代码根本收不到信号。这里直接把 .env 载进当前 shell 的环境，
# 再用 exec 替换掉本脚本进程本身——之后 Ctrl+C/SIGTERM 发给的就是 ./bin/server 自己。
#
# 不能直接 `source .env`：.env 里 MYSQL_DSN 这类值带未加引号的括号
# （tcp(127.0.0.1:3306)），bash 会把它当语法解析直接报错（zsh 比较宽容,
# 交互终端里手动 source 感觉没事,但脚本跑在真 bash 下会炸）。
# 也不能用 `IFS='=' read -r key value`：base64 密钥结尾的 `=`（padding）
# 会被当成末尾分隔符吞掉，密钥少一位。改用字符串截取，VALUE 全程只是
# 数据，既不会被当 shell 代码解析，也不会丢末尾字符。
set -a
while IFS= read -r line || [[ -n "$line" ]]; do
  [[ -z "$line" || "$line" == \#* ]] && continue
  key="${line%%=*}"
  value="${line#*=}"
  export "$key=$value"
done < .env
set +a

echo "启动后端（Ctrl+C 停止，会走优雅停机）"
exec ./bin/server
