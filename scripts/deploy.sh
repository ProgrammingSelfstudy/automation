#!/usr/bin/env bash
# 部署到服务器：后端（ssh 上去 git pull + go build + systemctl restart）
# + 前端（本地用生产环境变量 build，scp 上去原子替换 dist 目录）。
#
# 前提：
# - 服务器已经按 docs/deploy-centos7.md 搭好（systemd 服务、nginx、.env 等）
# - 本机已经 ssh-copy-id 过，SSH 密钥登录，脚本全程不用密码
#   （不要把密码写进这个脚本或者 deploy.env——一旦这类文件不小心被提交，
#   密码就永久留在 git 历史里了，删了文件也删不掉历史记录）
# - scripts/deploy.env（从 deploy.env.example 复制）配好 SERVER 等变量
# - web/.env.production 配好生产环境的 VITE_API_BASE_URL（跟本地开发用的
#   web/.env 分开，构建时临时换上、构建完立刻换回来，不影响本地 npm run dev）
#
# 用法：
#   scripts/deploy.sh              # 后端 + 前端都部署
#   SKIP_FRONTEND=1 scripts/deploy.sh   # 只部署后端
#   SKIP_BACKEND=1 scripts/deploy.sh    # 只部署前端
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

if [ -f "$SCRIPT_DIR/deploy.env" ]; then
  # shellcheck disable=SC1091
  source "$SCRIPT_DIR/deploy.env"
fi

SERVER="${SERVER:?没配置服务器地址：复制 scripts/deploy.env.example 成 scripts/deploy.env 再改成你自己的值}"
APP_DIR="${APP_DIR:-/opt/interface-load-test/app}"
BIN_PATH="${BIN_PATH:-/opt/interface-load-test/bin/server}"
SERVICE_NAME="${SERVICE_NAME:-interface-load-test}"
APP_USER="${APP_USER:-interface-load-test}"
SKIP_FRONTEND="${SKIP_FRONTEND:-0}"
SKIP_BACKEND="${SKIP_BACKEND:-0}"

if [ "$SKIP_BACKEND" != "1" ]; then
  echo "==> 部署后端：git pull + go build（在服务器上，Go 环境已经在那）"
  # systemctl restart 就是这里说的"杀掉进程"——用它而不是裸 kill/pkill，
  # 因为后端实现了收到 SIGTERM 的优雅停机（先关 HTTP、等在跑的压测任务
  # 收尾），systemctl restart 发的正是 SIGTERM，裸 kill -9 会跳过这段逻辑。
  # su -c 里单独跑的是另一个 bash 实例，外层这个 ssh 命令的 set -e 管不到它——
  # 之前漏了内层的 set -e，git pull 网络抖动失败了都会被 go build（用的是
  # 没更新的旧代码）的成功状态盖过去，整个部署"看起来成功"实际没更新代码。
  ssh "$SERVER" "
    set -e
    su - '$APP_USER' -s /bin/bash -c '
      set -e
      cd $APP_DIR
      git pull
      go build -o $BIN_PATH ./cmd/server
    '
    systemctl restart $SERVICE_NAME
    sleep 1
    systemctl is-active $SERVICE_NAME
  "
  echo "==> 后端已重启"
fi

if [ "$SKIP_FRONTEND" != "1" ]; then
  echo "==> 构建前端（生产环境变量）"
  cd "$REPO_ROOT/web"
  if [ ! -f .env.production ]; then
    echo "缺少 web/.env.production（生产环境的 VITE_API_BASE_URL），跳过前端部署。" >&2
    echo "写一份：echo 'VITE_API_BASE_URL=https://your-domain:port' > web/.env.production" >&2
    exit 1
  fi

  # 构建要用的是 .env.production，不是本地开发的 .env——临时换上、构建完
  # 立刻换回来，不能让这次部署顺带把本机 npm run dev 指向生产环境。
  dev_env_backup=""
  if [ -f .env ]; then
    dev_env_backup="$(mktemp)"
    cp .env "$dev_env_backup"
  fi
  cp .env.production .env
  restore_dev_env() {
    if [ -n "$dev_env_backup" ]; then
      mv "$dev_env_backup" .env
    else
      rm -f .env
    fi
  }
  trap restore_dev_env EXIT

  npm run build
  restore_dev_env
  trap - EXIT

  echo "==> 上传前端产物（先传到 dist_new，原子替换，避免半上传状态被用户请求到）"
  ssh "$SERVER" "rm -rf $APP_DIR/web/dist_new"
  scp -r dist "$SERVER:$APP_DIR/web/dist_new"
  ssh "$SERVER" "
    set -e
    cd $APP_DIR/web
    rm -rf dist_old
    mv dist dist_old
    mv dist_new dist
    chown -R $APP_USER:$APP_USER dist
    rm -rf dist_old
  "
  echo "==> 前端已部署（nginx 直接读新文件，不用重启）"
fi

echo "==> 部署完成"
