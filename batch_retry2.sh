#!/usr/bin/env bash
# 恢复批量搬运（第二批）：处理 batch_retry.sh 暂停时剩余的 4 个视频
# 使用修复后的代码（含语言校验防御：翻译结果/幂等/TTS 三层校验）
set -u
cd /home/guan/guan/code/ytb2bili-cli
LOG="data/batch_retry2.log"
MIN_INTERVAL=1200  # 20 分钟

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG"; }

RETRY_VIDS="MEfpHENH78c pZpoACNOPAw bFVa-lLlGYA 8JRJq4EEdik"

log "🚀 恢复批量搬运（第二批: ${RETRY_VIDS}）"

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

  attempt=1
  while [ $attempt -le 3 ]; do
    log "  尝试 $attempt/3: submit $vid"
    out=$(./ytb submit "https://www.youtube.com/watch?v=$vid" 2>&1 | tee -a "$LOG")

    if echo "$out" | grep -qE "投稿成功|BVID="; then
      log "✅ 投稿成功: $vid"
      date +%s > data/last_upload_ts
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
    log "⏳ 等待 ${MIN_INTERVAL}s（投稿间隔）..."
    sleep "$MIN_INTERVAL"
  fi
done

log "🎉 恢复批量搬运（第二批）完成！"
