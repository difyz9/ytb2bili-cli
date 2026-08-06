#!/usr/bin/env bash
# 重试 @NicolaiAI 剩余 7 个视频（WARP 修复后）
# 前 2 个已投稿（OjDeBzmGsl4, EdpWD1V2nq0）history 去重自动跳过
set -u
cd /home/guan/guan/code/ytb2bili-cli
LOG="data/batch_nicolai_retry.log"
MIN_INTERVAL=1200
MIN_DISK_GB=8
VIDS="aOpols4N1ao ImJiizbRf08 E7UkNupdRQw 83WzPqDvXgY Rtkrs6-uFB4 mqdIwY-AzbU UEUNw_NooYs"

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG"; }

disk_gb() { df / | awk 'NR==2 {print int($4/1024/1024)}'; }

cleanup() {
    local used=$(disk_gb)
    if [ "$used" -lt "$MIN_DISK_GB" ]; then
        log "⚠️ 磁盘仅 ${used}G，清理已投稿视频原始文件..."
        for d in data/downloads/*/; do
            [ -d "$d" ] || continue
            local id=$(basename "$d")
            if grep -q "\"$id\"" data/history/history.json 2>/dev/null; then
                rm -f "$d"*.mp4 "$d"*.webm "$d"*.part 2>/dev/null
                rm -rf "$d"voice 2>/dev/null
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

log "🚀 重试 @NicolaiAI 剩余 7 个视频（WARP 已修复）"
log "──────────────────────────────"

for vid in $VIDS; do
    log "🎬 处理: $vid"
    if grep -q "\"$vid\"" data/history/history.json 2>/dev/null; then
        log "ℹ️ $vid 已投稿，跳过"
        continue
    fi

    cleanup
    wait_interval

    success=0
    for attempt in 1 2 3; do
        log "  尝试 $attempt/3: submit $vid"
        if ./ytb submit "$vid" >> "$LOG" 2>&1; then
            if grep -q "\"$vid\"" data/history/history.json 2>/dev/null; then
                date +%s > data/last_upload_ts
                log "✅ 投稿成功: $vid"
                success=1
                break
            else
                log "⚠️ submit 退出0但 history 无记录，重试"
            fi
        else
            log "❌ 尝试 $attempt 失败"
            sleep 30
        fi
    done
    [ "$success" -eq 0 ] && log "❌ $vid 三次尝试均失败，继续下一个"
done

log "🎉 重试批量完成！"
