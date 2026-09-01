# 重构前基线快照（2026-09-02）

- 分支: `baseline/pre-refactor`（commit 76004cb，测试修复后全绿）
- `ytb-help.txt`: `ytb --help` 完整输出（Phase 2/3 后用于 diff 对比，子命令清单应零变化）
- `go-test-results.txt`: `go test ./... -skip='Live'` 结果（17 包 ok）
