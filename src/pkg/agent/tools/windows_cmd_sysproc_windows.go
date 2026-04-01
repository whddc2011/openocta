//go:build windows

package tools

import (
	"os/exec"
	"syscall"
)

func applyExecNoWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}

	// 这是 Windows 下**彻底静默无窗口**的终极组合
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000 | 0x20000000 | 0x00000008,
	}
}
