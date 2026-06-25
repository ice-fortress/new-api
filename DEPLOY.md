# 个人部署分支说明

本分支用于在个人 fork 中保存部署相关文件，同时让 `main` 尽量保持为官方上游的镜像。

## 分支职责

- `main`：跟随官方 `origin/main`，不放个人部署修改。
- `deploy/personal`：基于 `main`，额外保存个人部署脚本和 Docker Compose override。

## 日常同步

在 `deploy/personal` 分支运行：

```bash
./sync.sh
```

脚本会先检查工作区是否干净，再快进本地 `main` 到官方 `origin/main`，然后把 `deploy/personal` rebase 到最新 `main`，最后推送到个人 fork。

## 重新部署

```bash
./rebuild.sh
```

脚本会创建本地数据目录、校验 Docker Compose 配置、拉取镜像并重新启动服务。

## 本地运行数据

以下目录只保存本机运行数据，不应提交到 Git：

- `new-api-volumes/`
- `logs/`
- `.claude/`

