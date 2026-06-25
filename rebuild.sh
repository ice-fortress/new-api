#!/usr/bin/env bash
# rebuild.sh — 拉取最新 Docker 镜像并重新部署
#
# 职责：检查 compose 配置 → 创建本地目录 → pull 镜像 → up -d 重启 → 清理旧镜像 → 打印状态
# 不涉及 git，与 sync.sh 互相独立，便于单独排查卡点。
# 用法：./rebuild.sh

set -euo pipefail

if [ ! -f "docker-compose.yml" ]; then
    echo "请在仓库根目录运行 rebuild.sh"
    exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
    echo "当前环境不可用 docker compose，请先安装或启用 Docker Compose v2"
    exit 1
fi

echo "==> 创建本地数据目录..."
mkdir -p \
    new-api-volumes/new-api/data \
    new-api-volumes/redis/data \
    new-api-volumes/postgres/data \
    logs

echo ""
echo "==> 校验 Docker Compose 配置..."
docker compose config >/dev/null

echo ""
echo "==> 拉取最新 Docker 镜像..."
docker compose pull

echo ""
echo "==> 重新部署..."
docker compose up -d --remove-orphans

echo ""
echo "==> 清理旧镜像..."
docker image prune -f

echo ""
echo "==> 部署完成！"
docker compose ps
