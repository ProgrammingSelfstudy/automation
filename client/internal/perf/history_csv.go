package perf

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

var perfHistoryCSVHeader = []string{
	"collect_time",
	"sample_index",
	"app_cpu_percent",
	"total_cpu_percent",
	"memory_total_pss_mb",
	"memory_java_heap_mb",
	"memory_native_heap_mb",
	"memory_stack_mb",
	"memory_graphics_mb",
	"fps",
	"gpu_device_utilization_percent",
	"frames",
	"refresh_rate_hz",
	"small_jank",
	"jank",
	"big_jank",
	"total_small_jank",
	"total_jank",
	"total_big_jank",
}

// SavePerfHistoryCSV 把每秒采集样本保存成 CSV。
// 第一行是指标名，后面每一行对应一秒采集数据；没有的指标留空，方便 Excel/WPS 继续分析。
func SavePerfHistoryCSV(record PerfHistoryRecord) error {
	if err := os.MkdirAll(perfHistoryDir, 0755); err != nil {
		return fmt.Errorf("创建历史目录失败: %w", err)
	}

	filePath, err := perfHistoryCSVFilePath(record.TaskID)
	if err != nil {
		return err
	}

	tmpPath := filePath + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("创建 CSV 临时文件失败: %w", err)
	}

	// 写入 UTF-8 BOM，Excel 直接打开时中文不容易乱码。
	if _, err := file.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("写入 CSV BOM 失败: %w", err)
	}

	writer := csv.NewWriter(file)
	if err := writer.Write(perfHistoryCSVHeader); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("写入 CSV 表头失败: %w", err)
	}

	for index, sample := range record.Samples {
		row := sampleToCSVRow(index, sample)
		if err := writer.Write(row); err != nil {
			_ = file.Close()
			_ = os.Remove(tmpPath)
			return fmt.Errorf("写入 CSV 数据失败: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("刷新 CSV 数据失败: %w", err)
	}

	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("关闭 CSV 文件失败: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("保存 CSV 文件失败: %w", err)
	}

	return nil
}

func sampleToCSVRow(index int, sample MetricSample) []string {
	return []string{
		sample.CollectTime,
		strconv.Itoa(index + 1),
		sampleMetric(sample, "cpu", "app_cpu"),
		sampleMetric(sample, "cpu", "total_cpu"),
		sampleMetric(sample, "memory", "total_pss"),
		sampleMetric(sample, "memory", "java_heap"),
		sampleMetric(sample, "memory", "native_heap"),
		sampleMetric(sample, "memory", "stack"),
		sampleMetric(sample, "memory", "graphics"),
		sampleFPS(sample),
		sampleMetric(sample, "gpu", "device_utilization"),
		sampleMetric(sample, "fps", "frames"),
		sampleMetric(sample, "fps", "refresh_rate"),
		sampleMetric(sample, "jank", "small_jank"),
		sampleMetric(sample, "jank", "jank"),
		sampleMetric(sample, "jank", "big_jank"),
		sampleMetric(sample, "jank", "total_small_jank"),
		sampleMetric(sample, "jank", "total_jank"),
		sampleMetric(sample, "jank", "total_big_jank"),
	}
}

func sampleFPS(sample MetricSample) string {
	if value := sampleMetric(sample, "fps", "fps"); value != "" {
		return value
	}

	return sampleMetric(sample, "fps", "core_animation_frames_per_second")
}

func sampleMetric(sample MetricSample, group string, name string) string {
	data := sampleDataMap(sample)
	groupValue, ok := data[group].(map[string]interface{})
	if !ok {
		return ""
	}

	return csvValue(groupValue[name])
}

func sampleDataMap(sample MetricSample) map[string]interface{} {
	if sample.Data == nil {
		return map[string]interface{}{}
	}

	if data, ok := sample.Data.(map[string]interface{}); ok {
		return data
	}

	raw, err := json.Marshal(sample.Data)
	if err != nil {
		return map[string]interface{}{}
	}

	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return map[string]interface{}{}
	}

	return data
}

func csvValue(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 64)
	case json.Number:
		return typed.String()
	default:
		raw, _ := json.Marshal(typed)
		return string(raw)
	}
}
