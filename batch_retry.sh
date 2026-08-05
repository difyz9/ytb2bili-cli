#!/usr/bin/env bash
# 恢复批量搬运：重试之前失败/中断的视频（幂等续跑）
# 失败原因：翻译 API 限流(5条) + DNS 超时(1条) + 下载403(1条) + 未完成(1条)
# 翻译 API 已恢复，现在重试
set -u
cd /home/guan/guan/code/ytb2bili-cli
LOG="data/batch_retry.log"
MIN_INTERVAL=1200  # 20 分钟

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG"; }

# 待重试的视频（按原顺序：Tv8mLrLtyxo 大文件最后传也行，这里按失败顺序）
RETRY_VIDS="Tv8mLrLtyxo 4JofSJIrjwU uCzXJKitKBs yRtUkbq_hHo MEfpHENH78c pZpoACNOPAw bFVa-lLlGYA 8JRJq4EEdik"

log "🚀 恢复批量搬运（${RETRY_VIDS}）"

last_upload_ts=0
[ -f "data/last_upload_ts" ] && last_upload_ts=$(cat data/last_upload_ts)
now=$(date +%s)
elapsed=$(( now - last_upload_ts ))
if [ "$last_upload_ts" -ne 0 ] && [ "$elapsed" -lt "$MIN_INTERVAL" ]; then
  wait_s=$(( MIN_INTERVAL - elapsed ))
  log "⏳ 距上次投稿 ${elapsed}s，等待 ${wait_s}s（满足 20 分钟间隔）..."
  sleep "$wait_s"
fi

for vid in $RETRY_VIDS; do
  log "──────────────"
  log "🎬 重试: $vid"
  
  # 重试机制：最多 3 次（翻译 API 限流时自动重试）
  attempt=1
  while [ $attempt -le 3 ]; do
    log "  尝试 $attempt/3: submit $vid"
    out=$(./ytb submit "https://www.youtube.com/watch?v=$vid" 2>&1 | tee -a "$LOG")
    
    if echo "$out" | grep -qE "投稿成功|BVID="; then
      log "✅ 投稿成功: $vid"
      date +%s > data/last_upload_ts
      # 清理中间产物
      rm -f "data/downloads/$vid/$vid.mp4" 2>/dev/null
      rm -rf "data/downloads/$vid/voice" 2>/dev/null
      break
    elif echo "$out" | grep -q "已提交过"; then
      log "ℹ️ $vid 之前已投稿，跳过"
      date +%s > data/last_upload_ts
      break
    else
      log "❌ 尝试 $attempt 失败"
      attempt=$((attempt+1))
      [ $attempt -le 3 ] && sleep 90
    fi
  done
  
  if [ $attempt -gt 3 ]; then
    log "⚠️ $vid 重试 3 次仍失败，跳过"
  else
    # 成功后才等待 20 分钟
    log "⏳ 等待 ${MIN_INTERVAL}s（投稿间隔）..."
    sleep "$MIN_INTERVAL"
  fi
done

log "🎉 恢复批量搬运完成！"
