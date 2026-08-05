package scenario

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/expr-lang/expr"
	"github.com/tidwall/gjson"

	"interface-load-test/internal/accountpool"
	"interface-load-test/internal/logevent"
)

// Executor executes Scenarios. It keeps no per-run mutable state and is safe to
// share across concurrent workers when its HTTPDoer is safe.
type Executor struct {
	http HTTPDoer
	hub  *logevent.Hub
}

// NewExecutor creates a Scenario executor.
func NewExecutor(httpDoer HTTPDoer, hub *logevent.Hub) *Executor {
	if httpDoer == nil {
		httpDoer = NewHTTPClient(0)
	}
	return &Executor{
		http: httpDoer,
		hub:  hub,
	}
}

// Execute runs all Scenario steps in order.
func (e *Executor) Execute(ctx context.Context, taskID string, seqNo int, acc *accountpool.Account, sc Scenario) (result Result) {
	if ctx == nil {
		ctx = context.Background()
	}

	startedAt := time.Now()
	defer func() {
		result.CostMs = time.Since(startedAt).Milliseconds()
	}()

	vars := map[string]any{
		"account": acc,
	}

	for _, step := range sc.Steps {
		if err := ctx.Err(); err != nil {
			result.ErrMsg = err.Error()
			return result
		}

		stepLog, respBody, ok := e.executeStep(ctx, taskID, seqNo, acc, step, vars)
		result.Steps = append(result.Steps, stepLog)
		if !ok {
			result.ErrMsg = stepLog.ErrMsg
			return result
		}

		for key, path := range step.Extract {
			vars[key] = gjson.GetBytes(respBody, path).Value()
		}
	}

	if shouldEvaluateFormula(sc.Formula) {
		value, err := expr.Eval(sc.Formula, vars)
		if err != nil {
			result.ErrMsg = err.Error()
			return result
		}

		formulaResult, err := formulaToFloat64(value)
		if err != nil {
			result.ErrMsg = err.Error()
			return result
		}
		result.FormulaResult = formulaResult
	}

	result.Success = true
	return result
}

func (e *Executor) executeStep(ctx context.Context, taskID string, seqNo int, acc *accountpool.Account, step Step, vars map[string]any) (StepLog, []byte, bool) {
	startedAt := time.Now()
	stepLog := StepLog{Name: step.Name}

	finish := func(success bool, errMsg string, respBody []byte) (StepLog, []byte, bool) {
		stepLog.Response = string(respBody)
		stepLog.Success = success
		stepLog.ErrMsg = errMsg
		stepLog.CostMs = time.Since(startedAt).Milliseconds()
		e.publishStepEvent(taskID, seqNo, acc, stepLog)
		return stepLog, respBody, success
	}

	url, err := renderTemplate("step."+step.Name+".url", step.URL, vars)
	if err != nil {
		return finish(false, fmt.Sprintf("render url: %v", err), nil)
	}

	body, err := renderTemplate("step."+step.Name+".body", step.BodyTpl, vars)
	if err != nil {
		return finish(false, fmt.Sprintf("render body: %v", err), nil)
	}
	stepLog.Request = body

	headers, err := renderHeaders(step.Headers, vars)
	if err != nil {
		return finish(false, err.Error(), nil)
	}

	if err := ctx.Err(); err != nil {
		return finish(false, err.Error(), nil)
	}

	statusCode, respBody, err := e.http.Do(ctx, step.Method, url, []byte(body), headers)
	if err != nil {
		return finish(false, err.Error(), respBody)
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return finish(false, fmt.Sprintf("http status %d", statusCode), respBody)
	}

	return finish(true, "", respBody)
}

func (e *Executor) publishStepEvent(taskID string, seqNo int, acc *accountpool.Account, stepLog StepLog) {
	if e.hub == nil {
		return
	}

	accountID := ""
	if acc != nil {
		accountID = acc.ID
	}

	e.hub.Publish(logevent.Event{
		TaskID:    taskID,
		AccountID: accountID,
		SeqNo:     seqNo,
		StepName:  stepLog.Name,
		Success:   stepLog.Success,
		ErrMsg:    stepLog.ErrMsg,
		CostMs:    stepLog.CostMs,
		Timestamp: time.Now(),
	})
}
