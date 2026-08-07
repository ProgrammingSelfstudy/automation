package get

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"interface-load-test/client/common"
	"io"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const iosSysmonIntervalMS = 1000

type IOSCPUInfo struct {
	AppCpu float64 `json:"app_cpu"` // iOS 进程 CPU 使用率，对应 pymobiledevice3 的 cpuUsage
}

type IOSMemoryInfo struct {
	TotalPSS float64 `json:"total_pss"` // iOS 进程物理内存，单位 MB，对应 physFootprint
}

type iosSysmonSample struct {
	CPUUsage      float64 `json:"cpuUsage"`
	PhysFootprint string  `json:"physFootprint"`
}

type iosSysmonMonitor struct {
	cancel context.CancelFunc
	ready  chan struct{}
	once   sync.Once

	mu     sync.RWMutex
	latest *iosSysmonSample
	err    error
}

var iosSysmonMonitors sync.Map

// CollectIOSCPUAndMemory 从 iOS sysmon 后台进程取最新一秒 CPU/内存。
// 这里不每秒重新启动 pymobiledevice3，而是同一个任务复用一个后台进程，性能会稳定很多。
func CollectIOSCPUAndMemory(ctx context.Context, deviceID, processName, deviceModel string) (IOSCPUInfo, IOSMemoryInfo, error) {
	processName = strings.TrimSpace(processName)
	if processName == "" {
		return IOSCPUInfo{}, IOSMemoryInfo{}, fmt.Errorf("iOS process_name 不能为空")
	}

	monitor := loadIOSSysmonMonitor(deviceID, processName)

	select {
	case <-monitor.ready:
	case <-time.After(8 * time.Second):
		return IOSCPUInfo{}, IOSMemoryInfo{}, fmt.Errorf("等待 iOS sysmon 数据超时，process_name=%s", processName)
	case <-ctx.Done():
		return IOSCPUInfo{}, IOSMemoryInfo{}, ctx.Err()
	}

	monitor.mu.RLock()
	sample := monitor.latest
	lastErr := monitor.err
	monitor.mu.RUnlock()

	if sample == nil {
		if lastErr != nil {
			return IOSCPUInfo{}, IOSMemoryInfo{}, lastErr
		}
		return IOSCPUInfo{}, IOSMemoryInfo{}, fmt.Errorf("未获取到 iOS sysmon 数据，process_name=%s", processName)
	}

	memoryMB, err := parseIOSMemoryMB(sample.PhysFootprint)
	if err != nil {
		return IOSCPUInfo{}, IOSMemoryInfo{}, err
	}

	// pymobiledevice3 的 cpuUsage 是按整机核心累计的值，这里除以机型 CPU 核心数，和 Android 口径保持接近。
	coreCount := IOSCPUCoreCount(deviceModel)
	appCPU := math.Round(sample.CPUUsage/float64(coreCount)*100) / 100

	return IOSCPUInfo{AppCpu: appCPU}, IOSMemoryInfo{TotalPSS: memoryMB}, nil
}

func StopIOSSysmon(deviceID, processName string) {
	key := buildIOSSysmonKey(deviceID, processName)
	value, exists := iosSysmonMonitors.LoadAndDelete(key)
	if !exists {
		return
	}

	monitor, ok := value.(*iosSysmonMonitor)
	if ok && monitor.cancel != nil {
		monitor.cancel()
	}
}

func loadIOSSysmonMonitor(deviceID, processName string) *iosSysmonMonitor {
	key := buildIOSSysmonKey(deviceID, processName)
	if value, exists := iosSysmonMonitors.Load(key); exists {
		return value.(*iosSysmonMonitor)
	}

	// 在启动 goroutine 之前就创建 context 并把 cancel 写入结构体。
	// 如果 cancel 在 run() 内部赋值，StopIOSSysmon 可能在 goroutine 调度到之前读到 nil，
	// 造成 data race，也无法真正取消该采集进程。
	ctx, cancel := context.WithCancel(context.Background())
	monitor := &iosSysmonMonitor{ready: make(chan struct{}), cancel: cancel}

	actual, loaded := iosSysmonMonitors.LoadOrStore(key, monitor)
	if loaded {
		// 另一个协程抢先注册了相同 key，撤销我们创建的 context，返回已有的 monitor。
		cancel()
		return actual.(*iosSysmonMonitor)
	}

	go monitor.run(deviceID, processName, key, ctx)
	return monitor
}

func (m *iosSysmonMonitor) run(deviceID, processName, key string, ctx context.Context) {
	// ctx 和 cancel 已由 loadIOSSysmonMonitor 在 goroutine 启动前创建并赋值到结构体，
	// 这里直接使用，不再内部构造，消除与 StopIOSSysmon 之间的竞态。

	pythonCommand, pythonErr := common.PythonCommand()
	if pythonErr != nil {
		m.setError(errors.New("未检测到 Python 或 pymobiledevice3"))
		return
	}

	args := []string{
		"-m",
		"pymobiledevice3",
		"developer",
		"dvt",
		"sysmon",
		"process",
		"monitor",
		"process",
		"-f",
		"name=" + processName,
		"-k",
		"pid",
		"-k",
		"name",
		"-k",
		"cpuUsage",
		"-k",
		"physFootprint",
		"--human",
		"-i",
		strconv.Itoa(iosSysmonIntervalMS),
	}
	args = append(args, iosDVTConnectionArgs(deviceID)...)
	args = append(args, "--choose", "last")

	cmd := exec.CommandContext(ctx, pythonCommand, args...)
	// pymobiledevice3 在非交互 stdout 下会缓冲输出；关闭缓冲后才能每秒读到样本。
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.setError(fmt.Errorf("启动 iOS sysmon 失败: %w", err))
		return
	}
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		m.setError(fmt.Errorf("启动 iOS sysmon 失败: %w", err))
		return
	}

	if stderr != nil {
		go m.drainStderr(stderr)
	}
	m.readSamples(stdout)

	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		m.setError(fmt.Errorf("iOS sysmon 已退出: %w", err))
	}
	iosSysmonMonitors.Delete(key)
}

func iosDVTConnectionArgs(deviceID string) []string {
	// Windows 下 DVT 通道一般需要先启动 pymobiledevice3 remote tunneld，再通过 --tunnel 连接。
	if runtime.GOOS == "windows" {
		return []string{"--tunnel", deviceID}
	}
	return []string{"--userspace", "--udid", deviceID}
}

func (m *iosSysmonMonitor) readSamples(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var builder strings.Builder
	collecting := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "{") {
			builder.Reset()
			collecting = true
		}

		if !collecting {
			continue
		}

		builder.WriteString(line)
		builder.WriteByte('\n')

		if strings.HasPrefix(line, "}") {
			var sample iosSysmonSample
			if err := json.Unmarshal([]byte(builder.String()), &sample); err == nil {
				m.mu.Lock()
				m.latest = &sample
				m.err = nil
				m.mu.Unlock()
				m.once.Do(func() { close(m.ready) })
			}
			collecting = false
		}
	}

	if err := scanner.Err(); err != nil {
		m.setError(fmt.Errorf("读取 iOS sysmon 输出失败: %w", err))
	}
}

// drainStderr 消耗 stderr 避免管道满后阻塞整个采集进程。
// pymobiledevice3 会把普通警告（UserWarning、DeprecationWarning 等）写到 stderr，
// 这些不代表采集失败，不应调用 setError。
// 真正的进程退出错误由 run() 里的 cmd.Wait() 返回后统一处理。
func (m *iosSysmonMonitor) drainStderr(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		// 只消耗，不处理。
	}
}

func (m *iosSysmonMonitor) setError(err error) {
	m.mu.Lock()
	m.err = err
	m.mu.Unlock()
	m.once.Do(func() { close(m.ready) })
}

func buildIOSSysmonKey(deviceID, processName string) string {
	return strings.TrimSpace(deviceID) + "::" + strings.TrimSpace(processName)
}

func parseIOSMemoryMB(raw string) (float64, error) {
	text := strings.TrimSpace(strings.ToUpper(raw))
	if text == "" {
		return 0, fmt.Errorf("iOS physFootprint 为空")
	}

	numberText := text
	unit := "B"
	for _, item := range []string{"GB", "MB", "KB", "B"} {
		if strings.HasSuffix(text, item) {
			unit = item
			numberText = strings.TrimSpace(strings.TrimSuffix(text, item))
			break
		}
	}

	value, err := strconv.ParseFloat(numberText, 64)
	if err != nil {
		return 0, fmt.Errorf("解析 iOS physFootprint 失败: %s", raw)
	}

	switch unit {
	case "GB":
		value *= 1024
	case "KB":
		value /= 1024
	case "B":
		value /= 1024 * 1024
	}

	return math.Round(value*100) / 100, nil
}
