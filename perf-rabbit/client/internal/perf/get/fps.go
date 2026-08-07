package get

import (
	"fmt"
	"interface-load-test/perf-rabbit/client/common"
	"math"
	"strconv"
	"strings"
	"sync"
)

const (
	fpsMethodSurfaceFlinger = "surfaceflinger"
	fpsMethodGfxInfo        = "gfxinfo"

	fpsTimeSourceActualPresent  = "actual_present_time"
	fpsTimeSourceDisplayPresent = "display_present_time"
	fpsTimeSourceFrameCompleted = "frame_completed"

	maxSurfacePresentTime int64 = 9223372036854775807
)

// FPSInfo 是 FPS 采集结果。
type FPSInfo struct {
	FPS          float64 `json:"fps"`            // 当前 FPS
	Frames       int     `json:"frames"`         // 本次新增有效帧数量
	Method       string  `json:"method"`         // surfaceflinger 或 gfxinfo
	Layer        string  `json:"layer"`          // SurfaceFlinger 实际命中的 Layer；gfxinfo 时为空
	RefreshRate  float64 `json:"refresh_rate"`   // 当前屏幕刷新率，例如 60、120；gfxinfo 时为 0
	HasFrameData bool    `json:"has_frame_data"` // 本次是否有新增帧
	TimeSource   string  `json:"time_source"`    // FPS 使用的时间源
}

type fpsState struct {
	Initialized bool // 新任务第一次调用时会清空历史帧，避免把旧帧算进当前任务

	SurfaceCursors map[string]int64 // 每个 SurfaceFlinger Layer 最后处理到的呈现时间

	LastGfxDisplayPresentTimestamp int64 // gfxinfo DisplayPresentTime 游标
	LastGfxFrameCompletedTimestamp int64 // gfxinfo FrameCompleted 游标
}

var (
	fpsMu       sync.Mutex
	fpsStateMap = make(map[string]fpsState)
)

// ResetFPS 清空指定设备、应用的 FPS 采集状态。
// 下次调用 CollectFPS 时会重新清空历史帧。
func ResetFPS(deviceID, packageName string) {
	key := buildFPSKey(deviceID, packageName)

	fpsMu.Lock()
	delete(fpsStateMap, key)
	fpsMu.Unlock()
}

// CollectFPS 读取新增帧计算 FPS。
// 内部不 sleep，外层管理器按固定频率调用即可。
func CollectFPS(deviceID, packageName string) (interface{}, error) {
	key := buildFPSKey(deviceID, packageName)

	candidateLayers, _ := getSurfaceLayers(deviceID, packageName)

	if err := initFPSStateIfNeed(key, deviceID, packageName, candidateLayers); err != nil {
		return nil, err
	}

	if len(candidateLayers) > 0 {
		info, ok := collectSurfaceFlingerFPS(key, deviceID, candidateLayers)
		if ok {
			return info, nil
		}
	}

	return collectGfxInfoFPS(key, deviceID, packageName)
}

func buildFPSKey(deviceID, packageName string) string {
	return deviceID + "_" + packageName
}

// initFPSStateIfNeed 只在任务第一次采样时清空历史帧。
// 不能每秒 reset，否则 FPS 永远需要等待一段采样时间，会拖慢整个采集循环。
func initFPSStateIfNeed(
	key string,
	deviceID string,
	packageName string,
	candidateLayers []string,
) error {
	fpsMu.Lock()
	state := fpsStateMap[key]
	if state.SurfaceCursors == nil {
		state.SurfaceCursors = make(map[string]int64)
	}
	needInit := !state.Initialized
	fpsStateMap[key] = state
	fpsMu.Unlock()

	if !needInit {
		return nil
	}

	for _, layer := range candidateLayers {
		_, _ = common.AdbShellCommand(
			deviceID,
			fmt.Sprintf(
				"dumpsys SurfaceFlinger --latency-clear %s",
				shellQuote(layer),
			),
		)
	}

	resetOutput, err := common.AdbShell(
		deviceID,
		fmt.Sprintf("dumpsys gfxinfo %s reset", packageName),
	)
	if err != nil && strings.TrimSpace(resetOutput) == "" {
		return fmt.Errorf("重置 gfxinfo framestats 失败: %w", err)
	}

	if strings.Contains(resetOutput, "No process found") {
		return fmt.Errorf("应用进程不存在，package_name=%s", packageName)
	}

	fpsMu.Lock()
	state = fpsStateMap[key]
	if state.SurfaceCursors == nil {
		state.SurfaceCursors = make(map[string]int64)
	}
	for _, layer := range candidateLayers {
		// -1 表示新任务第一轮只建立游标，不把历史帧算进当前任务。
		state.SurfaceCursors[layer] = -1
	}
	state.Initialized = true
	fpsStateMap[key] = state
	fpsMu.Unlock()

	return nil
}

// collectSurfaceFlingerFPS 通过 SurfaceFlinger 的新增 ActualPresentTime 计算 FPS。
func collectSurfaceFlingerFPS(
	key string,
	deviceID string,
	candidateLayers []string,
) (FPSInfo, bool) {
	oldCursors := copySurfaceCursors(key)

	latestCursors := make(map[string]int64)
	bestLayer := ""
	bestNewTimes := make([]int64, 0)
	bestOldCursor := int64(0)
	bestRefreshPeriodNs := int64(0)

	for _, layer := range candidateLayers {
		latencyOutput, err := common.AdbShellCommand(
			deviceID,
			fmt.Sprintf(
				"dumpsys SurfaceFlinger --latency %s",
				shellQuote(layer),
			),
		)
		if err != nil {
			continue
		}

		frameTimes, refreshPeriodNs := parseSurfaceLatency(latencyOutput)
		oldCursor, knownLayer := oldCursors[layer]
		latestCursors[layer] = advanceCursor(maxInt64(oldCursor, 0), frameTimes)

		// 新出现的 Layer 先推进游标，不立刻计算，避免把历史帧算进当前秒。
		if !knownLayer || oldCursor < 0 {
			continue
		}

		newTimes := getNewTimestamps(frameTimes, oldCursor)
		if len(newTimes) > len(bestNewTimes) {
			bestLayer = layer
			bestNewTimes = newTimes
			bestOldCursor = oldCursor
			bestRefreshPeriodNs = refreshPeriodNs
		}
	}

	saveSurfaceCursors(key, latestCursors)

	if len(bestNewTimes) == 0 {
		return FPSInfo{}, false
	}

	return FPSInfo{
		FPS:          calculateFPSFromTimestamps(bestOldCursor, bestNewTimes),
		Frames:       len(bestNewTimes),
		Method:       fpsMethodSurfaceFlinger,
		Layer:        bestLayer,
		RefreshRate:  calculateRefreshRate(bestRefreshPeriodNs),
		HasFrameData: true,
		TimeSource:   fpsTimeSourceActualPresent,
	}, true
}

// collectGfxInfoFPS 在 SurfaceFlinger 不可用时，使用 gfxinfo 新增帧兜底。
func collectGfxInfoFPS(key, deviceID, packageName string) (FPSInfo, error) {
	output, err := common.AdbShell(
		deviceID,
		fmt.Sprintf("dumpsys gfxinfo %s framestats", packageName),
	)
	if err != nil && strings.TrimSpace(output) == "" {
		return FPSInfo{}, fmt.Errorf("获取 gfxinfo framestats 失败: %w", err)
	}

	if strings.Contains(output, "No process found") {
		return FPSInfo{}, fmt.Errorf("应用进程不存在，package_name=%s", packageName)
	}

	timestampSet := parseFrameTimestamps(output)
	if !timestampSet.FoundHeader {
		return FPSInfo{}, fmt.Errorf("未找到 gfxinfo framestats 表头，package_name=%s", packageName)
	}

	fpsMu.Lock()
	state := fpsStateMap[key]

	oldDisplayCursor := state.LastGfxDisplayPresentTimestamp
	oldFrameCompletedCursor := state.LastGfxFrameCompletedTimestamp

	displayNewTimes := getNewTimestamps(
		timestampSet.DisplayPresentTimes,
		oldDisplayCursor,
	)
	frameCompletedNewTimes := getNewTimestamps(
		timestampSet.FrameCompletedTimes,
		oldFrameCompletedCursor,
	)

	state.LastGfxDisplayPresentTimestamp = advanceCursor(
		oldDisplayCursor,
		timestampSet.DisplayPresentTimes,
	)
	state.LastGfxFrameCompletedTimestamp = advanceCursor(
		oldFrameCompletedCursor,
		timestampSet.FrameCompletedTimes,
	)

	fpsStateMap[key] = state
	fpsMu.Unlock()

	currentTimes := displayNewTimes
	oldCursor := oldDisplayCursor
	timeSource := fpsTimeSourceDisplayPresent

	if len(currentTimes) == 0 {
		currentTimes = frameCompletedNewTimes
		oldCursor = oldFrameCompletedCursor
		timeSource = fpsTimeSourceFrameCompleted
	}

	if len(currentTimes) == 0 {
		return FPSInfo{
			FPS:          0,
			Frames:       0,
			Method:       fpsMethodGfxInfo,
			Layer:        "",
			RefreshRate:  0,
			HasFrameData: false,
			TimeSource:   timeSource,
		}, nil
	}

	return FPSInfo{
		FPS:          calculateFPSFromTimestamps(oldCursor, currentTimes),
		Frames:       len(currentTimes),
		Method:       fpsMethodGfxInfo,
		Layer:        "",
		RefreshRate:  0,
		HasFrameData: true,
		TimeSource:   timeSource,
	}, nil
}

func getSurfaceLayers(deviceID, packageName string) ([]string, error) {
	layerOutput, err := common.AdbShell(deviceID, "dumpsys SurfaceFlinger --list")
	if err != nil {
		return nil, err
	}

	candidateLayers := make([]string, 0)
	layerMap := make(map[string]bool)

	for _, rawLine := range strings.Split(layerOutput, "\n") {
		layer := normalizeSurfaceLayerName(rawLine)
		if layer == "" {
			continue
		}

		if !strings.Contains(layer, packageName) {
			continue
		}

		// 这些通常不是实际绘制 Layer，不参与 FPS 候选。
		if strings.Contains(layer, "ActivityRecordInputSink") ||
			strings.Contains(layer, "Background for ") ||
			strings.Contains(layer, "ActivityRecord{") ||
			strings.Contains(layer, "Bounds for -") {
			continue
		}

		if layerMap[layer] {
			continue
		}

		layerMap[layer] = true
		candidateLayers = append(candidateLayers, layer)
	}

	return candidateLayers, nil
}

// normalizeSurfaceLayerName 从 dumpsys SurfaceFlinger --list 的输出中提取真实 Layer 名。
// 部分 Android 版本返回 RequestedLayerState{真实Layer parentId=...}，不能把整行拿去查 latency。
func normalizeSurfaceLayerName(rawLine string) string {
	layer := strings.TrimSpace(rawLine)
	if layer == "" {
		return ""
	}

	if strings.HasPrefix(layer, "RequestedLayerState{") {
		layer = strings.TrimPrefix(layer, "RequestedLayerState{")

		cutIndex := len(layer)
		for _, marker := range []string{
			" parentId=",
			" relativeParentId=",
			" z=",
		} {
			if index := strings.Index(layer, marker); index >= 0 && index < cutIndex {
				cutIndex = index
			}
		}

		layer = layer[:cutIndex]
		layer = strings.TrimSuffix(layer, "}")
	}

	return strings.TrimSpace(layer)
}

// parseSurfaceLatency 解析 dumpsys SurfaceFlinger --latency 输出。
// 第一行是刷新周期，后续三列里第二列是 ActualPresentTime。
func parseSurfaceLatency(output string) ([]int64, int64) {
	lines := strings.Split(output, "\n")
	if len(lines) == 0 {
		return nil, 0
	}

	refreshPeriodNs, _ := strconv.ParseInt(strings.TrimSpace(lines[0]), 10, 64)
	frameTimes := make([]int64, 0)

	for _, rawLine := range lines[1:] {
		fields := strings.Fields(rawLine)
		if len(fields) != 3 {
			continue
		}

		actualPresentTime, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil ||
			actualPresentTime <= 0 ||
			actualPresentTime == maxSurfacePresentTime {
			continue
		}

		frameTimes = append(frameTimes, actualPresentTime)
	}

	return normalizeTimestamps(frameTimes), refreshPeriodNs
}

func copySurfaceCursors(key string) map[string]int64 {
	fpsMu.Lock()
	defer fpsMu.Unlock()

	state := fpsStateMap[key]
	result := make(map[string]int64, len(state.SurfaceCursors))
	for layer, cursor := range state.SurfaceCursors {
		result[layer] = cursor
	}

	return result
}

func saveSurfaceCursors(key string, latestCursors map[string]int64) {
	fpsMu.Lock()
	defer fpsMu.Unlock()

	state := fpsStateMap[key]
	if state.SurfaceCursors == nil {
		state.SurfaceCursors = make(map[string]int64)
	}

	for layer, cursor := range latestCursors {
		if cursor > state.SurfaceCursors[layer] {
			state.SurfaceCursors[layer] = cursor
		}
	}

	fpsStateMap[key] = state
}

// calculateFPSFromTimestamps 根据新增帧时间戳计算当前窗口 FPS。
func calculateFPSFromTimestamps(oldCursor int64, frameTimes []int64) float64 {
	if len(frameTimes) == 0 {
		return 0
	}

	lastTime := frameTimes[len(frameTimes)-1]
	frameCount := len(frameTimes)
	durationNs := int64(0)

	if oldCursor > 0 && lastTime > oldCursor {
		durationNs = lastTime - oldCursor
	} else if len(frameTimes) >= 2 && lastTime > frameTimes[0] {
		frameCount = len(frameTimes) - 1
		durationNs = lastTime - frameTimes[0]
	}

	if frameCount <= 0 || durationNs <= 0 {
		return 0
	}

	fps := float64(frameCount) * 1_000_000_000 / float64(durationNs)
	return round2(fps)
}

func calculateRefreshRate(refreshPeriodNs int64) float64 {
	if refreshPeriodNs <= 0 {
		return 0
	}

	return round2(1_000_000_000 / float64(refreshPeriodNs))
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}

	return right
}

// shellQuote 给 Layer 名称加 Shell 单引号，避免括号、空格、# 等字符影响命令。
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
