package perfstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	// id 是 BIGINT AUTO_INCREMENT，不在 INSERT 列表里——数据库分配后
	// 用 LastInsertId() 读回来，这样任务 ID 是"123"这样短的数字，方便
	// 压测时口头对答案、贴日志，不用比对一串 UUID。
	insertPerfTaskSQL = `INSERT INTO perf_task
		(user_id, device_id, package_name, process_name, platform, device_model,
		 status, start_time, stop_time, sample_interval_ms, sample_count, last_error, samples)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	getPerfTaskSQL = `SELECT id, user_id, device_id, package_name, process_name, platform, device_model,
		status, start_time, stop_time, sample_interval_ms, sample_count, last_error, samples, created_at
		FROM perf_task WHERE id = ?`

	// 列表接口不选 samples——一次采集可能有几十上百 KB 的时间序列数据，
	// 列表页只需要摘要信息，带上 samples 会让列表接口越用越慢。
	listPerfTaskSQL = `SELECT id, user_id, device_id, package_name, process_name, platform, device_model,
		status, start_time, stop_time, sample_interval_ms, sample_count, last_error, created_at
		FROM perf_task ORDER BY start_time DESC`

	deletePerfTaskSQL = `DELETE FROM perf_task WHERE id = ?`
)

var (
	ErrNotFound         = errors.New("perf task not found")
	ErrUserIDRequired   = errors.New("user id is required")
	ErrDeviceIDRequired = errors.New("device id is required")
)

// PerfTaskSummary 是历史列表返回的摘要，不含完整 samples。
type PerfTaskSummary struct {
	ID               int64
	UserID           string
	DeviceID         string
	PackageName      string
	ProcessName      string
	Platform         string
	DeviceModel      string
	Status           string
	StartTime        time.Time
	StopTime         time.Time
	SampleIntervalMS int64
	SampleCount      int
	LastError        string
	CreatedAt        time.Time
}

// PerfTask 是一条完整的性能采集历史记录，包含完整时间序列数据。
//
// Samples 用 json.RawMessage 存，不在这一层强绑定具体的样本结构——
// 采集端（本地 Agent）用的是它自己的 MetricSample 类型，这一层只负责
// 原样存取，避免这个包依赖 perf-rabbit 那个独立 Go module 里的类型
// （目前两边是各自独立的 go.mod，见 docs/architecture-perf-rabbit-merge.md）。
type PerfTask struct {
	PerfTaskSummary
	Samples json.RawMessage
}

// Store 持久化性能采集历史记录。
type Store interface {
	Create(ctx context.Context, task *PerfTask) error
	Get(ctx context.Context, id int64) (*PerfTask, error)
	// List 返回全部记录，不按用户过滤——产品决定性能测试的历史数据所有
	// 登录用户都能互相看到，不用按团队/项目隔离（架构文档 Phase 4 曾经
	// 标注为待定，现在已确认不需要）。user_id 仍然存着，用来标记是谁上报的。
	List(ctx context.Context) ([]PerfTaskSummary, error)
	Delete(ctx context.Context, id int64) error
}

// SQLStore 是基于 database/sql 的 Store 实现。
type SQLStore struct {
	db *sql.DB
}

// NewSQLStore 用一个已经打开的数据库连接构建 Store。
func NewSQLStore(db *sql.DB) *SQLStore {
	return &SQLStore{db: db}
}

// NewMySQLStore 打开一个 MySQL 连接并 ping 一次确认可用。
func NewMySQLStore(dsn string) (*SQLStore, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return NewSQLStore(db), nil
}

// Create 校验并存一条性能采集历史记录。
func (s *SQLStore) Create(ctx context.Context, task *PerfTask) error {
	if strings.TrimSpace(task.UserID) == "" {
		return ErrUserIDRequired
	}
	if strings.TrimSpace(task.DeviceID) == "" {
		return ErrDeviceIDRequired
	}
	if task.Samples == nil {
		task.Samples = json.RawMessage("[]")
	}

	result, err := s.db.ExecContext(
		ctx,
		insertPerfTaskSQL,
		task.UserID,
		task.DeviceID,
		task.PackageName,
		task.ProcessName,
		task.Platform,
		nullableString(task.DeviceModel),
		task.Status,
		task.StartTime,
		task.StopTime,
		task.SampleIntervalMS,
		task.SampleCount,
		nullableString(task.LastError),
		string(task.Samples),
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	task.ID = id
	return nil
}

// Get 按 ID 查询一条完整记录，包含 samples。
func (s *SQLStore) Get(ctx context.Context, id int64) (*PerfTask, error) {
	row := s.db.QueryRowContext(ctx, getPerfTaskSQL, id)

	var task PerfTask
	var deviceModel, lastError sql.NullString
	var samples []byte

	err := row.Scan(
		&task.ID,
		&task.UserID,
		&task.DeviceID,
		&task.PackageName,
		&task.ProcessName,
		&task.Platform,
		&deviceModel,
		&task.Status,
		&task.StartTime,
		&task.StopTime,
		&task.SampleIntervalMS,
		&task.SampleCount,
		&lastError,
		&samples,
		&task.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	task.DeviceModel = deviceModel.String
	task.LastError = lastError.String
	task.Samples = append(json.RawMessage(nil), samples...)

	return &task, nil
}

// List 返回历史列表摘要，按开始时间倒序，不含 samples。
func (s *SQLStore) List(ctx context.Context) ([]PerfTaskSummary, error) {
	rows, err := s.db.QueryContext(ctx, listPerfTaskSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summaries := make([]PerfTaskSummary, 0)
	for rows.Next() {
		var summary PerfTaskSummary
		var deviceModel, lastError sql.NullString

		if err := rows.Scan(
			&summary.ID,
			&summary.UserID,
			&summary.DeviceID,
			&summary.PackageName,
			&summary.ProcessName,
			&summary.Platform,
			&deviceModel,
			&summary.Status,
			&summary.StartTime,
			&summary.StopTime,
			&summary.SampleIntervalMS,
			&summary.SampleCount,
			&lastError,
			&summary.CreatedAt,
		); err != nil {
			return nil, err
		}

		summary.DeviceModel = deviceModel.String
		summary.LastError = lastError.String
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return summaries, nil
}

// Delete 删除一条历史记录。
func (s *SQLStore) Delete(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, deletePerfTaskSQL, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
