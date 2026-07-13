#!/bin/bash
# ytb2bili-go 快速部署脚本（跨平台自动适配）
# 支持: Linux (Debian/Ubuntu, RHEL/Fedora, Arch), macOS, Windows (Git Bash/WSL)
# 使用方法: bash deploy.sh

set -e

# ─── 颜色 ─────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

info()  { echo -e "${CYAN}[INFO]${NC} $1"; }
ok()    { echo -e "${GREEN}[✅]${NC} $1"; }
warn()  { echo -e "${YELLOW}[⚠️]${NC} $1"; }
err()   { echo -e "${RED}[❌]${NC} $1"; }
header(){ echo -e "\n${BLUE}━━━ $1 ━━━${NC}"; }

# ─── 平台检测 ─────────────────────────────────────────────────────────────

detect_platform() {
    case "$(uname -s)" in
        Linux*)     OS="linux" ;;
        Darwin*)    OS="macos" ;;
        MINGW*|MSYS*|CYGWIN*) OS="windows" ;;
        *)          OS="unknown" ;;
    esac

    if [ "$OS" = "linux" ]; then
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            DISTRO="$ID"
            DISTRO_FAMILY="$ID_LIKE"
        elif [ -f /etc/debian_version ]; then
            DISTRO="debian"
            DISTRO_FAMILY="debian"
        elif [ -f /etc/redhat-release ]; then
            DISTRO="rhel"
            DISTRO_FAMILY="rhel"
        else
            DISTRO="unknown"
            DISTRO_FAMILY=""
        fi
    elif [ "$OS" = "macos" ]; then
        DISTRO="macos"
        DISTRO_FAMILY="macos"
    elif [ "$OS" = "windows" ]; then
        DISTRO="windows"
        DISTRO_FAMILY="windows"
    fi
}

# ─── 包管理器检测 ─────────────────────────────────────────────────────────

detect_pkg_manager() {
    case "$OS" in
        linux)
            if   command -v apt &>/dev/null; then PKG_MGR="apt";  INSTALL_CMD="sudo apt install -y"
            elif command -v dnf &>/dev/null; then PKG_MGR="dnf";  INSTALL_CMD="sudo dnf install -y"
            elif command -v yum &>/dev/null; then PKG_MGR="yum";  INSTALL_CMD="sudo yum install -y"
            elif command -v pacman &>/dev/null; then PKG_MGR="pacman"; INSTALL_CMD="sudo pacman -S --noconfirm"
            elif command -v zypper &>/dev/null; then PKG_MGR="zypper"; INSTALL_CMD="sudo zypper install -y"
            else PKG_MGR="unknown"; INSTALL_CMD=""
            fi
            ;;
        macos)
            if command -v brew &>/dev/null; then PKG_MGR="brew"; INSTALL_CMD="brew install"
            else PKG_MGR="unknown"; INSTALL_CMD=""
            fi
            ;;
        windows)
            if command -v winget &>/dev/null; then PKG_MGR="winget"; INSTALL_CMD="winget install"
            elif command -v choco &>/dev/null; then PKG_MGR="choco"; INSTALL_CMD="choco install -y"
            else PKG_MGR="scoop"; INSTALL_CMD="scoop install"
            fi
            ;;
    esac
}

# ─── 安装建议 ─────────────────────────────────────────────────────────────

pkg_names() {
    local dep=$1
    case "$PKG_MGR" in
        apt)      
            case "$dep" in
                go)     echo "golang" ;;
                yt-dlp) echo "yt-dlp" ;;
                ffmpeg) echo "ffmpeg" ;;
                deno)   echo "" ;;  # apt 没有 deno
            esac
            ;;
        dnf|yum)  
            case "$dep" in
                go)     echo "golang" ;;
                yt-dlp) echo "yt-dlp" ;;
                ffmpeg) echo "ffmpeg" ;;
                deno)   echo "" ;;
            esac
            ;;
        pacman)   
            case "$dep" in
                go)     echo "go" ;;
                yt-dlp) echo "yt-dlp" ;;
                ffmpeg) echo "ffmpeg" ;;
                deno)   echo "deno" ;;
            esac
            ;;
        brew)     
            case "$dep" in
                go)     echo "go" ;;
                yt-dlp) echo "yt-dlp" ;;
                ffmpeg) echo "ffmpeg" ;;
                deno)   echo "deno" ;;
            esac
            ;;
        winget)   
            case "$dep" in
                go)     echo "GoLang.Go" ;;
                yt-dlp) echo "yt-dlp.yt-dlp" ;;
                ffmpeg) echo "FFmpeg" ;;
                deno)   echo "DenoLand.Deno" ;;
            esac
            ;;
        choco)    
            case "$dep" in
                go)     echo "golang" ;;
                yt-dlp) echo "yt-dlp" ;;
                ffmpeg) echo "ffmpeg" ;;
                deno)   echo "deno" ;;
            esac
            ;;
        scoop)    
            case "$dep" in
                go)     echo "go" ;;
                yt-dlp) echo "yt-dlp" ;;
                ffmpeg) echo "ffmpeg" ;;
                deno)   echo "deno" ;;
            esac
            ;;
        *)
            echo ""
            ;;
    esac
}

# ─── 显示安装指南 ─────────────────────────────────────────────────────────

show_install_guide() {
    local dep=$1
    echo ""
    warn "$dep 未安装"
    case "$OS" in
        linux)
            case "$dep" in
                deno)
                    echo "  推荐安装方式:"
                    echo "  ${CYAN}curl -fsSL https://deno.land/install.sh | sh${NC}"
                    echo "  然后将 deno 加入 PATH:"
                    echo "  ${CYAN}export PATH=\"\$HOME/.deno/bin:\$PATH\"${NC}"
                    echo "  (建议添加到 ~/.bashrc 或 ~/.zshrc)"
                    if command -v npm &>/dev/null; then
                        echo ""
                        echo "  或通过 npm:"
                        echo "  ${CYAN}npm install -g deno${NC}"
                    fi
                    ;;
                *)
                    local pkg=$(pkg_names "$dep")
                    if [ -n "$pkg" ]; then
                        echo "  安装命令: ${CYAN}$INSTALL_CMD $pkg${NC}"
                    else
                        echo "  当前包管理器未找到 $dep，请手动安装。"
                    fi
                    ;;
            esac
            ;;
        macos)
            echo "  安装命令: ${CYAN}brew install $dep${NC}"
            ;;
        windows)
            local pkg=$(pkg_names "$dep")
            if [ -n "$pkg" ]; then
                echo "  安装命令: ${CYAN}$INSTALL_CMD $pkg${NC}"
            fi
            ;;
    esac
}

# ─── 检查单个依赖 ─────────────────────────────────────────────────────────

check_dep() {
    local dep=$1
    local label=${2:-$1}
    if command -v "$dep" &>/dev/null; then
        # 不同工具的版本参数不同
        local ver=""
        case "$dep" in
            go)     ver=$(go version 2>/dev/null | grep -oP 'go\S+') ;;
            ffmpeg) ver=$(ffmpeg -version 2>&1 | head -1) ;;
            deno)   ver=$(deno --version 2>/dev/null | head -1) ;;
            *)      ver=$("$dep" --version 2>/dev/null | head -1) ;;
        esac
        [ -n "$ver" ] && ok "$label ($ver)" || ok "$label"
        return 0
    else
        show_install_guide "$dep"
        return 1
    fi
}

# ─── 自动安装（可选） ────────────────────────────────────────────────────

auto_install_dep() {
    local dep=$1
    local pkg=$(pkg_names "$dep")
    
    if [ -z "$pkg" ] || [ -z "$INSTALL_CMD" ]; then
        return 1
    fi

    # 跳过 deno 特殊处理
    if [ "$dep" = "deno" ] && [ "$PKG_MGR" != "pacman" ] && [ "$PKG_MGR" != "brew" ]; then
        return 1
    fi

    info "正在安装 $dep..."
    if $INSTALL_CMD "$pkg" 2>&1; then
        ok "$dep 安装成功"
        return 0
    else
        err "$dep 安装失败，请手动安装"
        return 1
    fi
}

# ===========================================================================
# 主流程
# ===========================================================================

echo ""
echo -e "${BLUE}╔══════════════════════════════════════╗${NC}"
echo -e "${BLUE}║   🚀 ytb2bili-go 跨平台部署工具        ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════╝${NC}"

detect_platform
detect_pkg_manager

echo ""
info "系统: $OS"
info "发行版: ${DISTRO:-unknown} ${DISTRO_FAMILY:+($DISTRO_FAMILY)}"
info "架构: $(uname -m)"
info "包管理器: ${PKG_MGR:-未检测到}"

# ─── 第一步：检查依赖 ──────────────────────────────────────────────────

header "检查依赖"

MISSING_DEPS=()

check_dep "go"     "Go"      || MISSING_DEPS+=("go")
check_dep "yt-dlp" "yt-dlp"  || MISSING_DEPS+=("yt-dlp")
check_dep "ffmpeg" "ffmpeg"  || MISSING_DEPS+=("ffmpeg")
check_dep "deno"   "Deno"    || MISSING_DEPS+=("deno")

# ─── 第二步：尝试自动安装缺失依赖 ─────────────────────────────────────

if [ ${#MISSING_DEPS[@]} -gt 0 ]; then
    echo ""
    header "尝试自动安装缺失依赖"
    
    CAN_AUTO=true
    for dep in "${MISSING_DEPS[@]}"; do
        if ! auto_install_dep "$dep"; then
            CAN_AUTO=false
        fi
    done

    # 再次检查是否还有缺失
    STILL_MISSING=()
    for dep in "${MISSING_DEPS[@]}"; do
        if ! command -v "$dep" &>/dev/null; then
            STILL_MISSING+=("$dep")
        fi
    done

    if [ ${#STILL_MISSING[@]} -gt 0 ]; then
        echo ""
        warn "部分依赖仍未安装，请手动安装后再运行本脚本："
        for dep in "${STILL_MISSING[@]}"; do
            echo "  - $dep"
        done
        
        if [ "$OS" = "linux" ] && [ "$DISTRO" = "debian" ] || [ "$DISTRO" = "ubuntu" ]; then
            echo ""
            echo "Debian/Ubuntu 一键安装命令:"
            echo "  ${CYAN}sudo apt install -y golang yt-dlp ffmpeg${NC}"
            echo "  ${CYAN}curl -fsSL https://deno.land/install.sh | sh${NC}"
            echo "  ${CYAN}export PATH=\"\$HOME/.deno/bin:\$PATH\"${NC}"
        elif [ "$OS" = "linux" ] && ([ "$DISTRO" = "fedora" ] || [ "$DISTRO_FAMILY" = "rhel" ] || [ "$DISTRO_FAMILY" = "fedora" ]); then
            echo ""
            echo "RHEL/Fedora 一键安装命令:"
            echo "  ${CYAN}sudo dnf install -y golang yt-dlp ffmpeg${NC}"
            echo "  ${CYAN}curl -fsSL https://deno.land/install.sh | sh${NC}"
        elif [ "$OS" = "macos" ]; then
            echo ""
            echo "macOS 一键安装命令:"
            echo "  ${CYAN}brew install go yt-dlp ffmpeg deno${NC}"
        fi

        echo ""
        warn "安装完成后重新运行: ${CYAN}bash deploy.sh${NC}"
        exit 1
    fi
fi

echo ""
ok "所有依赖已就绪"

# ─── 第三步：编译项目 ──────────────────────────────────────────────────

header "编译项目"

echo ""
info "正在编译 ytb2bili..."

go build -o ytb2bili .

if [ $? -eq 0 ]; then
    ok "编译成功 → $(pwd)/ytb2bili ($(du -h ytb2bili | cut -f1))"
else
    err "编译失败"
    exit 1
fi

# ─── 第四步：创建数据目录 ─────────────────────────────────────────────

header "创建数据目录"

mkdir -p data/downloads
mkdir -p data/history
mkdir -p data/cookies
ok "数据目录已就绪 (data/downloads, data/history, data/cookies)"

# ─── 第五步：添加 PATH 建议 ───────────────────────────────────────────

if [ "$OS" = "linux" ] || [ "$OS" = "macos" ]; then
    # 检查 deno 是否在 PATH 中（如果通过 install.sh 安装）
    if ! command -v deno &>/dev/null && [ -f "$HOME/.deno/bin/deno" ]; then
        warn "deno 已安装但不在 PATH 中"
        echo "  运行以下命令添加到 PATH:"
        echo "  ${CYAN}export PATH=\"\$HOME/.deno/bin:\$PATH\"${NC}"
        echo "  建议添加到 ~/.bashrc 或 ~/.zshrc"
        echo "  ${CYAN}echo 'export PATH=\"\$HOME/.deno/bin:\$PATH\"' >> ~/.bashrc${NC}"
    fi
fi

# ─── 完成 ───────────────────────────────────────────────────────────────

echo ""
echo -e "${GREEN}╔══════════════════════════════════════╗${NC}"
echo -e "${GREEN}║   ✅ ytb2bili-go 部署完成！          ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════╝${NC}"
echo ""

# 显示可用命令
echo -e "  ${CYAN}./ytb2bili --help${NC}             查看所有命令"
echo -e "  ${CYAN}./ytb2bili search --max 5 \"关键词\"${NC}  搜索 YouTube"
echo -e "  ${CYAN}./ytb2bili login${NC}               B站扫码登录"
echo -e "  ${CYAN}./ytb2bili submit <URL>${NC}        搬运视频到B站"
echo ""

# 提示配置环境变量
header "环境变量配置（可选）"

echo ""
echo "  以下环境变量按需设置（已有默认值的可跳过）："
echo ""
echo "  ${YELLOW}DEEPSEEK_API_KEY${NC}=your-key     # DeepSeek API (翻译用)"
echo "  ${YELLOW}YOUTUBE_COOKIES${NC}=/path/to/cookies.txt  # YouTube cookie (防下载限制)"
echo "  ${YELLOW}LLM_MODEL${NC}=deepseek-v4-flash     # LLM 模型名 (默认)"
echo "  ${YELLOW}LLM_BASE_URL${NC}=https://api.deepseek.com  # LLM 地址 (默认)"
echo ""

# 飞书多维表格配置（如果存在 env 文件则提示）
if [ -f .env ]; then
    info "检测到 .env 文件，环境变量已加载"
else
    echo "  飞书多维表格配置（可选，配合 Chrome 扩展使用）："
    echo "  ${YELLOW}FEISHU_APP_ID${NC}=xxx"
    echo "  ${YELLOW}FEISHU_APP_SECRET${NC}=xxx"
    echo "  ${YELLOW}BITABLE_APP_TOKEN${NC}=xxx"
    echo "  ${YELLOW}BITABLE_TABLE_ID${NC}=xxx"
    echo ""
fi

# 建议设置别名
echo "  建议设置别名："
echo "  ${CYAN}alias y2b='$(pwd)/ytb2bili'${NC}"
echo "  （建议添加到 ~/.bashrc 或 ~/.zshrc）"
echo ""
echo -e "${GREEN}━━━ 部署完成，开始搬运！🚀 ━━━${NC}"
echo ""
