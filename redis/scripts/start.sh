#!/usr/bin/env bash
# 启动 lockx/redis（分布式锁 Redis） 独立环境
set -euo pipefail
cd "$(dirname "$0")/.."

# docker CLI 未必在 PATH 里：Docker Desktop 装在 /Applications 时，
# CLI 实际位于 /usr/local/bin/docker，而该路径常被精简掉。
DOCKER="$(command -v docker || true)"
if [ -z "$DOCKER" ] && [ -x /usr/local/bin/docker ]; then
  DOCKER=/usr/local/bin/docker
fi
if [ -z "$DOCKER" ]; then
  echo "ERROR: 未找到 docker CLI。请安装 Docker，或用 DOCKER=/path/to/docker $0 指定。"
  exit 1
fi

# 主端口可用环境变量覆盖：HOST_PORT=16379 ./start.sh
# （驱动测试与示例硬编码标准端口，故默认仍是标准端口）
HOST_PORT="${HOST_PORT:-6379}"
export HOST_PORT

# 宿主端口预检：先探再起，避免"起了才发现端口被占"留下半起状态。
# 本机常见占用来源：SDK 根 compose、hubx-micro-apps 六件套、dockers 通用中间件。
for p in 6379; do
  if (exec 3<>"/dev/tcp/127.0.0.1/${p}") 2>/dev/null; then
    occ="$("$DOCKER" ps --format '{{.Names}}\t{{.Ports}}' 2>/dev/null | grep -E "[.:]${p}->" | awk -F'\t' '{{print $1}}' | head -1)"
    echo "ERROR: 宿主端口 ${p} 已被占用${occ:+（容器: $occ)}。"
    echo "       要么停掉占用方，要么换宿主端口：HOST_PORT=<空闲端口> $0"
    echo "       查占用方: $DOCKER ps --format '{{.Names}}\t{{.Ports}}' | grep ${p}"
    exit 1
  fi
done

echo "Starting lockx/redis（分布式锁 Redis）..."
"$DOCKER" compose up -d

echo "Waiting for Redis to be healthy (up to 120s)..."
for i in $(seq 1 $(( 120 / 2 ))); do
  st="$("$DOCKER" inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' hubx-lockx-redis 2>/dev/null || echo missing)"
  if [ "$st" = "healthy" ]; then
    echo "Redis is ready: 127.0.0.1:6379（密码 redis123456）"
    exit 0
  fi
  sleep 2
done

echo "ERROR: Redis 在 120s 内未变为 healthy，最后状态: ${st:-unknown}"
echo "       查看日志: $DOCKER compose logs redis"
exit 1
