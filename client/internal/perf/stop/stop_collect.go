package stop

import (
	"context"
	"net/http"
	"strings"
	"time"

	"interface-load-test/client/common"
	"interface-load-test/client/internal/perf"
)

// StopCollectPerf 停止指定性能采集任务。
func StopCollectPerf(w http.ResponseWriter, r *http.Request) {
	// 从 URL 路径中获取任务 ID。
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	if taskID == "" {
		common.Fail(w, 10004, "task_id 不能为空")
		return
	}

	// 最多等待 5 秒，等待采集 goroutine 退出。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	task, err := perf.DefaultManager.Stop(ctx, taskID)
	if err != nil {
		common.Fail(w, 10008, "停止性能采集失败: "+err.Error())
		return
	}

	stopTime := time.Now().Format("2006-01-02 15:04:05")

	// 设备掉线导致的自动中断，Manager 在 finish() 里已经用真实断开时间
	// 保存过一次历史了。这里如果还按当前时间再存一遍，会把准确的断开
	// 时间覆盖成"用户点了停止按钮"的时间，跟实际发生的时间对不上。
	if task.Status == perf.TaskStatusInterrupted {
		common.Success(w, StopCollectResponse{
			TaskID:      task.TaskID,
			DeviceID:    task.DeviceID,
			PackageName: task.PackageName,
			ProcessName: task.ProcessName,
			Platform:    task.Platform,
			DeviceModel: task.DeviceModel,
			Status:      task.Status,
			StopTime:    stopTime,
		})
		return
	}

	if err := perf.SavePerfHistory(task, stopTime); err != nil {
		common.Fail(w, 10010, "保存历史采集失败: "+err.Error())
		return
	}

	common.Success(w, StopCollectResponse{
		TaskID:      task.TaskID,
		DeviceID:    task.DeviceID,
		PackageName: task.PackageName,
		ProcessName: task.ProcessName,
		Platform:    task.Platform,
		DeviceModel: task.DeviceModel,
		Status:      task.Status,
		StopTime:    stopTime,
	})
}
