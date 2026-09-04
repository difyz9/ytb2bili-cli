package server

import "os"

// writeBrokenJSON 向 path 写入损坏 JSON（测试用）。
func writeBrokenJSON(path string) error {
	return os.WriteFile(path, []byte(`{"id":`), 0644)
}
