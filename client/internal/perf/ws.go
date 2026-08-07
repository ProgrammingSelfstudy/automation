package perf

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

const wsWriteTimeout = 5 * time.Second

// CollectPerfWSMessage 是推送给前端的增量消息，字段跟轮询接口
// GetCollectPerfResponse 保持一致，前端解析逻辑可以直接复用。
type CollectPerfWSMessage struct {
	TaskID       string         `json:"task_id"`
	Status       string         `json:"status"`
	LastError    string         `json:"last_error"`
	TotalSamples int            `json:"total_samples"`
	NextFrom     int            `json:"next_from"`
	Samples      []MetricSample `json:"samples"`
}

// ServeCollectWS 用 WebSocket 推送性能采集任务的增量数据，替代前端原来
// 按固定间隔轮询 GetCollectPerf 的方式：有新样本或任务状态变化时才推送，
// 不是定时器空转。
func ServeCollectWS(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	if taskID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if _, _, err := DefaultManager.GetTask(taskID, 0); err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// 本地 Agent 只监听 127.0.0.1，中心平台前端可能跑在开发端口或生产
		// 域名，跟 CORSMiddleware 的取舍一致：允许任意来源接入。
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()

	notify, unsubscribe := DefaultHub.Subscribe(taskID)
	defer unsubscribe()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go discardClientMessages(ctx, cancel, conn)

	fromSample := 0
	// 连接建立时先把已经采集到的数据一次性推过去——开始采集接口本身就会
	// 同步做一次采集，WS 连上时这条数据往往已经在了；之后只推增量。
	if !pushCollectUpdate(ctx, conn, taskID, &fromSample) {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-notify:
			if !ok {
				return
			}
			if !pushCollectUpdate(ctx, conn, taskID, &fromSample) {
				return
			}
		}
	}
}

// pushCollectUpdate 推一次增量更新，返回 false 表示应该结束这个连接
// （写失败，或者任务已经不是 collecting 状态、后面不会再有更新了）。
func pushCollectUpdate(ctx context.Context, conn *websocket.Conn, taskID string, fromSample *int) bool {
	task, totalSamples, err := DefaultManager.GetTask(taskID, *fromSample)
	if err != nil {
		return false
	}

	message := CollectPerfWSMessage{
		TaskID:       task.TaskID,
		Status:       task.Status,
		LastError:    task.LastError,
		TotalSamples: totalSamples,
		NextFrom:     totalSamples,
		Samples:      task.Samples,
	}
	*fromSample = totalSamples

	data, err := json.Marshal(message)
	if err != nil {
		return false
	}

	writeCtx, writeCancel := context.WithTimeout(ctx, wsWriteTimeout)
	writeErr := conn.Write(writeCtx, websocket.MessageText, data)
	writeCancel()
	if writeErr != nil {
		return false
	}

	return task.Status == TaskStatusCollecting
}

func discardClientMessages(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn) {
	defer cancel()

	for {
		_, reader, err := conn.Reader(ctx)
		if err != nil {
			return
		}
		if _, err := io.Copy(io.Discard, reader); err != nil {
			return
		}
	}
}
