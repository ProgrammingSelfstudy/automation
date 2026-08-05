package export

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"interface-load-test/internal/resultstore"
)

func TestBuildWorkbookWithMultipleAccounts(t *testing.T) {
	createdAt := time.Date(2026, 8, 4, 14, 0, 0, 0, time.UTC)
	file, err := BuildWorkbook("task-1", []resultstore.AccountResults{
		{
			AccountID:   "acc-1",
			AccountName: "alice",
			Rows: []resultstore.ResultRow{
				{SeqNo: 1, CostMs: 100, FormulaResult: 10.5, Success: true, ErrMsg: "", CreatedAt: createdAt},
				{SeqNo: 2, CostMs: 120, FormulaResult: 11.5, Success: false, ErrMsg: "boom", CreatedAt: createdAt.Add(time.Second)},
			},
		},
		{
			AccountID:   "acc-2",
			AccountName: "bob",
			Rows: []resultstore.ResultRow{
				{SeqNo: 1, CostMs: 200, FormulaResult: 20.5, Success: true, CreatedAt: createdAt.Add(2 * time.Second)},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildWorkbook() error = %v", err)
	}
	defer file.Close()

	workbook := reopenWorkbook(t, file)
	defer workbook.Close()

	if got, want := workbook.GetSheetList(), []string{"alice", "bob"}; !equalStrings(got, want) {
		t.Fatalf("sheet list = %v, want %v", got, want)
	}
	assertHeaders(t, workbook, "alice")
	assertHeaders(t, workbook, "bob")
	assertRow(t, workbook, "alice", 2, []string{"1", "100", "10.5", "TRUE", "", createdAt.Format(time.RFC3339)})
	assertRow(t, workbook, "alice", 3, []string{"2", "120", "11.5", "FALSE", "boom", createdAt.Add(time.Second).Format(time.RFC3339)})
	assertRow(t, workbook, "bob", 2, []string{"1", "200", "20.5", "TRUE", "", createdAt.Add(2 * time.Second).Format(time.RFC3339)})
}

func TestBuildWorkbookWithSingleAccountReusesDefaultSheet(t *testing.T) {
	file, err := BuildWorkbook("task-1", []resultstore.AccountResults{
		{
			AccountID:   "acc-1",
			AccountName: "only-account",
			Rows: []resultstore.ResultRow{
				{SeqNo: 1, CreatedAt: time.Date(2026, 8, 4, 15, 0, 0, 0, time.UTC)},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildWorkbook() error = %v", err)
	}
	defer file.Close()

	workbook := reopenWorkbook(t, file)
	defer workbook.Close()

	sheets := workbook.GetSheetList()
	if got, want := len(sheets), 1; got != want {
		t.Fatalf("sheet count = %d, want %d; sheets=%v", got, want, sheets)
	}
	if got, want := sheets[0], "only-account"; got != want {
		t.Fatalf("sheet name = %q, want %q", got, want)
	}
}

func TestBuildWorkbookSanitizesSheetNames(t *testing.T) {
	longName := "account/name:with*bad?[chars]-and-a-very-long-suffix"
	file, err := BuildWorkbook("task-1", []resultstore.AccountResults{
		{AccountID: "acc-1", AccountName: longName},
		{AccountID: "acc-2", AccountName: "   "},
	})
	if err != nil {
		t.Fatalf("BuildWorkbook() error = %v", err)
	}
	defer file.Close()

	workbook := reopenWorkbook(t, file)
	defer workbook.Close()

	sheets := workbook.GetSheetList()
	if got, want := len(sheets), 2; got != want {
		t.Fatalf("sheet count = %d, want %d; sheets=%v", got, want, sheets)
	}
	if !isLegalSheetName(sheets[0]) {
		t.Fatalf("sanitized sheet name %q is not legal", sheets[0])
	}
	if strings.ContainsAny(sheets[0], `:\/?*[]`) {
		t.Fatalf("sanitized sheet name %q still contains invalid characters", sheets[0])
	}
	if !strings.HasPrefix(sheets[0], "account_name_with_bad__chars_") {
		t.Fatalf("sanitized sheet name %q is not readable enough", sheets[0])
	}
	if got, want := sheets[1], "Account"; got != want {
		t.Fatalf("blank account name sheet = %q, want %q", got, want)
	}
}

func TestBuildWorkbookDeduplicatesSanitizedSheetNames(t *testing.T) {
	file, err := BuildWorkbook("task-1", []resultstore.AccountResults{
		{AccountID: "acc-1", AccountName: "a/b"},
		{AccountID: "acc-2", AccountName: "a:b"},
		{AccountID: "acc-3", AccountName: "A_B"},
	})
	if err != nil {
		t.Fatalf("BuildWorkbook() error = %v", err)
	}
	defer file.Close()

	workbook := reopenWorkbook(t, file)
	defer workbook.Close()

	if got, want := workbook.GetSheetList(), []string{"a_b", "a_b-2", "A_B-3"}; !equalStrings(got, want) {
		t.Fatalf("sheet list = %v, want %v", got, want)
	}
}

func TestBuildWorkbookKeepsRowOrder(t *testing.T) {
	createdAt := time.Date(2026, 8, 4, 16, 0, 0, 0, time.UTC)
	file, err := BuildWorkbook("task-1", []resultstore.AccountResults{
		{
			AccountID:   "acc-1",
			AccountName: "alice",
			Rows: []resultstore.ResultRow{
				{SeqNo: 2, CostMs: 200, CreatedAt: createdAt},
				{SeqNo: 1, CostMs: 100, CreatedAt: createdAt.Add(time.Second)},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildWorkbook() error = %v", err)
	}
	defer file.Close()

	workbook := reopenWorkbook(t, file)
	defer workbook.Close()

	assertRow(t, workbook, "alice", 2, []string{"2", "200", "0", "FALSE", "", createdAt.Format(time.RFC3339)})
	assertRow(t, workbook, "alice", 3, []string{"1", "100", "0", "FALSE", "", createdAt.Add(time.Second).Format(time.RFC3339)})
}

func TestBuildWorkbookWithNoResults(t *testing.T) {
	file, err := BuildWorkbook("task-empty", nil)
	if !errors.Is(err, ErrNoResults) {
		t.Fatalf("BuildWorkbook() error = %v, want %v", err, ErrNoResults)
	}
	if file != nil {
		file.Close()
		t.Fatal("BuildWorkbook() file is non-nil, want nil")
	}
}

func reopenWorkbook(t *testing.T, file *excelize.File) *excelize.File {
	t.Helper()

	buffer, err := file.WriteToBuffer()
	if err != nil {
		t.Fatalf("WriteToBuffer() error = %v", err)
	}
	workbook, err := excelize.OpenReader(bytes.NewReader(buffer.Bytes()))
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	return workbook
}

func assertHeaders(t *testing.T, workbook *excelize.File, sheet string) {
	t.Helper()

	for i, header := range headers {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			t.Fatalf("CoordinatesToCellName() error = %v", err)
		}
		if got, want := cellValue(t, workbook, sheet, cell), header; got != want {
			t.Fatalf("%s!%s = %q, want %q", sheet, cell, got, want)
		}
	}
}

func assertRow(t *testing.T, workbook *excelize.File, sheet string, row int, values []string) {
	t.Helper()

	for i, want := range values {
		cell, err := excelize.CoordinatesToCellName(i+1, row)
		if err != nil {
			t.Fatalf("CoordinatesToCellName() error = %v", err)
		}
		if got := cellValue(t, workbook, sheet, cell); got != want {
			t.Fatalf("%s!%s = %q, want %q", sheet, cell, got, want)
		}
	}
}

func cellValue(t *testing.T, workbook *excelize.File, sheet string, cell string) string {
	t.Helper()

	value, err := workbook.GetCellValue(sheet, cell)
	if err != nil {
		t.Fatalf("GetCellValue(%s, %s) error = %v", sheet, cell, err)
	}
	return value
}

func isLegalSheetName(name string) bool {
	return name != "" && len([]rune(name)) <= maxSheetNameLen && !strings.ContainsAny(name, `:\/?*[]`)
}

func equalStrings(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
