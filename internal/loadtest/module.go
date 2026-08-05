package loadtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"interface-load-test/internal/accountpool"
	"interface-load-test/internal/logevent"
	"interface-load-test/internal/scenario"
	"interface-load-test/internal/task"
	"interface-load-test/internal/workerpool"
)

const Type = "load_test"

var (
	ErrInvalidConfig         = errors.New("invalid load test config")
	ErrScenarioStepsRequired = errors.New("scenario must contain at least one step")
	ErrPerAccountCount       = errors.New("per account count must be positive")
)

// Config is the load-test module configuration.
type Config struct {
	Scenario        scenario.Scenario `json:"scenario"`
	PerAccountCount int               `json:"per_account_count"`
}

// Module wires scenario execution into the task module interface.
type Module struct {
	hub      *logevent.Hub
	sink     workerpool.ResultSink
	httpDoer scenario.HTTPDoer
}

// NewModule creates a load-test module.
func NewModule(hub *logevent.Hub, sink workerpool.ResultSink) *Module {
	return &Module{
		hub:      hub,
		sink:     sink,
		httpDoer: scenario.NewHTTPClient(0),
	}
}

// Type returns the module type.
func (m *Module) Type() string {
	return Type
}

// ValidateConfig validates load-test JSON config.
func (m *Module) ValidateConfig(cfg json.RawMessage) error {
	parsed, err := parseConfig(cfg)
	if err != nil {
		return err
	}
	return validateConfig(parsed)
}

// ExecutionCount returns the per-account row count from config.
func (m *Module) ExecutionCount(cfg json.RawMessage) (int, error) {
	parsed, err := parseConfig(cfg)
	if err != nil {
		return 0, err
	}
	return parsed.PerAccountCount, nil
}

// Run executes the load-test task.
func (m *Module) Run(ctx context.Context, t *task.Task, accounts []*accountpool.Account) (task.RunResult, error) {
	cfg, err := parseConfig(t.Config)
	if err != nil {
		return task.RunResult{}, err
	}
	if err := validateConfig(cfg); err != nil {
		return task.RunResult{}, err
	}

	httpDoer := m.httpDoer
	if httpDoer == nil {
		httpDoer = scenario.NewHTTPClient(0)
	}

	accountPool := accountpool.NewAccountPool(accounts)
	executor := scenario.NewExecutor(httpDoer, m.hub)
	pool := workerpool.NewPool(accountPool, executor, m.sink)
	summary := pool.Run(ctx, t.ID, cfg.Scenario, cfg.PerAccountCount, t.Concurrency)

	return task.RunResult{
		SuccessCount: summary.SuccessCount,
		FailCount:    summary.FailCount,
	}, nil
}

func parseConfig(cfg json.RawMessage) (Config, error) {
	var parsed Config
	if err := json.Unmarshal(cfg, &parsed); err != nil {
		return Config{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	return parsed, nil
}

func validateConfig(cfg Config) error {
	if len(cfg.Scenario.Steps) == 0 {
		return ErrScenarioStepsRequired
	}
	if cfg.PerAccountCount <= 0 {
		return ErrPerAccountCount
	}
	return nil
}
