package get

import (
	"client/common"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
)

const (
	filmFrameMs      = 1000.0 / 24.0 // 41.67ms，一帧电影帧耗时
	jankThresholdMs  = filmFrameMs * 2
	bigJankThreshold = filmFrameMs * 3

	timeSourceDisplayPresent = "display_present_time"
	timeSourceFrameCompleted = "frame_completed"
	timeSourceUnavailable    = "unavailable"

	maxInvalidTimestamp int64 = 1<<63 - 1
)

type JankInfo struct {
	SmallJank      int     `json:"small_jank"`        // 本次 SmallJank 次数
	Jank           int     `json:"jank"`              // 本次 Jank 次数
	BigJank        int     `json:"big_jank"`          // 本次 BigJank 次数
	TotalSmallJank int     `json:"total_small_jank"`  // 当前任务累计 SmallJank
	TotalJank      int     `json:"total_jank"`        // 当前任务累计 Jank
	TotalBigJank   int     `json:"total_big_jank"`    // 当前任务累计 BigJank
	Frames         int     `json:"frames"`            // 本次新增有效帧数量
	MaxFrameTimeMs float64 `json:"max_frame_time_ms"` // 本次最大 FTime，单位 ms
	HasFrameData   bool    `json:"has_frame_data"`    // 是否有可计算的 FTime
	Method         string  `json:"method"`            // 固定 gfxinfo
	TimeSource     string  `json:"time_source"`       // display_present_time / frame_completed / unavailable
}

type jankState struct {
	Initialized bool // 是否已经执行过 gfxinfo reset

	ActiveTimeSource string // 当前正在使用的时间源

	LastDisplayPresentTimestamp int64 // DisplayPresentTime 最后处理时间戳
	LastFrameCompletedTimestamp int64 // FrameCompleted 最后处理时间戳

	LastThreeFTimes []float64 // 当前时间源最近三个 FTime，切换时间源时清空

	TotalSmallJank int
	TotalJank      int
	TotalBigJank   int
}

type frameTimestampSet struct {
	DisplayPresentTimes []int64
	FrameCompletedTimes []int64
	FoundHeader         bool
}

var (
	jankMu       sync.Mutex
	jankStateMap = make(map[string]jankState)
)

// ResetJankTotal 清空指定设备、应用的卡顿采集状态。
// 下次调用 CollectJank 时会重新执行一次 gfxinfo reset。
func ResetJankTotal(deviceID, packageName string) {
	key := deviceID + "_" + packageName

	jankMu.Lock()
	delete(jankStateMap, key)
	jankMu.Unlock()
}

// CollectJank 读取当前 framestats 的新增帧并计算卡顿。
// 内部不等待，外层按每秒一次的频率调用即可。
func CollectJank(deviceID, packageName string) (interface{}, error) {
	key := deviceID + "_" + packageName

	jankMu.Lock()
	state, exists := jankStateMap[key]
	initialized := exists && state.Initialized
	jankMu.Unlock()

	// 新任务只 reset 一次。
	if !initialized {
		resetOutput, _ := common.AdbShell(
			deviceID,
			fmt.Sprintf("dumpsys gfxinfo %s reset", packageName),
		)

		if strings.Contains(resetOutput, "No process found") {
			return nil, fmt.Errorf("应用进程不存在，package_name=%s", packageName)
		}

		jankMu.Lock()
		jankStateMap[key] = jankState{
			Initialized: true,
		}
		jankMu.Unlock()
	}

	output, err := common.AdbShell(
		deviceID,
		fmt.Sprintf("dumpsys gfxinfo %s framestats", packageName),
	)

	// 某些 ROM 即使有有效输出，也会返回 exit status 1。
	if err != nil && strings.TrimSpace(output) == "" {
		return nil, fmt.Errorf("获取 gfxinfo framestats 失败: %w", err)
	}

	if strings.Contains(output, "No process found") {
		return nil, fmt.Errorf("应用进程不存在，package_name=%s", packageName)
	}

	timestampSet := parseFrameTimestamps(output)

	if !timestampSet.FoundHeader {
		return nil, fmt.Errorf(
			"未找到 gfxinfo framestats 表头，package_name=%s",
			packageName,
		)
	}

	jankMu.Lock()
	defer jankMu.Unlock()

	state = jankStateMap[key]

	// 保存本轮开始前的游标。
	// 计算跨秒 FTime 必须使用旧游标，不能使用推进后的新游标。
	oldDisplayCursor := state.LastDisplayPresentTimestamp
	oldFrameCompletedCursor := state.LastFrameCompletedTimestamp

	// 分别取两套时间源本轮新增的数据。
	displayNewTimes := getNewTimestamps(
		timestampSet.DisplayPresentTimes,
		oldDisplayCursor,
	)

	frameCompletedNewTimes := getNewTimestamps(
		timestampSet.FrameCompletedTimes,
		oldFrameCompletedCursor,
	)

	// 无论本轮最终使用哪个来源，两个游标都必须推进。
	// 否则以后回退到备用来源时，会重放此前积压的旧帧。
	state.LastDisplayPresentTimestamp = advanceCursor(
		oldDisplayCursor,
		timestampSet.DisplayPresentTimes,
	)

	state.LastFrameCompletedTimestamp = advanceCursor(
		oldFrameCompletedCursor,
		timestampSet.FrameCompletedTimes,
	)

	// 当前轮优先使用 DisplayPresentTime。
	currentSource := timeSourceUnavailable
	currentTimes := make([]int64, 0)

	if len(displayNewTimes) > 0 {
		currentSource = timeSourceDisplayPresent
		currentTimes = displayNewTimes
	} else if len(frameCompletedNewTimes) > 0 {
		currentSource = timeSourceFrameCompleted
		currentTimes = frameCompletedNewTimes
	}

	// SurfaceView 视频等场景，gfxinfo 可能没有可用 UI 帧。
	if currentSource == timeSourceUnavailable {
		jankStateMap[key] = state

		lastSource := state.ActiveTimeSource
		if lastSource == "" {
			lastSource = timeSourceUnavailable
		}

		return JankInfo{
			TotalSmallJank: state.TotalSmallJank,
			TotalJank:      state.TotalJank,
			TotalBigJank:   state.TotalBigJank,
			Frames:         0,
			MaxFrameTimeMs: 0,
			HasFrameData:   false,
			Method:         "gfxinfo",
			TimeSource:     lastSource,
		}, nil
	}

	// 时间源切换时，不能混用上一套时间源的前三帧 FTime。
	sourceChanged := state.ActiveTimeSource != "" &&
		state.ActiveTimeSource != currentSource

	if sourceChanged {
		state.LastThreeFTimes = nil
	}

	// 未切换来源时，才使用同源旧游标计算跨秒第一段 FTime。
	previousTimestamp := int64(0)

	if !sourceChanged {
		if currentSource == timeSourceDisplayPresent {
			previousTimestamp = oldDisplayCursor
		} else if currentSource == timeSourceFrameCompleted {
			previousTimestamp = oldFrameCompletedCursor
		}
	}

	state.ActiveTimeSource = currentSource

	frameTimes := make([]float64, 0)
	maxFrameTimeMs := 0.0
	lastTimestamp := previousTimestamp

	for _, timestamp := range currentTimes {
		if lastTimestamp > 0 && timestamp > lastTimestamp {
			diffNs := timestamp - lastTimestamp

			// 不过滤 >= 1 秒。
			// 已经过 IntendedVsync、非零、单调递增校验，
			// 大间隔可能是真实冻结，也应参与 BigJank 判断。
			if diffNs > 0 {
				frameTimeMs := float64(diffNs) / 1_000_000

				frameTimes = append(frameTimes, frameTimeMs)

				if frameTimeMs > maxFrameTimeMs {
					maxFrameTimeMs = frameTimeMs
				}
			}
		}

		lastTimestamp = timestamp
	}

	smallJank := 0
	jank := 0
	bigJank := 0

	// 保留上一轮最后三个 FTime，避免跨秒漏掉边界卡顿。
	historyFTimes := append([]float64{}, state.LastThreeFTimes...)

	for _, currentFrameTimeMs := range frameTimes {
		if len(historyFTimes) >= 3 {
			previousAverageMs := (historyFTimes[len(historyFTimes)-1] +
				historyFTimes[len(historyFTimes)-2] +
				historyFTimes[len(historyFTimes)-3]) / 3

			// 三类卡顿共用相对条件：
			// 当前 FTime > 前三帧平均 FTime 的 2 倍。
			if previousAverageMs > 0 &&
				currentFrameTimeMs > previousAverageMs*2 {
				if currentFrameTimeMs > filmFrameMs {
					smallJank++
				}

				if currentFrameTimeMs > jankThresholdMs {
					jank++
				}

				if currentFrameTimeMs > bigJankThreshold {
					bigJank++
				}
			}
		}

		historyFTimes = append(historyFTimes, currentFrameTimeMs)

		if len(historyFTimes) > 3 {
			historyFTimes = historyFTimes[len(historyFTimes)-3:]
		}
	}

	state.LastThreeFTimes = historyFTimes
	state.TotalSmallJank += smallJank
	state.TotalJank += jank
	state.TotalBigJank += bigJank

	jankStateMap[key] = state

	return JankInfo{
		SmallJank:      smallJank,
		Jank:           jank,
		BigJank:        bigJank,
		TotalSmallJank: state.TotalSmallJank,
		TotalJank:      state.TotalJank,
		TotalBigJank:   state.TotalBigJank,
		Frames:         len(currentTimes),
		MaxFrameTimeMs: math.Round(maxFrameTimeMs*100) / 100,
		HasFrameData:   len(frameTimes) > 0,
		Method:         "gfxinfo",
		TimeSource:     currentSource,
	}, nil
}

// parseFrameTimestamps 动态解析 framestats 表头。
// 每一条帧记录都要求时间戳不早于同一行 IntendedVsync。
func parseFrameTimestamps(output string) frameTimestampSet {
	result := frameTimestampSet{
		DisplayPresentTimes: make([]int64, 0),
		FrameCompletedTimes: make([]int64, 0),
	}

	inProfileData := false

	intendedVsyncIndex := -1
	displayPresentIndex := -1
	frameCompletedIndex := -1
	headerFieldCount := 0

	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)

		if line == "---PROFILEDATA---" {
			if !inProfileData {
				inProfileData = true
				continue
			}

			// 第二个 PROFILEDATA 表示当前 CSV 数据区结束。
			break
		}

		if !inProfileData || line == "" {
			continue
		}

		// 动态识别表头。
		if strings.Contains(line, "IntendedVsync") &&
			strings.Contains(line, "FrameCompleted") &&
			strings.Contains(line, ",") {
			headers := strings.Split(line, ",")

			for index, header := range headers {
				switch strings.TrimSpace(header) {
				case "IntendedVsync":
					intendedVsyncIndex = index
				case "DisplayPresentTime":
					displayPresentIndex = index
				case "FrameCompleted":
					frameCompletedIndex = index
				}
			}

			headerFieldCount = len(headers)

			result.FoundHeader = intendedVsyncIndex >= 0 &&
				frameCompletedIndex >= 0

			continue
		}

		if !result.FoundHeader {
			continue
		}

		fields := strings.Split(line, ",")

		// 数据字段数不一致时，说明不是有效 CSV 帧记录。
		if len(fields) != headerFieldCount {
			continue
		}

		if intendedVsyncIndex < 0 ||
			len(fields) <= intendedVsyncIndex {
			continue
		}

		intendedVsync, err := strconv.ParseInt(
			strings.TrimSpace(fields[intendedVsyncIndex]),
			10,
			64,
		)
		if err != nil ||
			intendedVsync <= 0 ||
			intendedVsync == maxInvalidTimestamp {
			continue
		}

		// 不根据 Flags 是否为 0 过滤。
		// vivo 等机型 Flags 可能始终是较大的数字。

		if displayPresentIndex >= 0 &&
			len(fields) > displayPresentIndex {
			displayPresentTime, err := strconv.ParseInt(
				strings.TrimSpace(fields[displayPresentIndex]),
				10,
				64,
			)

			// DisplayPresentTime 早于 IntendedVsync 视为异常数据。
			if err == nil &&
				displayPresentTime > 0 &&
				displayPresentTime != maxInvalidTimestamp &&
				displayPresentTime >= intendedVsync {
				result.DisplayPresentTimes = append(
					result.DisplayPresentTimes,
					displayPresentTime,
				)
			}
		}

		if frameCompletedIndex >= 0 &&
			len(fields) > frameCompletedIndex {
			frameCompletedTime, err := strconv.ParseInt(
				strings.TrimSpace(fields[frameCompletedIndex]),
				10,
				64,
			)

			// FrameCompleted 早于 IntendedVsync 视为异常数据。
			if err == nil &&
				frameCompletedTime > 0 &&
				frameCompletedTime != maxInvalidTimestamp &&
				frameCompletedTime >= intendedVsync {
				result.FrameCompletedTimes = append(
					result.FrameCompletedTimes,
					frameCompletedTime,
				)
			}
		}
	}

	result.DisplayPresentTimes = normalizeTimestamps(
		result.DisplayPresentTimes,
	)

	result.FrameCompletedTimes = normalizeTimestamps(
		result.FrameCompletedTimes,
	)

	return result
}

// normalizeTimestamps 只保留严格递增的有效时间戳。
func normalizeTimestamps(timestamps []int64) []int64 {
	result := make([]int64, 0, len(timestamps))

	for _, timestamp := range timestamps {
		if timestamp <= 0 || timestamp == maxInvalidTimestamp {
			continue
		}

		if len(result) > 0 &&
			timestamp <= result[len(result)-1] {
			continue
		}

		result = append(result, timestamp)
	}

	return result
}

// getNewTimestamps 返回指定游标之后的新时间戳。
func getNewTimestamps(timestamps []int64, cursor int64) []int64 {
	result := make([]int64, 0)

	for _, timestamp := range timestamps {
		if timestamp > cursor {
			result = append(result, timestamp)
		}
	}

	return result
}

// advanceCursor 将游标推进到本轮该时间源最新的时间戳。
func advanceCursor(cursor int64, timestamps []int64) int64 {
	if len(timestamps) == 0 {
		return cursor
	}

	latestTimestamp := timestamps[len(timestamps)-1]

	if latestTimestamp > cursor {
		return latestTimestamp
	}

	return cursor
}
