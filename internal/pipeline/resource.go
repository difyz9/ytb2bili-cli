package pipeline

import (
	"os"
	"path/filepath"
	"runtime"
)

// ProjectRoot 解析项目根目录（含 skills/、.venv/ 等随仓库分发的资源）。
// 优先级：
//  1. $YTB2BILI_PROJECT_DIR（显式指定，最可靠）
//  2. 可执行文件所在目录（若其下存在 skills/）
//  3. 当前工作目录（若其下存在 skills/）
//  4. 源码位置回溯（go run / 测试场景：internal/pipeline/ → 项目根）
//
// 这样二进制无论安装到何处、从哪个目录调用，都能定位到 skills/ 与 .venv/，
// 使 CLI 不局限于"必须在项目目录里运行"。
func ProjectRoot() string {
	if p := os.Getenv("YTB2BILI_PROJECT_DIR"); p != "" {
		return p
	}
	if exe, err := os.Executable(); err == nil {
		if dir := filepath.Dir(exe); hasSkillsDir(dir) {
			return dir
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if hasSkillsDir(cwd) {
			return cwd
		}
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	}
	return "."
}

func hasSkillsDir(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, "skills"))
	return err == nil && fi.IsDir()
}

// SkillScript 返回 skills/<rel> 下脚本的绝对路径。
func SkillScript(rel string) string {
	return filepath.Join(ProjectRoot(), "skills", rel)
}

// VenvPython 返回项目 .venv 下的 python3 解释器路径。
// 优先级：$YTB2BILI_PYTHON > <项目根>/.venv/bin/python3 > PATH 中的 python3。
func VenvPython() string {
	if p := os.Getenv("YTB2BILI_PYTHON"); p != "" {
		return p
	}
	p := filepath.Join(ProjectRoot(), ".venv", "bin", "python3")
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		return p
	}
	return "python3"
}
