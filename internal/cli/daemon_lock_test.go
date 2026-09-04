package cli

import (
	"testing"
)

// ─── P0：daemon 单实例锁 ────────────────────────────────────────────────────

func TestAcquireDaemonLockSingleInstance(t *testing.T) {
	dir := t.TempDir()

	// 第一次获取应成功
	release, err := acquireDaemonLock(dir)
	if err != nil {
		t.Fatalf("首次获取 daemon 锁失败: %v", err)
	}

	// 同一路径第二次获取（模拟第二个 daemon 实例）应失败
	release2, err := acquireDaemonLock(dir)
	if err == nil {
		release2()
		t.Fatal("第二个实例应获取锁失败")
	}
	if release2 != nil {
		release2()
	}

	// 释放后可再次获取（模拟旧实例退出后重启）
	release()
	release3, err := acquireDaemonLock(dir)
	if err != nil {
		t.Fatalf("释放后应能重新获取: %v", err)
	}
	release3()
}

// 不同数据目录互不影响（两个不同 daemon 各自独立）
func TestAcquireDaemonLockSeparateDirs(t *testing.T) {
	r1, err := acquireDaemonLock(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer r1()
	r2, err := acquireDaemonLock(t.TempDir())
	if err != nil {
		t.Fatalf("不同 data_dir 应互不干扰: %v", err)
	}
	r2()
}
