// Package tools provides custom tools for the agent runtime.
package tools

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"strings"

	"github.com/stellarlinkco/agentsdk-go/pkg/tool"
)

// WindowsCmdTool executes shell commands on Windows via cmd.exe. Only available when the agent runs on Windows.
type WindowsCmdTool struct{}

// Name returns the tool name.
func (WindowsCmdTool) Name() string {
	return "windows_exec_cmd"
}

// Description returns the tool description.
func (WindowsCmdTool) Description() string {
	return "Execute a command or batch script on Windows using cmd.exe. Only works when the agent is running on Windows (runtime.GOOS=windows). Use cmd /c syntax for single commands, e.g. 'dir', 'echo hello', 'whoami'."
}

// Schema returns the parameter schema.
func (WindowsCmdTool) Schema() *tool.JSONSchema {
	return &tool.JSONSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Windows command to execute silently in background (no window, no flash). Supports dir, ipconfig, tasklist, etc.",
			},
		},
		Required: []string{"command"},
	}
}

// Execute runs the tool.
func (WindowsCmdTool) Execute(ctx context.Context, params map[string]interface{}) (*tool.ToolResult, error) {
	_ = ctx

	if runtime.GOOS != "windows" {
		return &tool.ToolResult{
			Success: false,
			Output:  "windows_exec_cmd: this tool only works on Windows. Current OS is " + runtime.GOOS + ".",
		}, nil
	}

	cmdStr, _ := params["command"].(string)
	cmdStr = strings.TrimSpace(cmdStr)
	if cmdStr == "" {
		return &tool.ToolResult{Success: false, Output: "command is required"}, nil
	}

	// 👇 这一行是关键：改用 powershell 无窗口执行
	// 支持所有 cmd 命令：dir, ipconfig, tasklist 等全部兼容
	cmd := exec.Command(
		"powershell",
		"-NoProfile",
		"-NonInteractive",
		"-WindowStyle", "Hidden", // 强制无窗口
		"-Command", cmdStr,
	)

	// 👇 额外再加一层系统级隐藏（双重保险）
	applyExecNoWindow(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	outStr := strings.TrimSpace(stdout.String())
	errStr := strings.TrimSpace(stderr.String())

	if err != nil {
		combined := outStr
		if errStr != "" {
			if combined != "" {
				combined += "\n"
			}
			combined += errStr
		}
		if combined == "" {
			combined = err.Error()
		}
		return &tool.ToolResult{Success: false, Output: combined}, nil
	}

	output := outStr
	if errStr != "" {
		output = outStr + "\n" + errStr
	}
	if output == "" {
		output = "(command completed with no output)"
	}
	return &tool.ToolResult{Success: true, Output: output}, nil
}
