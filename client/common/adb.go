package common

import (
	"context"
	"os/exec"
	"strings"
)

func AdbShell(serial, cmdStr string) (string, error) {
	return AdbShellContext(context.Background(), serial, cmdStr)
}

// AdbShellCommand 执行需要保留引号、空格、括号的 adb shell 命令。
// 例如 SurfaceFlinger Layer 名经常包含空格和 (BLAST)，不能用 strings.Fields 拆分。
func AdbShellCommand(serial, cmdStr string) (string, error) {
	return AdbShellCommandContext(context.Background(), serial, cmdStr)
}

// AdbShellContext 执行可取消的 adb shell 命令，便于性能采集停止时及时退出。
func AdbShellContext(ctx context.Context, serial, cmdStr string) (string, error) {
	// 组装adb参数：adb -s 设备号 shell 手机内部指令
	args := []string{"-s", serial, "shell"}
	// 把手机内部指令拆分成多个参数追加进去
	args = append(args, strings.Fields(cmdStr)...)
	// 执行系统adb命令
	cmd := exec.CommandContext(ctx, "adb", args...)
	// 捕获命令输出+错误
	out, err := cmd.CombinedOutput()
	// 去掉换行、前后空格，返回干净字符串
	return strings.TrimSpace(string(out)), err
}

// AdbShellCommandContext 执行整条 adb shell 命令，不拆分命令字符串。
func AdbShellCommandContext(ctx context.Context, serial, cmdStr string) (string, error) {
	cmd := exec.CommandContext(ctx, "adb", "-s", serial, "shell", cmdStr)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
