// Package diskspace 提供磁盘可用空间查询（用于 daemon 磁盘水位保护与产物清理统计）。
package diskspace

import (
	"fmt"
	"path/filepath"
	"syscall"
)

// FreeBytes 返回 path 所在文件系统对当前（非特权）用户可用的字节数。
// 路径不存在时向上回溯到最近的已存在父目录，尽量给出结果。
func FreeBytes(path string) (uint64, error) {
	target := path
	for {
		if target == "" {
			return 0, fmt.Errorf("无法解析路径: %s", path)
		}
		var st syscall.Statfs_t
		if err := syscall.Statfs(target, &st); err == nil {
			return st.Bavail * uint64(st.Bsize), nil
		}
		parent := filepath.Dir(target)
		if parent == target {
			return 0, fmt.Errorf("查询磁盘可用空间失败: %s", path)
		}
		target = parent
	}
}

// FreeGB 返回 path 所在文件系统可用空间（GB，1GB = 1024^3）。
func FreeGB(path string) (float64, error) {
	n, err := FreeBytes(path)
	if err != nil {
		return 0, err
	}
	return float64(n) / (1024 * 1024 * 1024), nil
}

// FormatBytes 人类可读的大小（B/KB/MB/GB）。
func FormatBytes(n uint64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.2f GB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
