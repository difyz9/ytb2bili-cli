#!/usr/bin/env bash
# 等待批量搬运完成 → 自动验证 channel baseline 粉丝数获取（OOM 竞争解除后）
set -u
cd /home/guan/guan/code/ytb2bili-cli
LOG="data/batch_carry.log"

echo "[$(date '+%H:%M:%S')] 等待批量搬运完成（轮询 batch_carry 进程）..."
while pgrep -f "batch_carry.sh" > /dev/null 2>&1; do
  sleep 120
done
echo "[$(date '+%H:%M:%S')] ✅ 批量搬运已结束"

# 等内存完全释放
sleep 10
free -h | head -2

echo ""
echo "[$(date '+%H:%M:%S')] 验证粉丝数获取（OOM 竞争解除）..."
./ytb channel baseline UCgscS8mBsQZ5sFRkJIFWD7Q 2>&1 | head -6

echo ""
echo "[$(date '+%H:%M:%S')] 验证 nowcast 完整链路..."
./ytb auto --dry-run --scorer nowcast "robotics" 2>&1 | head -12
