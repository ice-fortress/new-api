#!/usr/bin/env bash
# update.sh — 部署机拉取最新 deploy/personal 并重新部署
#
# 职责：fetch fork → 对齐本地 deploy/personal 到 fork 最新状态 → 调用 rebuild.sh 重新部署
# deploy/personal 在同步机上会被 rebase 后强推，历史会变化，所以这里不用 git pull，
# 而是比对本地独有提交涉及的文件在远程新历史中的内容：一致则视为安全，直接对齐；
# 不一致则停止，避免覆盖本机专属修改（例如本机定制过的 docker-compose.override.yml）。
# 前提：仅限部署机使用（origin 直接指向个人 fork）；请在 deploy/personal 分支运行，
# 且工作区无未提交修改。
# 用法：./update.sh

set -euo pipefail

REMOTE="origin"
DEPLOY_BRANCH="deploy/personal"

echo "==> 检查当前分支..."
CURRENT_BRANCH=$(git branch --show-current)
if [ "$CURRENT_BRANCH" != "$DEPLOY_BRANCH" ]; then
    echo "    当前分支是 $CURRENT_BRANCH，请先切换到 $DEPLOY_BRANCH 后再更新"
    exit 1
fi

echo "==> 检查工作区状态..."
if ! git diff --quiet || ! git diff --cached --quiet; then
    echo "    工作区存在已跟踪文件修改，请先 commit 或 stash 后再更新"
    git status --short
    exit 1
fi

echo "==> 获取最新代码..."
git fetch "$REMOTE"

LOCAL_HEAD=$(git rev-parse "$DEPLOY_BRANCH")
REMOTE_HEAD=$(git rev-parse "$REMOTE/$DEPLOY_BRANCH")

if [ "$LOCAL_HEAD" = "$REMOTE_HEAD" ]; then
    echo "    已经是最新"
else
    MERGE_BASE=$(git merge-base "$DEPLOY_BRANCH" "$REMOTE/$DEPLOY_BRANCH")
    mapfile -t LOCAL_ONLY_FILES < <(git diff --name-only "$MERGE_BASE" "$LOCAL_HEAD")

    if [ "${#LOCAL_ONLY_FILES[@]}" -eq 0 ] || git diff --quiet "$REMOTE_HEAD" "$LOCAL_HEAD" -- "${LOCAL_ONLY_FILES[@]}"; then
        echo "    对齐本地 $DEPLOY_BRANCH 到 $REMOTE/$DEPLOY_BRANCH..."
        git reset --hard "$REMOTE_HEAD"
    else
        echo "    本地独有提交涉及的文件与远程新历史内容不一致，可能包含本机专属修改，已停止："
        git diff --stat "$REMOTE_HEAD" "$LOCAL_HEAD" -- "${LOCAL_ONLY_FILES[@]}"
        echo "    请人工检查差异：git diff $REMOTE_HEAD $LOCAL_HEAD -- <file>"
        echo "    确认可以覆盖后手动执行：git reset --hard $REMOTE_HEAD"
        exit 1
    fi
fi

echo ""
echo "==> 调用 rebuild.sh 重新部署..."
./rebuild.sh

echo ""
echo "==> 更新完成！"
