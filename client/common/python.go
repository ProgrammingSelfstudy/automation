package common

import (
	"context"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// pythonCacheEntry 缓存一次 Python 检测结果。
// 检测成功时永久缓存；失败时超过重试窗口后才重新探测，
// 避免启动时临时失败导致整个会话都无法使用 iOS 功能。
type pythonCacheEntry struct {
	command  string    // 实际可执行的 Python 路径；失败时为空字符串
	ok       bool      // 本次检测是否成功
	cachedAt time.Time // 检测时间；失败时用于计算是否到了重试窗口
}

// pythonFailRetryInterval 检测失败后的最短重试间隔。
// 间隔内的调用直接返回失败，不重复执行耗时的探测逻辑。
const pythonFailRetryInterval = 5 * time.Minute

var (
	pythonCommandMu    sync.Mutex
	pythonCommandCache *pythonCacheEntry
)

// PythonCommand 返回能运行 pymobiledevice3 的 Python 命令。
// 先真实探测环境，成功后永久缓存；失败时 5 分钟后允许重试；
// 如需指定虚拟环境，可设置 PERF_RABBIT_PYTHON。
func PythonCommand() (string, error) {
	pythonCommandMu.Lock()
	defer pythonCommandMu.Unlock()

	now := time.Now()
	if pythonCommandCache != nil {
		if pythonCommandCache.ok {
			// 检测成功，永久使用缓存结果，不重复探测。
			return pythonCommandCache.command, nil
		}
		if now.Sub(pythonCommandCache.cachedAt) < pythonFailRetryInterval {
			// 检测失败，尚未到重试窗口，直接返回缓存的失败结果。
			return "", exec.ErrNotFound
		}
		// 已过重试窗口，重新执行探测。
		log.Printf("Python 检测缓存已过期，重新探测（上次失败于 %s）", pythonCommandCache.cachedAt.Format("15:04:05"))
	}

	cmd := detectPythonCommand()
	pythonCommandCache = &pythonCacheEntry{
		command:  cmd,
		ok:       cmd != "",
		cachedAt: now,
	}

	if cmd == "" {
		return "", exec.ErrNotFound
	}
	return cmd, nil
}

func detectPythonCommand() string {
	if value := strings.TrimSpace(os.Getenv("PERF_RABBIT_PYTHON")); value != "" {
		pythonPath, ok := resolvePythonExecutable(value)
		if ok && ensurePymobiledevice3Ready(pythonPath) {
			return pythonPath
		}
		return ""
	}

	for _, command := range []string{"python", "python3", "py -3.12", "py -3.11", "py -3.10", "py -3.9", "py -3.8", "py -3"} {
		pythonPath, ok := resolvePythonExecutable(command)
		if ok && ensurePymobiledevice3Ready(pythonPath) {
			return pythonPath
		}
	}

	return ""
}

func resolvePythonExecutable(command string) (string, bool) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", false
	}

	path, err := exec.LookPath(parts[0])
	if err != nil {
		log.Printf("未找到 Python 命令: %s, error=%v", command, err)
		return "", false
	}
	log.Printf("发现 Python 命令: %s => %s", command, path)

	// WindowsApps 里的 python/python3 可能只是商店别名，不是真正解释器。
	// 这里让 Python 自己打印 sys.executable，拿到真实 exe 后再缓存。
	args := append(parts[1:], "-c", "import sys; print(sys.executable)")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
	if err != nil {
		log.Printf("Python 命令不可执行: %s, error=%v, output=%s", command, err, strings.TrimSpace(string(out)))
		return "", false
	}

	pythonPath := strings.TrimSpace(string(out))
	if pythonPath == "" {
		log.Printf("Python 命令未返回真实路径: %s", command)
		return "", false
	}

	// 用 pythonPath（sys.executable 解析出的真实路径）校验版本，不能用 path——
	// path 在 Windows 商店别名场景下就是上面特意绕过的那个假 stub，
	// 版本校验如果还传 path，等于把这段代码要规避的问题又带回来了。
	if !isPythonVersionSupported(pythonPath, parts[1:]) {
		return "", false
	}

	log.Printf("Python 真实路径: %s => %s", command, pythonPath)
	return pythonPath, true
}

func isPythonVersionSupported(command string, prefixArgs []string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := append([]string{}, prefixArgs...)
	args = append(args, "-c", "import sys; print('%d.%d' % sys.version_info[:2]); raise SystemExit(0 if sys.version_info >= (3, 8) else 1)")
	out, err := exec.CommandContext(ctx, command, args...).CombinedOutput()
	version := strings.TrimSpace(string(out))
	if err != nil {
		log.Printf("Python 版本过低或不可用: %s, version=%s, error=%v", command, version, err)
		return false
	}

	log.Printf("Python 版本可用: %s, version=%s", command, version)
	return true
}

func ensurePymobiledevice3Ready(command string) bool {
	if isPymobiledevice3Ready(command) {
		return true
	}
	if installPymobiledevice3(command) {
		return isPymobiledevice3Ready(command)
	}
	return false
}

func isPymobiledevice3Ready(command string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	log.Printf("检查 pymobiledevice3 命令: %s -m pymobiledevice3 --help", command)
	out, err := exec.CommandContext(ctx, command, "-m", "pymobiledevice3", "--help").CombinedOutput()
	if err != nil {
		log.Printf("pymobiledevice3 命令不可用: %s, error=%v, output=%s", command, err, strings.TrimSpace(string(out)))
	}
	return err == nil
}

func installPymobiledevice3(command string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	log.Printf("自动安装 pymobiledevice3: %s -m pip install --user pymobiledevice3", command)
	out, err := exec.CommandContext(ctx, command, "-m", "pip", "install", "--user", "pymobiledevice3").CombinedOutput()
	if err != nil {
		log.Printf("自动安装 pymobiledevice3 失败: %s, error=%v, output=%s", command, err, strings.TrimSpace(string(out)))
		return false
	}

	log.Printf("自动安装 pymobiledevice3 成功: %s", command)
	return true
}
