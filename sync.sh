#!/usr/bin/env bash
# sync.sh — 安全同步官方上游，并刷新个人部署分支
#
# 职责：fetch 官方上游 → 快进本地 main → rebase deploy/personal → 推送到 fork
# 不使用 reset --hard，避免误删个人部署分支上的已跟踪文件。
# 前提：请在 deploy/personal 分支运行，并先提交或暂存好本地修改。
# 用法：./sync.sh

set -euo pipefail

UPSTREAM_REMOTE="origin"
UPSTREAM_BRANCH="main"
FORK_REMOTE="fork"
BASE_BRANCH="main"
DEPLOY_BRANCH="deploy/personal"

echo "==> 检查当前分支..."
CURRENT_BRANCH=$(git branch --show-current)
if [ "$CURRENT_BRANCH" != "$DEPLOY_BRANCH" ]; then
    echo "    当前分支是 $CURRENT_BRANCH，请先切换到 $DEPLOY_BRANCH 后再同步"
    exit 1
fi

echo "==> 检查工作区状态..."
if ! git diff --quiet || ! git diff --cached --quiet; then
    echo "    工作区存在已跟踪文件修改，请先 commit 或 stash 后再同步"
    git status --short
    exit 1
fi

UNTRACKED_FILES=$(git ls-files --others --exclude-standard)
if [ -n "$UNTRACKED_FILES" ]; then
    echo "    工作区存在未跟踪文件，为避免切换或 rebase 时覆盖文件，请先处理："
    echo "$UNTRACKED_FILES"
    exit 1
fi

echo "==> 获取上游最新代码..."
git fetch "$UPSTREAM_REMOTE"

LOCAL_BASE=$(git rev-parse "$BASE_BRANCH")
REMOTE_BASE=$(git rev-parse "$UPSTREAM_REMOTE/$UPSTREAM_BRANCH")

echo "==> 更新本地 $BASE_BRANCH 到官方 $UPSTREAM_REMOTE/$UPSTREAM_BRANCH..."
if [ "$LOCAL_BASE" = "$REMOTE_BASE" ]; then
    echo "    $BASE_BRANCH 已经是官方最新"
elif git merge-base --is-ancestor "$BASE_BRANCH" "$UPSTREAM_REMOTE/$UPSTREAM_BRANCH"; then
    # 只允许快进 main；如果 main 有个人提交，下面的分支移动会被前面的祖先检查拦住。
    git branch -f "$BASE_BRANCH" "$UPSTREAM_REMOTE/$UPSTREAM_BRANCH"
    echo "    $BASE_BRANCH 已快进到 $REMOTE_BASE"
else
    echo "    本地 $BASE_BRANCH 与官方分支发生分叉，停止同步以避免覆盖本地提交"
    echo "    请先人工检查：git log --oneline --left-right $BASE_BRANCH...$UPSTREAM_REMOTE/$UPSTREAM_BRANCH"
    exit 1
fi

echo ""
echo "==> 推送官方镜像分支到 fork..."
git push "$FORK_REMOTE" "$BASE_BRANCH:$UPSTREAM_BRANCH"

echo ""
echo "==> 将 $DEPLOY_BRANCH rebase 到最新 $BASE_BRANCH..."
git rebase "$BASE_BRANCH"

echo ""
echo "==> 推送个人部署分支到 fork..."
git push "$FORK_REMOTE" "$DEPLOY_BRANCH:$DEPLOY_BRANCH" --force-with-lease

echo ""
echo "==> 同步完成！"
