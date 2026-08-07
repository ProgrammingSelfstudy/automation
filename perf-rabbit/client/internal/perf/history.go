package perf

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"client/common"
)

// perfHistoryDir 历史采集文件目录，由 init() 初始化。
// 以可执行文件所在目录为基准，避免因启动目录不同（例如双击 macOS App 时 CWD 是 /）
// 导致历史文件散落在不可预期的路径。
var perfHistoryDir string

func init() {
	exe, err := os.Executable()
	if err != nil {
		// 获取可执行文件路径失败时，退回相对路径（主要发生在 go run 等开发场景）。
		perfHistoryDir = filepath.Join("data", "perf")
		return
	}
	perfHistoryDir = filepath.Join(filepath.Dir(exe), "data", "perf")
}

// PerfHistoryRecord 是一条完整的历史采集记录。
// 文件里保存完整 samples，详情页直接读取这个结构展示图表。
type PerfHistoryRecord struct {
	TaskID         string         `json:"task_id"`            // 任务ID
	DeviceID       string         `json:"device_id"`          // Android 序列号 / iOS UDID
	PackageName    string         `json:"package_name"`       // Android 包名 / iOS Bundle ID
	ProcessName    string         `json:"process_name"`       // Android 等同包名；iOS 是进程名
	Platform       string         `json:"platform"`           // android / ios
	DeviceModel    string         `json:"device_model"`       // iOS 机型
	Status         string         `json:"status"`             // stopped / interrupted
	StartTime      string         `json:"start_time"`         // 开始采集时间
	StopTime       string         `json:"stop_time"`          // 停止采集时间
	SampleInterval int64          `json:"sample_interval_ms"` // 采集间隔，单位毫秒
	SampleCount    int            `json:"sample_count"`       // 样本数量
	LastError      string         `json:"last_error"`         // 最近一次采集错误
	Samples        []MetricSample `json:"samples"`            // 完整采集数据
}

// PerfHistorySummary 是历史列表返回的摘要。
// 列表不返回 samples，避免历史多了以后列表接口太大。
type PerfHistorySummary struct {
	TaskID         string `json:"task_id"`            // 任务ID
	DeviceID       string `json:"device_id"`          // Android 序列号 / iOS UDID
	PackageName    string `json:"package_name"`       // Android 包名 / iOS Bundle ID
	ProcessName    string `json:"process_name"`       // Android 等同包名；iOS 是进程名
	Platform       string `json:"platform"`           // android / ios
	DeviceModel    string `json:"device_model"`       // iOS 机型
	Status         string `json:"status"`             // stopped / interrupted
	StartTime      string `json:"start_time"`         // 开始采集时间
	StopTime       string `json:"stop_time"`          // 停止采集时间
	SampleInterval int64  `json:"sample_interval_ms"` // 采集间隔，单位毫秒
	SampleCount    int    `json:"sample_count"`       // 样本数量
	LastError      string `json:"last_error"`         // 最近一次采集错误
}

// SavePerfHistory 把停止后的任务保存成 JSON 文件。
func SavePerfHistory(task Task, stopTime string) error {
	if strings.TrimSpace(task.TaskID) == "" {
		return fmt.Errorf("task_id 不能为空")
	}

	status := task.Status
	if status == "" {
		status = TaskStatusStopped
	}

	record := PerfHistoryRecord{
		TaskID:         task.TaskID,
		DeviceID:       task.DeviceID,
		PackageName:    task.PackageName,
		ProcessName:    task.ProcessName,
		Platform:       task.Platform,
		DeviceModel:    task.DeviceModel,
		Status:         status,
		StartTime:      task.StartTime,
		StopTime:       stopTime,
		SampleInterval: task.SampleInterval,
		SampleCount:    len(task.Samples),
		LastError:      task.LastError,
		Samples:        append([]MetricSample(nil), task.Samples...),
	}

	if err := os.MkdirAll(perfHistoryDir, 0755); err != nil {
		return fmt.Errorf("创建历史目录失败: %w", err)
	}

	filePath, err := perfHistoryFilePath(task.TaskID)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化历史数据失败: %w", err)
	}

	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("写入历史临时文件失败: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("保存历史文件失败: %w", err)
	}

	if err := SavePerfHistoryCSV(record); err != nil {
		return err
	}

	return nil
}

// GetPerfHistoryList 查询历史采集列表。
func GetPerfHistoryList(w http.ResponseWriter, r *http.Request) {
	records, err := readAllPerfHistory()
	if err != nil {
		common.Fail(w, 10011, "查询历史采集列表失败: "+err.Error())
		return
	}

	summaries := make([]PerfHistorySummary, 0, len(records))
	for _, record := range records {
		summaries = append(summaries, PerfHistorySummary{
			TaskID:         record.TaskID,
			DeviceID:       record.DeviceID,
			PackageName:    record.PackageName,
			ProcessName:    record.ProcessName,
			Platform:       record.Platform,
			DeviceModel:    record.DeviceModel,
			Status:         record.Status,
			StartTime:      record.StartTime,
			StopTime:       record.StopTime,
			SampleInterval: record.SampleInterval,
			SampleCount:    record.SampleCount,
			LastError:      record.LastError,
		})
	}

	common.Success(w, map[string]any{
		"total": len(summaries),
		"tasks": summaries,
	})
}

// GetPerfHistoryDetail 查询单条历史采集详情。
func GetPerfHistoryDetail(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimSpace(r.PathValue("taskId"))

	record, err := readPerfHistory(taskID)
	if err != nil {
		common.Fail(w, 10011, "查询历史采集详情失败: "+err.Error())
		return
	}

	common.Success(w, record)
}

// DownloadPerfHistoryCSV 下载单条历史采集对应的 CSV 表格。
func DownloadPerfHistoryCSV(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimSpace(r.PathValue("taskId"))

	csvPath, err := perfHistoryCSVFilePath(taskID)
	if err != nil {
		common.Fail(w, 10011, "下载历史 CSV 失败: "+err.Error())
		return
	}

	// 兼容老历史：如果之前只保存过 JSON，没有 CSV，下载时补生成一次。
	if _, err := os.Stat(csvPath); err != nil {
		if !os.IsNotExist(err) {
			common.Fail(w, 10011, "读取历史 CSV 失败: "+err.Error())
			return
		}

		record, err := readPerfHistory(taskID)
		if err != nil {
			common.Fail(w, 10011, "查询历史采集详情失败: "+err.Error())
			return
		}
		if err := SavePerfHistoryCSV(record); err != nil {
			common.Fail(w, 10011, "生成历史 CSV 失败: "+err.Error())
			return
		}
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, taskID))
	http.ServeFile(w, r, csvPath)
}

// DeletePerfHistory 删除单条历史采集记录。
func DeletePerfHistory(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimSpace(r.PathValue("taskId"))

	filePath, err := perfHistoryFilePath(taskID)
	if err != nil {
		common.Fail(w, 10012, "删除历史采集失败: "+err.Error())
		return
	}

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			common.Fail(w, 10012, "历史采集不存在，task_id="+taskID)
			return
		}

		common.Fail(w, 10012, "删除历史采集失败: "+err.Error())
		return
	}

	if csvPath, err := perfHistoryCSVFilePath(taskID); err == nil {
		if err := os.Remove(csvPath); err != nil && !os.IsNotExist(err) {
			common.Fail(w, 10012, "删除历史 CSV 失败: "+err.Error())
			return
		}
	}

	common.Success(w, map[string]any{
		"task_id": taskID,
		"deleted": true,
	})
}

func readAllPerfHistory() ([]PerfHistoryRecord, error) {
	entries, err := os.ReadDir(perfHistoryDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []PerfHistoryRecord{}, nil
		}

		return nil, err
	}

	records := make([]PerfHistoryRecord, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		taskID := strings.TrimSuffix(entry.Name(), ".json")
		record, err := readPerfHistory(taskID)
		if err != nil {
			// 单个历史文件异常时跳过，避免一个坏文件拖垮整个历史列表。
			continue
		}

		records = append(records, record)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].StartTime > records[j].StartTime
	})

	return records, nil
}

func readPerfHistory(taskID string) (PerfHistoryRecord, error) {
	filePath, err := perfHistoryFilePath(taskID)
	if err != nil {
		return PerfHistoryRecord{}, err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return PerfHistoryRecord{}, fmt.Errorf("历史采集不存在，task_id=%s", taskID)
		}

		return PerfHistoryRecord{}, err
	}

	var record PerfHistoryRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return PerfHistoryRecord{}, fmt.Errorf("解析历史采集文件失败: %w", err)
	}

	if record.SampleCount == 0 {
		record.SampleCount = len(record.Samples)
	}

	return record, nil
}

func perfHistoryFilePath(taskID string) (string, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return "", fmt.Errorf("task_id 不能为空")
	}

	for _, char := range taskID {
		if char >= 'a' && char <= 'z' ||
			char >= 'A' && char <= 'Z' ||
			char >= '0' && char <= '9' ||
			char == '_' ||
			char == '-' ||
			char == '.' {
			continue
		}

		return "", fmt.Errorf("task_id 包含非法字符")
	}

	return filepath.Join(perfHistoryDir, taskID+".json"), nil
}

func perfHistoryCSVFilePath(taskID string) (string, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return "", fmt.Errorf("task_id 不能为空")
	}

	if strings.Contains(taskID, "/") || strings.Contains(taskID, "\\") || strings.Contains(taskID, "..") {
		return "", fmt.Errorf("task_id 非法")
	}

	return filepath.Join(perfHistoryDir, taskID+".csv"), nil
}
