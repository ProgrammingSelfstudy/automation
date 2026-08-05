package export

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"interface-load-test/internal/resultstore"
)

const (
	defaultSheetName = "Sheet1"
	maxSheetNameLen  = 31
)

var (
	// ErrNoResults is returned when there is no task data to export.
	ErrNoResults = errors.New("no results to export")
)

var headers = []string{"序号", "耗时(ms)", "公式结果", "是否成功", "错误信息", "执行时间"}

// ResultStore is the minimal store dependency used by ExportTask.
type ResultStore interface {
	ListByTaskGroupedByAccount(ctx context.Context, taskID string) ([]resultstore.AccountResults, error)
}

// BuildWorkbook builds an Excel workbook from already-grouped task results.
func BuildWorkbook(taskID string, groups []resultstore.AccountResults) (*excelize.File, error) {
	if len(groups) == 0 {
		return nil, ErrNoResults
	}

	file := excelize.NewFile()
	usedSheetNames := make(map[string]struct{}, len(groups))

	for i, group := range groups {
		sheetName := uniqueSheetName(group.AccountName, usedSheetNames)
		var err error
		if i == 0 {
			err = file.SetSheetName(defaultSheetName, sheetName)
		} else {
			_, err = file.NewSheet(sheetName)
		}
		if err != nil {
			file.Close()
			return nil, err
		}

		if err := writeSheet(file, sheetName, group.Rows); err != nil {
			file.Close()
			return nil, err
		}
	}

	file.SetActiveSheet(0)
	return file, nil
}

// ExportTask queries task results and builds an Excel workbook.
func ExportTask(ctx context.Context, store ResultStore, taskID string) (*excelize.File, error) {
	groups, err := store.ListByTaskGroupedByAccount(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return BuildWorkbook(taskID, groups)
}

func writeSheet(file *excelize.File, sheetName string, rows []resultstore.ResultRow) error {
	for i, header := range headers {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return err
		}
		if err := file.SetCellValue(sheetName, cell, header); err != nil {
			return err
		}
	}

	for rowIndex, row := range rows {
		values := []any{
			row.SeqNo,
			row.CostMs,
			row.FormulaResult,
			row.Success,
			row.ErrMsg,
			row.CreatedAt.Format(time.RFC3339),
		}
		for columnIndex, value := range values {
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+2)
			if err != nil {
				return err
			}
			if err := file.SetCellValue(sheetName, cell, value); err != nil {
				return err
			}
		}
	}

	return nil
}

func uniqueSheetName(rawName string, used map[string]struct{}) string {
	base := sanitizeSheetName(rawName)
	name := base
	for i := 2; ; i++ {
		key := strings.ToLower(name)
		if _, ok := used[key]; !ok {
			used[key] = struct{}{}
			return name
		}

		suffix := fmt.Sprintf("-%d", i)
		name = truncateRunes(base, maxSheetNameLen-len([]rune(suffix))) + suffix
	}
}

func sanitizeSheetName(name string) string {
	name = strings.TrimSpace(name)
	var out strings.Builder
	for _, r := range name {
		switch r {
		case ':', '\\', '/', '?', '*', '[', ']':
			out.WriteRune('_')
		default:
			out.WriteRune(r)
		}
	}

	sanitized := strings.TrimSpace(out.String())
	if sanitized == "" {
		sanitized = "Account"
	}
	return truncateRunes(sanitized, maxSheetNameLen)
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
