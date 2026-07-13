#!/bin/bash

# AutoRelease - 自动化 GitHub Release 发布工具
# 基于 Git 标签自动递增版本号，简化发布流程
# Usage: ./release.sh [notes_file] [files...]

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m' # No Color

# 项目信息
PROJECT_NAME="vid-fetcher"
PROJECT_VERSION="1.0.0"

# ============================================
# 📝 配置区域 - 在此修改你的发布配置
# ============================================

# 要上传的文件列表（可在下方 FILES 数组中修改）
FILES=(
    "./dist/ytb2bili-extension-1.0.6-chrome.zip"
    "./dist/ytb2bili-extension-1.0.6-firefox.zip"
)

# Release 标题模板（支持占位符）
# {VERSION}      - 完整版本号，如: v1.0.5
# {VERSION_NO_V} - 不带 v 的版本号，如: 1.0.5
RELEASE_TITLE="Version {VERSION_NO_V} - Auto Release"

# 是否创建为草稿（true/false）
DRAFT="false"

# 是否标记为预发布版本（true/false）
PRERELEASE="false"

# 默认发布说明文件
DEFAULT_NOTES_FILE="release_notes.md"

# ============================================
# 🎣 自定义 Hooks（可选）
# ============================================

# 发布前执行的操作
pre_release_hook() {
    echo "🔧 执行发布前准备..."
    # 在这里添加你的构建命令
    # npm run build
    # npm run test
}

# 发布后执行的操作
post_release_hook() {
    echo "📢 执行发布后操作..."
    # 在这里添加你的通知命令
    # ./scripts/notify-team.sh "$TAG_NAME"
}

# ============================================
# 💡 提示：不需要修改以下代码
# ============================================

# 帮助信息
show_help() {
    cat << EOF

${CYAN}╔════════════════════════════════════════════════════════════════╗${NC}
${CYAN}║${NC}  ${BLUE}${PROJECT_NAME} v${PROJECT_VERSION}${NC} - 自动化 GitHub Release 发布工具  ${CYAN}║${NC}
${CYAN}╚════════════════════════════════════════════════════════════════╝${NC}

${GREEN}✨ 核心特性:${NC}
  🏷️  自动从 Git 获取最新 tag 并递增版本号
  🚀 零配置版本管理，无需手动维护版本号
  📦 批量文件上传，支持多平台发布
  📊 智能文件大小显示（MB + 字节）
  🎨 友好的彩色输出和进度提示
  🔄 完美支持 CI/CD 自动化流程

${GREEN}📖 使用方法:${NC}
  $0 [notes_file] [files...]

${GREEN}📋 参数说明:${NC}
  notes_file      发布说明 Markdown 文件 (可选，默认: $DEFAULT_NOTES_FILE)
  files           额外要上传的文件 (可选，追加到配置的 FILES 数组)

${GREEN}⚙️  配置说明:${NC}
  • 在脚本顶部的 ${YELLOW}配置区域${NC} 修改以下参数：
    ${CYAN}FILES${NC}           - 待上传文件数组
    ${CYAN}RELEASE_TITLE${NC}   - 发布标题模板（支持占位符）
    ${CYAN}DRAFT${NC}           - 是否为草稿 (true/false)
    ${CYAN}PRERELEASE${NC}      - 是否为预发布 (true/false)
  
  • 占位符支持：
    ${CYAN}{VERSION}${NC}       - 完整版本号 (如: v1.0.5)
    ${CYAN}{VERSION_NO_V}${NC}  - 不带 v 的版本号 (如: 1.0.5)

${GREEN}💡 命令示例:${NC}
  ${CYAN}# 使用默认配置（读取 $DEFAULT_NOTES_FILE）${NC}
  $0

  ${CYAN}# 指定发布说明文件${NC}
  $0 changelog.md

  ${CYAN}# 指定发布说明文件和额外上传文件${NC}
  $0 release_notes.md ./extra/file1.zip ./extra/file2.tar.gz

  ${CYAN}# 查看帮助信息${NC}
  $0 --help

  ${CYAN}# 查看版本${NC}
  $0 --version

${GREEN}🔄 工作流程:${NC}
  1️⃣  解析命令行参数
  2️⃣  环境检查（gh、git、登录状态）
  3️⃣  自动获取最新 Git tag 并递增版本号
  4️⃣  验证文件存在并显示大小
  5️⃣  读取发布说明文件
  6️⃣  创建并推送 Git tag
  7️⃣  创建 GitHub Release 并上传文件
  8️⃣  执行自定义 Hooks（可选）

${GREEN}📦 环境要求:${NC}
  ✅ GitHub CLI (gh) - https://cli.github.com/
  ✅ Git 仓库
  ✅ 已登录 GitHub: ${YELLOW}gh auth login${NC}

${GREEN}🏷️  版本管理:${NC}
  脚本会自动获取最新 tag 并递增 patch 版本号
  ${CYAN}示例:${NC} v1.0.12 → v1.0.13 → v1.0.14 ...
  
  ${YELLOW}如需升级主版本或次版本，手动创建 tag:${NC}
  git tag v2.0.0 -m "Major version 2.0.0"
  git push origin v2.0.0

${GREEN}📚 项目地址:${NC}
  https://github.com/difyz9/AutoRelease

EOF
}

# 获取最新的 Git Tag 并自动递增
get_next_version() {
    echo ""
    echo -e "${CYAN}╔════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║${NC}  ${BLUE}🏷️  自动获取版本号${NC}                 ${CYAN}║${NC}"
    echo -e "${CYAN}╚════════════════════════════════════════╝${NC}"
    
    # 获取最新的 tag（按版本号排序）
    local latest_tag=$(git tag --sort=-v:refname | head -n 1)
    
    if [ -z "$latest_tag" ]; then
        # 如果没有任何 tag，使用默认版本
        TAG_NAME="v1.0.0"
        echo -e "${YELLOW}⚠️  仓库中没有任何 tag${NC}"
        echo -e "${GREEN}✅ 使用默认版本号: ${MAGENTA}$TAG_NAME${NC}"
    else
        echo -e "📋 最新 tag: ${CYAN}$latest_tag${NC}"
        
        # 自动递增版本号
        TAG_NAME=$(auto_increment_version "$latest_tag")
        echo -e "${GREEN}✅ 自动递增版本号: ${CYAN}$latest_tag${NC} ${YELLOW}→${NC} ${MAGENTA}$TAG_NAME${NC}"
    fi
    
    echo -e "${CYAN}╚════════════════════════════════════════╝${NC}"
}

# 读取发布说明文件
read_release_notes() {
    local notes_file="$1"
    
    if [ ! -f "$notes_file" ]; then
        echo -e "${RED}❌ 错误: 发布说明文件不存在: $notes_file${NC}"
        echo -e "${YELLOW}💡 提示: 请创建 $notes_file 或指定其他 Markdown 文件${NC}"
        exit 1
    fi
    
    echo -e "${BLUE}📝 读取发布说明: ${CYAN}$notes_file${NC}"
    
    # 读取文件内容
    RELEASE_NOTES=$(cat "$notes_file")
    
    if [ -z "$RELEASE_NOTES" ]; then
        echo -e "${RED}❌ 错误: 发布说明文件为空${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}✅ 发布说明读取成功${NC}"
}

# 验证环境
check_environment() {
    # 检查 gh 是否安装
    if ! command -v gh &> /dev/null; then
        echo -e "${RED}❌ 错误: GitHub CLI (gh) 未安装${NC}"
        echo ""
        echo -e "${YELLOW}💡 安装方法:${NC}"
        echo -e "  ${CYAN}macOS:${NC}   brew install gh"
        echo -e "  ${CYAN}Linux:${NC}   sudo apt install gh"
        echo -e "  ${CYAN}Windows:${NC} winget install GitHub.cli"
        echo -e "  ${CYAN}官网:${NC}    https://cli.github.com/"
        exit 1
    fi
    
    # 检查是否在 git 仓库中
    if ! git rev-parse --git-dir > /dev/null 2>&1; then
        echo -e "${RED}❌ 错误: 当前目录不是 git 仓库${NC}"
        echo -e "${YELLOW}💡 提示: 请在 git 仓库目录中运行此脚本${NC}"
        exit 1
    fi
    
    # 检查是否已登录 GitHub CLI
    if ! gh auth status &> /dev/null; then
        echo -e "${RED}❌ 错误: 未登录 GitHub CLI${NC}"
        echo -e "${YELLOW}💡 登录方法: ${GREEN}gh auth login${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}✅ 环境检查通过${NC}"
}

# 验证文件
validate_files() {
    local all_valid=true
    
    echo -e "${BLUE}📦 验证文件:${NC}"
    echo ""
    for file in "${FILES[@]}"; do
        if [ ! -f "$file" ]; then
            echo -e "${RED}❌ 文件不存在: $file${NC}"
            all_valid=false
        else
            # 获取文件大小（字节）
            local size_bytes=$(stat -f%z "$file" 2>/dev/null || stat -c%s "$file" 2>/dev/null)
            # 转换为人类可读格式
            local size_mb=$(awk "BEGIN {printf \"%.2f\", $size_bytes/1024/1024}")
            echo -e "${GREEN}  ✅ ${CYAN}$(basename "$file")${NC}"
            echo -e "     📊 大小: ${YELLOW}${size_mb} MB${NC} ${BLUE}(${size_bytes} bytes)${NC}"
            echo -e "     📁 路径: ${file}"
            echo ""
        fi
    done
    
    if [ "$all_valid" = false ]; then
        echo -e "${RED}❌ 错误: 部分文件不存在${NC}"
        exit 1
    fi
}

# 读取发布说明
prepare_release_notes() {
    # 替换发布标题中的占位符
    RELEASE_TITLE="${RELEASE_TITLE//\{VERSION\}/$TAG_NAME}"
    RELEASE_TITLE="${RELEASE_TITLE//\{VERSION_NO_V\}/${TAG_NAME#v}}"
    
    # 替换发布说明中的占位符
    RELEASE_NOTES="${RELEASE_NOTES//\{VERSION\}/$TAG_NAME}"
    RELEASE_NOTES="${RELEASE_NOTES//\{VERSION_NO_V\}/${TAG_NAME#v}}"
    
    echo -e "${GREEN}✅ 发布信息已准备完成${NC}"
    echo -e "${BLUE}   标题: ${CYAN}$RELEASE_TITLE${NC}"
    echo -e "${BLUE}   说明: ${CYAN}${#RELEASE_NOTES} 字符${NC}"
}

# 自动递增版本号
auto_increment_version() {
    local tag="$1"
    
    # 解析版本号 (支持 v1.2.3 或 1.2.3 格式)
    if [[ "$tag" =~ ^(v?)([0-9]+)\.([0-9]+)\.([0-9]+)(-.*)?$ ]]; then
        local prefix="${BASH_REMATCH[1]}"
        local major="${BASH_REMATCH[2]}"
        local minor="${BASH_REMATCH[3]}"
        local patch="${BASH_REMATCH[4]}"
        local suffix="${BASH_REMATCH[5]}"
        
        # 递增 patch 版本号
        patch=$((patch + 1))
        
        # 返回新版本号
        echo "${prefix}${major}.${minor}.${patch}${suffix}"
    else
        # 如果无法解析，返回原版本号加 .1
        echo "${tag}.1"
    fi
}

# 创建并推送 Git Tag
create_and_push_tag() {
    echo ""
    echo -e "${CYAN}╔════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║${NC}  ${BLUE}🏷️  检查并创建 Git Tag${NC}             ${CYAN}║${NC}"
    echo -e "${CYAN}╚════════════════════════════════════════╝${NC}"
    
    # 循环检查远程，如果存在则继续递增（理论上不应该发生，但作为保险）
    while git ls-remote --tags origin | grep -q "refs/tags/$TAG_NAME$"; do
        echo -e "${YELLOW}⚠️  Tag ${MAGENTA}$TAG_NAME${NC} 已存在于远程仓库（继续递增）${NC}"
        TAG_NAME=$(auto_increment_version "$TAG_NAME")
        echo -e "${BLUE}💡 自动递增版本号到: ${MAGENTA}${TAG_NAME}${NC}"
    done
    
    # 设置 RELEASE_TITLE（如果未设置或为空）
    if [ -z "$RELEASE_TITLE" ]; then
        RELEASE_TITLE="Release $TAG_NAME"
    else
        # 尝试更新 RELEASE_TITLE 中的版本号占位符
        RELEASE_TITLE="${RELEASE_TITLE//\{VERSION\}/$TAG_NAME}"
        RELEASE_TITLE="${RELEASE_TITLE//\{VERSION_NO_V\}/${TAG_NAME#v}}"
    fi
    
    echo -e "🏷️  Tag:   ${MAGENTA}$TAG_NAME${NC}"
    echo -e "📝 Title: ${CYAN}$RELEASE_TITLE${NC}"
    echo ""
    
    # 检查本地是否存在
    if git rev-parse "$TAG_NAME" >/dev/null 2>&1; then
        echo -e "${YELLOW}⚠️  Tag ${MAGENTA}$TAG_NAME${NC} 已存在于本地但不在远程${NC}"
        echo -e "${YELLOW}⬆️  推送本地 tag 到远程...${NC}"
        git push origin "$TAG_NAME"
        echo -e "${GREEN}✅ Tag 推送成功${NC}"
    else
        # 创建新 tag
        echo -e "${YELLOW}➕ 创建新 tag: ${MAGENTA}$TAG_NAME${NC}"
        
        # 获取当前 commit
        local current_commit=$(git rev-parse HEAD)
        echo -e "📌 当前 commit: ${BLUE}${current_commit:0:8}${NC}"
        
        # 创建 tag
        git tag -a "$TAG_NAME" -m "Release $TAG_NAME"
        echo -e "${GREEN}✅ Tag 创建成功${NC}"
        
        # 推送 tag 到远程
        echo -e "${YELLOW}⬆️  推送 tag 到远程仓库...${NC}"
        git push origin "$TAG_NAME"
        echo -e "${GREEN}✅ Tag 推送成功${NC}"
    fi
    
    echo -e "${CYAN}╚════════════════════════════════════════╝${NC}"
}

# 执行发布
do_release() {
    echo ""
    echo -e "${CYAN}╔════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║${NC}  ${BLUE}🚀 开始发布流程${NC}                     ${CYAN}║${NC}"
    echo -e "${CYAN}╚════════════════════════════════════════╝${NC}"
    echo -e "🏷️  Tag:        ${MAGENTA}$TAG_NAME${NC}"
    echo -e "📝 Title:      ${CYAN}$RELEASE_TITLE${NC}"
    echo -e "📦 Files:      ${GREEN}${#FILES[@]} 个文件${NC}"
    echo -e "📋 Draft:      ${YELLOW}$DRAFT${NC}"
    echo -e "🔖 Prerelease: ${YELLOW}$PRERELEASE${NC}"
    echo -e "${CYAN}╚════════════════════════════════════════╝${NC}"
    echo ""
    
    # 执行 pre-release hook
    if type pre_release_hook &> /dev/null; then
        echo -e "${YELLOW}🔧 执行 pre-release hook...${NC}"
        echo ""
        pre_release_hook
        echo ""
    fi
    
    # 检查 release 是否已存在
    if gh release view "$TAG_NAME" &> /dev/null; then
        echo -e "${YELLOW}⚠️  Release ${MAGENTA}$TAG_NAME${NC} 已存在，将添加文件到现有 release${NC}"
        echo ""
        
        # 上传文件到现有 release
        for file in "${FILES[@]}"; do
            echo -e "${GREEN}⬆️  上传: ${CYAN}$(basename "$file")${NC}"
            gh release upload "$TAG_NAME" "$file" --clobber
        done
        
        echo ""
        echo -e "${GREEN}✅ 所有文件已成功上传到 release ${MAGENTA}$TAG_NAME${NC}"
    else
        echo -e "${YELLOW}➕ 创建新的 release: ${MAGENTA}$TAG_NAME${NC}"
        echo ""
        
        # 构建 gh release create 命令参数数组（先不上传文件）
        local gh_args=("release" "create" "$TAG_NAME")
        
        # 添加标题
        gh_args+=("--title" "$RELEASE_TITLE")
        
        # 添加发布说明
        if [ -n "$RELEASE_NOTES" ]; then
            gh_args+=("--notes" "$RELEASE_NOTES")
        else
            gh_args+=("--generate-notes")
        fi
        
        # 添加 draft 和 prerelease 标志
        if [ "$DRAFT" = "true" ]; then
            gh_args+=("--draft")
        fi
        
        if [ "$PRERELEASE" = "true" ]; then
            gh_args+=("--prerelease")
        fi
        
        # 执行命令（先创建 release，不上传文件）
        if ! gh "${gh_args[@]}"; then
            echo ""
            echo -e "${RED}❌ 错误: Release 创建失败${NC}"
            exit 1
        fi
        
        echo ""
        echo -e "${GREEN}✅ Release 创建成功${NC}"
        
        # 验证 release 是否真的创建成功
        if ! gh release view "$TAG_NAME" &> /dev/null; then
            echo -e "${RED}❌ 错误: Release 创建后无法访问${NC}"
            exit 1
        fi
        
        # 分别上传每个文件（更可靠）
        echo ""
        echo -e "${YELLOW}⬆️  开始上传文件...${NC}"
        for file in "${FILES[@]}"; do
            echo -e "${GREEN}⬆️  上传: ${CYAN}$(basename "$file")${NC}"
            if ! gh release upload "$TAG_NAME" "$file" --clobber; then
                echo -e "${RED}❌ 错误: 文件上传失败: $file${NC}"
                echo -e "${YELLOW}💡 提示: 你可以稍后手动上传文件或重新运行脚本${NC}"
                # 不退出，继续尝试其他文件
            fi
        done
        
        echo ""
        echo -e "${GREEN}✅ 文件上传完成${NC}"
    fi
    
    # 获取 release URL
    RELEASE_URL=$(gh release view "$TAG_NAME" --json url -q .url)
    
    echo ""
    echo -e "${GREEN}╔════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║${NC}  ${YELLOW}🎉 发布成功！${NC}                        ${GREEN}║${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════╝${NC}"
    echo -e "${GREEN}🔗 Release URL:${NC}"
    echo -e "${CYAN}$RELEASE_URL${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════╝${NC}"
    
    # 执行 post-release hook
    if type post_release_hook &> /dev/null; then
        echo ""
        echo -e "${YELLOW}🔧 执行 post-release hook...${NC}"
        echo ""
        post_release_hook
    fi
}

# 主函数
main() {
    # 处理帮助
    if [ "$1" = "-h" ] || [ "$1" = "--help" ]; then
        show_help
        exit 0
    fi
    
    # 显示版本信息
    if [ "$1" = "-v" ] || [ "$1" = "--version" ]; then
        echo -e "${CYAN}${PROJECT_NAME} v${PROJECT_VERSION}${NC}"
        echo -e "${BLUE}自动化 GitHub Release 发布工具${NC}"
        exit 0
    fi
    
    # 解析命令行参数
    local notes_file="${1:-$DEFAULT_NOTES_FILE}"
    shift 2>/dev/null || true
    
    # 添加命令行传入的额外文件
    if [ $# -gt 0 ]; then
        FILES+=("$@")
    fi
    
    # 显示欢迎信息
    echo ""
    echo -e "${CYAN}╔════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║${NC}  ${BLUE}${PROJECT_NAME} v${PROJECT_VERSION}${NC}                    ${CYAN}║${NC}"
    echo -e "${CYAN}║${NC}  ${GREEN}自动化 GitHub Release 发布工具${NC}      ${CYAN}║${NC}"
    echo -e "${CYAN}╚════════════════════════════════════════╝${NC}"
    echo ""
    
    # 执行流程
    check_environment
    echo ""
    read_release_notes "$notes_file"
    echo ""
    get_next_version
    echo ""
    validate_files
    echo ""
    prepare_release_notes
    echo ""
    create_and_push_tag
    echo ""
    do_release
    echo ""
}

# 运行主函数
main "$@"
