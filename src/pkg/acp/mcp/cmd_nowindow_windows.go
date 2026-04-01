//go:build windows

package mcp

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW — 控制台子进程不分配/显示新控制台窗口（避免从 GUI 启动时闪黑框）。
const creationFlagsNoWindow = 0x08000000

func configureMCPCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: creationFlagsNoWindow,
	}
}
