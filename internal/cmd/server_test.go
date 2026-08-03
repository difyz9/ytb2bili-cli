package cmd

import "testing"

func TestServerDaemonCommand(t *testing.T) {
	cmd := serverDaemonCommand("")
	args := cmd.Args
	// Args[0] 是可执行文件路径，Args[1:] 是子命令参数
	if len(args) < 3 || args[1] != "server" || args[2] != "run" {
		t.Fatalf("args=%v, want [self server run ...]", args)
	}
}

func TestServerDaemonCommandWithAddr(t *testing.T) {
	cmd := serverDaemonCommand(":9000")
	found := false
	for i, a := range cmd.Args {
		if a == "--addr" && i+1 < len(cmd.Args) && cmd.Args[i+1] == ":9000" {
			found = true
		}
	}
	if !found {
		t.Fatalf("args=%v missing --addr :9000", cmd.Args)
	}
}

func TestServerCmdHasSubcommands(t *testing.T) {
	c := newServerCmd()
	names := map[string]bool{}
	for _, sub := range c.Commands() {
		names[sub.Name()] = true
	}
	for _, want := range []string{"start", "stop", "restart", "status", "run"} {
		if !names[want] {
			t.Fatalf("server missing %q subcommand", want)
		}
	}
}

func TestRootMovedServerManagementUnderServer(t *testing.T) {
	root := newRootCmd()
	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}
	for _, gone := range []string{"start", "stop", "restart", "status"} {
		if names[gone] {
			t.Fatalf("top-level %q should be removed (moved under server)", gone)
		}
	}
	if !names["server"] {
		t.Fatal("root missing server command")
	}
}
