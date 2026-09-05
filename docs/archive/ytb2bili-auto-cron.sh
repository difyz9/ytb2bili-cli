#!/usr/bin/env bash
# ytb2bili-auto-cron.sh — 定时自动搜索、下载、上传并监听字幕
# 每30分钟由 cron 触发，自动从标准关键词库中选词，执行完整搬运流水线
set -euo pipefail

PROJECT_DIR="$HOME/guan/code/ytb2bili-go"
DATA_DIR="$PROJECT_DIR/data"
BIN="$PROJECT_DIR/ytb2bili"
STATE_FILE="$DATA_DIR/cron/cron_state.json"

# 标准关键词库（与 search.go StandardKeywords 同步）
KEYWORDS_AI=(
  "ai-1" "ai-2" "ai-3" "ai-4" "ai-5" "ai-6"
  "ai-7" "ai-8" "ai-9" "ai-10" "ai-11" "ai-12"
)
KEYWORDS_WEB=(
  "web-1" "web-2" "web-3" "web-4" "web-5" "web-6" "web-7" "web-8"
)
KEYWORDS_LLM=(
  "llm-1" "llm-2" "llm-3" "llm-4" "llm-5" "llm-6" "llm-7" "llm-8"
)
KEYWORDS_MEDIA=(
  "media-1" "media-2" "media-3" "media-4" "media-5" "media-6"
)
KEYWORDS_PI=(
  "pi-1" "pi-2" "pi-3" "pi-4" "pi-5" "pi-6" "pi-7" "pi-8" "pi-9" "pi-10"
)
ALL_CATEGORIES=(ai web llm media pi)

mkdir -p "$DATA_DIR/cron"

# 从 state 文件读取上次执行的位置
if [ -f "$STATE_FILE" ]; then
  LAST_CAT=$(python3 -c "import json; d=json.load(open('$STATE_FILE')); print(d.get('category',''))" 2>/dev/null || echo "")
  LAST_IDX=$(python3 -c "import json; d=json.load(open('$STATE_FILE')); print(d.get('index',0))" 2>/dev/null || echo "0")
  LAST_RUN=$(python3 -c "import json; d=json.load(open('$STATE_FILE')); print(d.get('last_run',''))" 2>/dev/null || echo "")
else
  LAST_CAT=""
  LAST_IDX=0
  LAST_RUN=""
fi

# Round-robin: 选下一个分类
CAT_INDEX=0
for i in "${!ALL_CATEGORIES[@]}"; do
  if [ "${ALL_CATEGORIES[$i]}" = "$LAST_CAT" ]; then
    CAT_INDEX=$(( (i + 1) % ${#ALL_CATEGORIES[@]} ))
    break
  fi
done

CAT="${ALL_CATEGORIES[$CAT_INDEX]}"

# 根据分类选关键词和索引
case "$CAT" in
  ai)    KEYWORDS=("${KEYWORDS_AI[@]}") ;;
  web)   KEYWORDS=("${KEYWORDS_WEB[@]}") ;;
  llm)   KEYWORDS=("${KEYWORDS_LLM[@]}") ;;
  media) KEYWORDS=("${KEYWORDS_MEDIA[@]}") ;;
  pi)    KEYWORDS=("${KEYWORDS_PI[@]}") ;;
esac

# 轮转关键词索引
KW_IDX=$((LAST_IDX % ${#KEYWORDS[@]}))
SELECTED_KW="${KEYWORDS[$KW_IDX]}"

echo "========================================"
echo "🕐 $(date '+%Y-%m-%d %H:%M:%S')"
echo "📂 分类: $CAT (${CAT_INDEX}/4)"
echo "🔑 关键词: $SELECTED_KW (index $KW_IDX)"
echo "========================================"

# 执行自动搬运（最多1个视频）
cd "$PROJECT_DIR"
$BIN auto --max-videos 1 "$SELECTED_KW" 2>&1 || true

# 保存状态
NOW=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
python3 -c "
import json
with open('$STATE_FILE', 'w') as f:
    json.dump({
        'category': '$CAT',
        'index': $((KW_IDX + 1)),
        'last_run': '$NOW'
    }, f)
" 2>/dev/null

# ── 字幕监听 ──
# auto 命令同步了字幕追踪记录到 $DATA_DIR/subtitles/，但不会启动监听
# 检查有没有 pending 字幕，然后尝试用 subtitle retry 重试
echo ""
echo "📝 检查待上传字幕..."
PENDING_VIDEOS=$($BIN subtitle status 2>&1 | grep "pending\|failed" | head -5 || true)
if [ -n "$PENDING_VIDEOS" ]; then
  echo "  发现待上传字幕，尝试重试..."
  # 提取 BVID
  for bvid in $(echo "$PENDING_VIDEOS" | grep -oP 'BV[A-Za-z0-9]+' | sort -u); do
    echo "  🔄 $bvid 字幕重试..."
    # 后台启动监听（异步，最多等3分钟后退出）
    timeout 180 $BIN subtitle retry "$bvid" 2>&1 || true
    echo "  ✅ $bvid 处理完成"
  done
else
  echo "  无待上传字幕"
fi

echo ""
echo "✅ 本轮完成"
