package perf

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"interface-load-test/perf-rabbit/client/common"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	TaskStatusCollecting  = "collecting"  // 采集中
	TaskStatusStopped     = "stopped"     // 已停止
	TaskStatusInterrupted = "interrupted" // 设备断开等异常导致采集中断
)

// MetricSample 是一次完整性能采集数据。
type MetricSample struct {
	CollectTime string      `json:"collect_time"` // 采集时间
	Data        interface{} `json:"data"`         // CPU、内存、FPS、卡顿等组合数据
}

// CollectFunc 是实际采集函数。
// 后面你在这里关联 CPU、内存、流量、FPS、卡顿的统一采集方法。
type CollectFunc func(
	ctx context.Context,
	deviceID string,
	packageName string,
	processName string,
	platform string,
	deviceModel string,
) (MetricSample, error)

// Task 是返回给接口和前端的任务数据。
type Task struct {
	TaskID         string         `json:"task_id"`            // 服务端生成的任务ID
	DeviceID       string         `json:"device_id"`          // Android 序列号 / iOS UDID
	PackageName    string         `json:"package_name"`       // Android 包名 / iOS Bundle ID
	ProcessName    string         `json:"process_name"`       // Android 等同包名；iOS 是进程名，例如 VoiceRoom
	Platform       string         `json:"platform"`           // android / ios
	DeviceModel    string         `json:"device_model"`       // iOS 用于查 CPU 核心数；Android 可为空
	Status         string         `json:"status"`             // collecting / stopped
	StartTime      string         `json:"start_time"`         // 开始时间
	SampleInterval int64          `json:"sample_interval_ms"` // 采集间隔，单位毫秒
	Samples        []MetricSample `json:"samples"`            // 已采集数据
	LastError      string         `json:"last_error"`         // 最近一次采集错误
}

// runningTask 是内部运行中的任务状态。
type runningTask struct {
	Task

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.RWMutex
}

// Manager 统一管理所有性能采集任务。
type Manager struct {
	mu       sync.RWMutex
	tasks    map[string]*runningTask // taskID -> 任务
	targets  map[string]string       // deviceID + packageName -> taskID
	interval time.Duration
	collect  CollectFunc
}

// DefaultManager 是默认性能任务管理器。
var DefaultManager = NewManager(time.Second, nil)

// NewManager 创建性能任务管理器。
func NewManager(interval time.Duration, collect CollectFunc) *Manager {
	if interval <= 0 {
		interval = time.Second
	}

	return &Manager{
		tasks:    make(map[string]*runningTask),
		targets:  make(map[string]string),
		interval: interval,
		collect:  collect,
	}
}

// SetCollector 设置实际采集函数。
// 项目启动时调用一次即可。
func (m *Manager) SetCollector(collect CollectFunc) {
	m.mu.Lock()
	m.collect = collect
	m.mu.Unlock()
}

// Start 创建并启动采集任务。
// 注意：调用方传 context.Background()，不要传 c.Request.Context()。
func (m *Manager) Start(
	parentCtx context.Context,
	deviceID string,
	packageName string,
	processName string,
	platform string,
	deviceModel string,
) (Task, error) {
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	if deviceID == "" {
		return Task{}, fmt.Errorf("device_id 不能为空")
	}

	if packageName == "" {
		return Task{}, fmt.Errorf("package_name 不能为空")
	}
	if processName == "" {
		processName = packageName
	}
	if platform == "" {
		platform = "android"
	}

	m.mu.Lock()

	if m.collect == nil {
		m.mu.Unlock()
		return Task{}, fmt.Errorf("未设置性能采集函数")
	}

	targetKey := buildTargetKey(deviceID, packageName)

	// 同一个设备、同一个包名只允许一个运行任务。
	if runningTaskID, exists := m.targets[targetKey]; exists {
		m.mu.Unlock()
		return Task{}, fmt.Errorf(
			"该应用正在采集，task_id=%s",
			runningTaskID,
		)
	}

	taskID, err := m.generateTaskIDLocked()
	if err != nil {
		m.mu.Unlock()
		return Task{}, fmt.Errorf("生成任务ID失败: %w", err)
	}

	taskCtx, cancel := context.WithCancel(parentCtx)

	task := &runningTask{
		Task: Task{
			TaskID:         taskID,
			DeviceID:       deviceID,
			PackageName:    packageName,
			ProcessName:    processName,
			Platform:       platform,
			DeviceModel:    deviceModel,
			Status:         TaskStatusCollecting,
			StartTime:      time.Now().Format("2006-01-02 15:04:05"),
			SampleInterval: m.interval.Milliseconds(),
			Samples:        make([]MetricSample, 0),
		},
		ctx:    taskCtx,
		cancel: cancel,
		done:   make(chan struct{}),
	}

	m.tasks[taskID] = task
	m.targets[targetKey] = taskID

	m.mu.Unlock()

	// Manager 已确认这是一个新任务后，再重置带状态的指标。
	// 不能放在 HTTP handler 里提前重置，否则重复启动失败也可能清掉正在运行任务的状态。
	ResetPerformanceMetrics(deviceID, packageName, processName)

	// 开始采集接口请求后，立刻执行第一条采集。
	// 成功后 task.Samples[0] 一定存在。
	if err := m.collectOnce(task); err != nil {
		m.removeStartFailedTask(task)

		return Task{}, fmt.Errorf("首次采集失败: %w", err)
	}

	// 首条数据采集完成后，再启动每秒循环。
	go m.run(task)

	taskSnapshot, _ := task.snapshot(0)
	return taskSnapshot, nil
}

// Stop 停止指定采集任务。
func (m *Manager) Stop(ctx context.Context, taskID string) (Task, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	m.mu.RLock()
	task, exists := m.tasks[taskID]
	m.mu.RUnlock()

	if !exists {
		return Task{}, fmt.Errorf("任务不存在，task_id=%s", taskID)
	}

	task.cancel()

	select {
	case <-task.done:
		taskSnapshot, _ := task.snapshot(0)
		return taskSnapshot, nil

	case <-ctx.Done():
		taskSnapshot, _ := task.snapshot(0)
		return taskSnapshot, fmt.Errorf("等待任务停止超时: %w", ctx.Err())
	}
}

// GetTask 查询任务状态和已采集数据。
// fromSample 为样本起始下标，前端轮询时可以只拉取新增数据。
func (m *Manager) GetTask(taskID string, fromSample int) (Task, int, error) {
	m.mu.RLock()
	task, exists := m.tasks[taskID]
	m.mu.RUnlock()

	if !exists {
		return Task{}, 0, fmt.Errorf("任务不存在，task_id=%s", taskID)
	}

	taskSnapshot, totalSamples := task.snapshot(fromSample)
	return taskSnapshot, totalSamples, nil
}

// run 每个任务一个 goroutine，每秒采集一次。
func (m *Manager) run(task *runningTask) {
	defer m.finish(task)

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-task.ctx.Done():
			return

		case <-ticker.C:
			// 普通采集失败只记录错误继续采；设备掉线必须立即中断任务。
			if err := m.collectOnce(task); err != nil &&
				!isDeviceOnline(task.DeviceID, task.Platform) {
				task.mu.Lock()
				task.Status = TaskStatusInterrupted
				task.LastError = "设备已断开，采集已自动中断: " + err.Error()
				task.mu.Unlock()

				return
			}
			DefaultHub.Notify(task.TaskID)
		}
	}
}

// collectOnce 执行一次实际性能采集。
func (m *Manager) collectOnce(task *runningTask) error {
	m.mu.RLock()
	collect := m.collect
	m.mu.RUnlock()

	if collect == nil {
		return fmt.Errorf("未设置性能采集函数")
	}

	sample, err := collect(
		task.ctx,
		task.DeviceID,
		task.PackageName,
		task.ProcessName,
		task.Platform,
		task.DeviceModel,
	)
	if err != nil {
		task.mu.Lock()
		task.LastError = err.Error()
		task.mu.Unlock()

		return err
	}

	if sample.CollectTime == "" {
		sample.CollectTime = time.Now().Format("2006-01-02 15:04:05")
	}

	task.mu.Lock()
	task.Samples = append(task.Samples, sample)
	task.LastError = ""
	task.mu.Unlock()

	return nil
}

// finish 任务退出后更新状态并释放设备应用占用。
func (m *Manager) finish(task *runningTask) {
	task.mu.Lock()
	if task.Status == TaskStatusCollecting {
		task.Status = TaskStatusStopped
	}
	status := task.Status
	task.mu.Unlock()

	// 设备掉线自动中断时，也要保存历史，前端历史页才能看到这次异常采集。
	if status == TaskStatusInterrupted {
		taskSnapshot, _ := task.snapshot(0)
		stopTime := time.Now().Format("2006-01-02 15:04:05")
		if err := SavePerfHistory(taskSnapshot, stopTime); err != nil {
			task.mu.Lock()
			task.LastError = task.LastError + "; 保存历史采集失败: " + err.Error()
			task.mu.Unlock()
		}
	}

	// 任务结束后释放指标内部状态，避免下一次任务读到旧累计值。
	ResetPerformanceMetrics(task.DeviceID, task.PackageName, task.ProcessName)

	targetKey := buildTargetKey(
		task.DeviceID,
		task.PackageName,
	)

	m.mu.Lock()

	if m.targets[targetKey] == task.TaskID {
		delete(m.targets, targetKey)
	}

	m.mu.Unlock()

	// 正常停止和设备掉线中断都会走到这里——WS 订阅者靠这次通知才知道任务
	// 结束了，不然掉线那种"提前 return，不走 ticker 分支"的路径永远不会
	// 唤醒还在等待的连接。
	DefaultHub.Notify(task.TaskID)

	close(task.done)

	// 任务结束后延迟 5 分钟再从内存中清除。
	// 这样前端在 stop 接口返回后还能再轮询一次确认 stopped 状态，
	// 而不是立即拿到"任务不存在"错误。
	// 不能立即 delete：stop handler 在调用 Stop() 之后还会调用 SavePerfHistory，
	// 如果此时任务已从 m.tasks 删除，后续 GetTask 就会失败。
	go func() {
		time.Sleep(5 * time.Minute)
		m.mu.Lock()
		delete(m.tasks, task.TaskID)
		m.mu.Unlock()
	}()
}

// snapshot 返回任务副本，避免接口读取时出现并发读写问题。
// fromSample 大于 0 时只复制新增样本，避免长时间采集后 GET 返回体越来越大。
func (task *runningTask) snapshot(fromSample int) (Task, int) {
	task.mu.RLock()
	defer task.mu.RUnlock()

	result := task.Task
	totalSamples := len(task.Samples)

	if fromSample < 0 {
		fromSample = 0
	}
	if fromSample > totalSamples {
		fromSample = totalSamples
	}

	result.Samples = append([]MetricSample(nil), task.Samples[fromSample:]...)

	return result, totalSamples
}

// removeStartFailedTask 清理首次采集失败的任务。
func (m *Manager) removeStartFailedTask(task *runningTask) {
	task.cancel()

	task.mu.Lock()
	task.Status = TaskStatusStopped
	task.mu.Unlock()

	// 首次采集失败也要清理内部状态，避免下次启动复用半初始化数据。
	ResetPerformanceMetrics(task.DeviceID, task.PackageName, task.ProcessName)

	targetKey := buildTargetKey(
		task.DeviceID,
		task.PackageName,
	)

	m.mu.Lock()
	delete(m.tasks, task.TaskID)

	if m.targets[targetKey] == task.TaskID {
		delete(m.targets, targetKey)
	}

	m.mu.Unlock()

	close(task.done)
}

// generateTaskIDLocked 在持有 Manager 锁时调用。
func (m *Manager) generateTaskIDLocked() (string, error) {
	for {
		randomBytes := make([]byte, 4)

		if _, err := rand.Read(randomBytes); err != nil {
			return "", err
		}

		taskID := fmt.Sprintf(
			"task_%s_%s",
			time.Now().Format("20060102_150405"),
			hex.EncodeToString(randomBytes),
		)

		if _, exists := m.tasks[taskID]; !exists {
			return taskID, nil
		}
	}
}

// buildTargetKey 同设备、同包名视为同一个采集目标。
func buildTargetKey(deviceID string, packageName string) string {
	return deviceID + "::" + packageName
}

// isDeviceOnline 判断目标设备是否仍在线。
// 采集命令失败后再查一次设备列表，避免把应用未运行误判成设备掉线。
func isDeviceOnline(deviceID string, platform string) bool {
	if strings.EqualFold(platform, "ios") {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		pythonCommand, pythonErr := common.PythonCommand()
		if pythonErr != nil {
			return false
		}

		out, err := exec.CommandContext(
			ctx,
			pythonCommand,
			"-m",
			"pymobiledevice3",
			"apps",
			"list",
			"--udid",
			deviceID,
			"--type",
			"User",
		).CombinedOutput()
		return err == nil && strings.HasPrefix(strings.TrimSpace(string(out)), "{")
	}

	output, err := exec.Command("adb", "devices", "-l").CombinedOutput()
	if err != nil {
		return false
	}

	for _, rawLine := range strings.Split(string(output), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "List of devices attached") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == deviceID {
			return fields[1] == "device"
		}
	}

	return false
}
