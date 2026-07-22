# 个人部署分支说明

本分支用于在个人 fork 中保存部署相关文件，同时让 `main` 尽量保持为官方上游的镜像。

## 分支职责

- `main`：跟随官方 `origin/main`，不放个人部署修改。
- `deploy/personal`：基于 `main`，额外保存个人部署脚本和 Docker Compose override。

## 日常同步（同步机）

仅限配置了 `origin`(官方上游) + `fork`(个人 fork) 双 remote 的机器，在 `deploy/personal` 分支运行：

```bash
./sync.sh
```

脚本会先检查工作区是否干净，再快进本地 `main` 到官方 `origin/main`，然后把 `deploy/personal` rebase 到最新 `main`，最后推送到个人 fork。

## 日常更新（部署机）

仅限 `origin` 直接指向个人 fork 的部署机（例如服务器），在 `deploy/personal` 分支运行：

```bash
./update.sh
```

`deploy/personal` 在同步机上会被 rebase 后强推，历史会变化，所以脚本不用 `git pull`，而是拉取 fork 最新代码后比对内容决定是否直接对齐（`reset --hard`）；如果本机独有的提交涉及的文件与远程新历史内容不一致（可能是本机专属修改），脚本会停下来提示人工检查，不会静默覆盖。对齐后会自动调用 `rebuild.sh` 重新部署。

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

