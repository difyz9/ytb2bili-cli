#!/usr/bin/env bash
# ============================================================
# Tech With Tim 频道 17 条视频 → B站 批量搬运调度器
# 特性：
#   - 每个视频完整流水线: download→transcribe→translate→metadata→tts→audio-sync→upload
#   - 投稿间隔 ≥20 分钟（防限流）
#   - 磁盘空间管理：低于阈值时清理已投稿视频的中间产物
#   - 幂等断点续跑：重启后跳过已完成步骤
#   - 日志: data/batch_carry.log
# ============================================================
set -u

cd /home/guan/guan/code/ytb2bili-cli
LOG="data/batch_carry.log"
MIN_INTERVAL=1200        # 投稿最小间隔 20 分钟（秒）
DISK_THRESHOLD=8         # 磁盘可用低于 8G 时清理
QUEUE_FILE="/tmp/twt_final.txt"

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG"; }

# 磁盘清理：删除已投稿视频的原始 mp4 + voice（保留 synced.mp4 和字幕）
cleanup_disk() {
  local avail_gb=$(df -BG / | awk 'NR==2 {gsub("G","",$4); print $4}')
  if [ "$avail_gb" -lt "$DISK_THRESHOLD" ]; then
    log "⚠️ 磁盘仅剩 ${avail_gb}G，清理已投稿视频的中间产物..."
    for d in data/downloads/*/; do
      [ -d "$d" ] || continue
      vid=$(basename "$d")
      # 检查该视频是否已投稿（history 或 synced 存在且任务完成）
      if [ -f "${d}${vid}.synced.mp4" ]; then
        rm -f "${d}${vid}.mp4" 2>/dev/null
        rm -rf "${d}voice" 2>/dev/null
        log "  清理 $vid 的原始视频和配音"
      fi
    done
    df -h / | tail -1 | tee -a "$LOG"
  fi
}

# 检查投稿间隔：距离上次投稿时间
last_upload_ts=0
if [ -f "data/last_upload_ts" ]; then
  last_upload_ts=$(cat data/last_upload_ts)
fi

now=$(date +%s)
elapsed=$(( now - last_upload_ts ))

if [ "$last_upload_ts" -ne 0 ] && [ "$elapsed" -lt "$MIN_INTERVAL" ]; then
  wait_s=$(( MIN_INTERVAL - elapsed ))
  log "⏳ 距上次投稿 ${elapsed}s，等待 ${wait_s}s 后开始（满足 20 分钟间隔）..."
  sleep "$wait_s"
fi

log "🚀 批量搬运开始：$(wc -l < "$QUEUE_FILE") 条视频"

while read -r line; do
  vid=$(echo "$line" | cut -d'|' -f1)
  title=$(echo "$line" | cut -d'|' -f4)
  log "──────────────"
  log "🎬 处理: $vid - $title"

  # 磁盘检查
  cleanup_disk

  # 完整流水线（幂等：已完成步骤自动跳过）
  log "📦 运行完整流水线: submit $vid"
  if ./ytb submit "https://www.youtube.com/watch?v=$vid" 2>&1 | tee -a "$LOG" | grep -qE "投稿成功|BVID="; then
    log "✅ 投稿成功: $vid"
    date +%s > data/last_upload_ts

    # 投稿后清理该视频的原始文件（保留 synced.mp4 + 字幕供字幕上传）
    rm -f "data/downloads/$vid/$vid.mp4" 2>/dev/null
    rm -rf "data/downloads/$vid/voice" 2>/dev/null
    log "  已清理 $vid 中间产物"

    # 投稿后等待 20 分钟再处理下一个
    log "⏳ 等待 ${MIN_INTERVAL}s（投稿间隔）..."
    sleep "$MIN_INTERVAL"
  else
    log "❌ 投稿失败或已存在: $vid"
    # 检查是否"已提交过"（重复投稿 = 之前已成功）
    if ./ytb submit "https://www.youtube.com/watch?v=$vid" 2>&1 | grep -q "已提交过"; then
      log "ℹ️ $vid 之前已投稿，跳过等待"
      date +%s > data/last_upload_ts
    else
      log "⚠️ $vid 处理失败，继续下一个"
      sleep 60
    fi
  fi
done < "$QUEUE_FILE"

log "🎉 批量搬运全部完成！"
