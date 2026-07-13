#!/bin/bash

# 版本发布脚本
# 使用方法: ./scripts/release.sh [patch|minor|major]

set -e

# 检查是否在 git 仓库中
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo "❌ 错误: 当前目录不是 git 仓库"
    exit 1
fi

# 检查工作区是否干净
if [ -n "$(git status --porcelain)" ]; then
    echo "❌ 错误: 工作区有未提交的更改，请先提交或暂存"
    git status --short
    exit 1
fi

# 检查是否在 main 分支
current_branch=$(git branch --show-current)
if [ "$current_branch" != "main" ]; then
    echo "⚠️  警告: 当前不在 main 分支 (当前: $current_branch)"
    read -p "是否继续? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# 获取当前版本
current_version=$(node -p "require('./package.json').version")
echo "📦 当前版本: v$current_version"

# 确定新版本
version_type=${1:-patch}
case $version_type in
    patch|minor|major)
        echo "🔄 版本类型: $version_type"
        ;;
    *)
        echo "❌ 错误: 版本类型必须是 patch, minor, 或 major"
        echo "使用方法: $0 [patch|minor|major]"
        exit 1
        ;;
esac

# 使用 npm version 更新版本号
echo "⬆️  更新版本号..."
new_version=$(npm version $version_type --no-git-tag-version)
echo "✅ 新版本: $new_version"

# 更新 wxt.config.ts 中的版本号
echo "🔧 更新 wxt.config.ts 中的版本号..."
sed -i.bak "s/version: '[^']*'/version: '${new_version#v}'/" wxt.config.ts && rm wxt.config.ts.bak

# 提交更改
echo "💾 提交版本更新..."
git add package.json wxt.config.ts
git commit -m "chore: bump version to $new_version"

# 创建标签
echo "🏷️  创建标签 $new_version..."
git tag -a "$new_version" -m "Release $new_version"

# 推送到远程
echo "🚀 推送到远程仓库..."
git push origin main
git push origin "$new_version"

echo "✅ 版本发布完成!"
echo "📦 版本: $new_version"
echo "🔗 GitHub Actions 将自动构建并发布到 Releases"
echo "🌐 查看构建状态: https://github.com/$(git config --get remote.origin.url | sed 's/.*github.com[:/]\([^.]*\).*/\1/')/actions"