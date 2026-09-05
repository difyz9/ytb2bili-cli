package cli

import (
	"encoding/json"
	"fmt"
	"os"
)

// jsonMode 标记当前命令处于 --json 机器可读模式。
// 由各命令的 RunE 在入口处设置、结束（defer）时复位。命令串行执行，无需加锁。
var jsonMode bool

// outf 输出一条信息行。--json 模式下转到 stderr，保证 stdout 仅含 JSON 结果，
// 使命令可作为流水线/Agent 中的一个可解析步骤被调用。
func outf(format string, args ...interface{}) {
	if jsonMode {
		fmt.Fprintf(os.Stderr, format, args...)
		return
	}
	fmt.Printf(format, args...)
}

// emitJSON 向 stdout 输出缩进 JSON 结果对象（--json 机器可读模式）。
func emitJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
