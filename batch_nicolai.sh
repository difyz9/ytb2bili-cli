#!/usr/bin/env bash
# 批量搬运 @NicolaiAI 频道近 1 个月视频（9 个）
# Nicolai Nielsen - AI 智能体/编程主题频道
# 约束: 投稿间隔 >= 20 分钟 (1200s)，磁盘 <8G 自动清理
set -u
cd /home/guan/guan/code/ytb2bili-cli
LOG="data/batch_nicolai.log"
MIN_INTERVAL=1200
MIN_DISK_GB=8
VIDS="OjDeBzmGsl4 EdpWD1V2nq0 aOpols4N1ao ImJiizbRf08 E7UkNupdRQw 83WzPqDvXgY Rtkrs6-uFB4 mqdIwY-AzbU UEUNw_NooYs"

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG"; }

disk_gb() { df / | awk 'NR==2 {print int($4/1024/1024)}'; }

cleanup() {
    local used=$(disk_gb)
    if [ "$used" -lt "$MIN_DISK_GB" ]; then
        log "⚠️ 磁盘仅 ${used}G，清理已投稿视频的原始文件..."
        for d in data/downloads/*/; do
            [ -d "$d" ] || continue
            local id=$(basename "$d")
            # 仅清理 history 中已投稿的（保留 synced.mp4 + 字幕）
            if grep -q "\"$id\"" data/history/history.json 2>/dev/null; then
                rm -f "$d"*.mp4 "$d"*.webm "$d"*.part 2>/dev/null
                rm -rf "$d"voice 2>/dev/null
                log "  已清理: $id (保留 synced.mp4 + 字幕)"
            fi
        done
        log "清理后磁盘: $(disk_gb)G"
    fi
}

wait_interval() {
    local last=""
    [ -f data/last_upload_ts ] && last=$(cat data/last_upload_ts)
    local now=$(date +%s)
    if [ -n "$last" ]; then
        local diff=$((now - last))
        if [ "$diff" -lt "$MIN_INTERVAL" ]; then
            local wait=$((MIN_INTERVAL - diff))
            log "⏳ 距上次投稿 ${diff}s，等待 ${wait}s..."
            sleep "$wait"
        fi
    fi
}

log "🚀 开始批量搬运 @NicolaiAI 近1个月视频（9个）"
log "──────────────────────────────"

for vid in $VIDS; do
    log "🎬 处理: $vid"
    # 跳过已投稿
    if grep -q "\"$vid\"" data/history/history.json 2>/dev/null; then
        log "ℹ️ $vid 之前已投稿，跳过"
        continue
    fi

    # 磁盘检查
    cleanup

    # 等待投稿间隔
    wait_interval

    success=0
    for attempt in 1 2 3; do
        log "  尝试 $attempt/3: submit $vid"
        if ./ytb submit "$vid" >> "$LOG" 2>&1; then
            # 确认真的投稿成功（history 有记录才算）
            if grep -q "\"$vid\"" data/history/history.json 2>/dev/null; then
                date +%s > data/last_upload_ts
                log "✅ 投稿成功: $vid"
                success=1
                break
            else
                log "⚠️ submit 退出0但 history 无记录，可能未投稿，重试"
            fi
        else
            log "❌ 尝试 $attempt 失败"
            sleep 30
        fi
    done
    [ "$success" -eq 0 ] && log "❌ $vid 三次尝试均失败，继续下一个"
done

log "🎉 批量搬运完成！"
