package resultstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
	"unicode/utf8"

	_ "github.com/go-sql-driver/mysql"

	"interface-load-test/internal/accountpool"
	"interface-load-test/internal/scenario"
)

const (
	maxAccountNameRunes = 128
	maxErrMsgBytes      = 65535

	insertTaskResultSQL = `INSERT INTO task_result (task_id, account_id, account_name, seq_no, steps, formula_result, success, err_msg, cost_ms) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	listByTaskSQL       = `SELECT id, task_id, account_id, account_name, seq_no, steps, formula_result, success, err_msg, cost_ms, created_at FROM task_result WHERE task_id = ? ORDER BY account_id ASC, seq_no ASC`
)

// Store persists task results and reads them for export.
type Store struct {
	db *sql.DB
}

// New builds a Store from an already-open database handle.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// NewMySQLStore opens a MySQL connection and pings it once.
func NewMySQLStore(dsn string) (*Store, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return New(db), nil
}

// Save implements workerpool.ResultSink.
func (s *Store) Save(ctx context.Context, taskID string, acc *accountpool.Account, seqNo int, result scenario.Result) error {
	stepsToStore := result.Steps
	if stepsToStore == nil {
		stepsToStore = []scenario.StepLog{}
	}
	steps, err := json.Marshal(stepsToStore)
	if err != nil {
		return err
	}

	accountID := ""
	accountName := ""
	if acc != nil {
		accountID = acc.ID
		accountName = acc.Username
	}

	success := 0
	if result.Success {
		success = 1
	}

	_, err = s.db.ExecContext(
		ctx,
		insertTaskResultSQL,
		taskID,
		accountID,
		truncateRunes(accountName, maxAccountNameRunes),
		seqNo,
		string(steps),
		result.FormulaResult,
		success,
		truncateUTF8Bytes(result.ErrMsg, maxErrMsgBytes),
		result.CostMs,
	)
	return err
}

// ResultRow is one persisted task_result row.
type ResultRow struct {
	ID            int64
	TaskID        string
	AccountID     string
	AccountName   string
	SeqNo         int
	Steps         json.RawMessage
	FormulaResult float64
	Success       bool
	ErrMsg        string
	CostMs        int64
	CreatedAt     time.Time
}

// AccountResults groups result rows for one account.
type AccountResults struct {
	AccountID   string
	AccountName string
	Rows        []ResultRow
}

// ListByTaskGroupedByAccount returns deterministic account groups and seq order.
func (s *Store) ListByTaskGroupedByAccount(ctx context.Context, taskID string) ([]AccountResults, error) {
	rows, err := s.db.QueryContext(ctx, listByTaskSQL, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make([]AccountResults, 0)
	groupIndex := make(map[string]int)

	for rows.Next() {
		row, err := scanResultRow(rows)
		if err != nil {
			return nil, err
		}

		index, ok := groupIndex[row.AccountID]
		if !ok {
			index = len(groups)
			groupIndex[row.AccountID] = index
			groups = append(groups, AccountResults{
				AccountID:   row.AccountID,
				AccountName: row.AccountName,
			})
		}
		groups[index].Rows = append(groups[index].Rows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return groups, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanResultRow(scanner rowScanner) (ResultRow, error) {
	var row ResultRow
	var steps []byte
	var formulaResult sql.NullFloat64
	var errMsg sql.NullString
	var costMs sql.NullInt64
	var success int

	err := scanner.Scan(
		&row.ID,
		&row.TaskID,
		&row.AccountID,
		&row.AccountName,
		&row.SeqNo,
		&steps,
		&formulaResult,
		&success,
		&errMsg,
		&costMs,
		&row.CreatedAt,
	)
	if err != nil {
		return ResultRow{}, err
	}

	row.Steps = append(json.RawMessage(nil), steps...)
	if formulaResult.Valid {
		row.FormulaResult = formulaResult.Float64
	}
	row.Success = success != 0
	if errMsg.Valid {
		row.ErrMsg = errMsg.String
	}
	if costMs.Valid {
		row.CostMs = costMs.Int64
	}

	return row, nil
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}

func truncateUTF8Bytes(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}

	end := 0
	for i := range s {
		if i > maxBytes {
			break
		}
		end = i
	}
	if end == 0 && !utf8.ValidString(s[:maxBytes]) {
		return ""
	}
	return s[:end]
}
