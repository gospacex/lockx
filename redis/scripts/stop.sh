#!/usr/bin/env bash
# 停止并清理 lockx/redis（分布式锁 Redis） 独立环境（数据卷保留在 ./data，如需连数据一起删用 stop.sh --volumes）
set -euo pipefail
cd "$(dirname "$0")/.."

DOCKER="$(command -v docker || true)"
if [ -z "$DOCKER" ] && [ -x /usr/local/bin/docker ]; then
  DOCKER=/usr/local/bin/docker
fi
if [ -z "$DOCKER" ]; then
  echo "ERROR: 未找到 docker CLI。"
  exit 1
fi

if [ "${1:-}" = "--volumes" ]; then
  echo "Stopping lockx/redis（分布式锁 Redis） and removing volumes..."
  "$DOCKER" compose down -v --remove-orphans
else
  echo "Stopping lockx/redis（分布式锁 Redis）..."
  "$DOCKER" compose down --remove-orphans
fi
echo "Done."
