package scenario

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// Step describes one HTTP call in a scenario.
type Step struct {
	Name    string            `json:"name"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	BodyTpl string            `json:"body_tpl"`
	Headers map[string]string `json:"headers"`
	Extract map[string]string `json:"extract"`
}

// Scenario describes a sequence of HTTP steps and an optional formula.
type Scenario struct {
	Steps   []Step `json:"steps"`
	Formula string `json:"formula"`
}

// StepLog contains full request/response data for result storage and export.
type StepLog struct {
	Name     string `json:"name"`
	Request  string `json:"request"`
	Response string `json:"response"`
	CostMs   int64  `json:"cost_ms"`
	Success  bool   `json:"success"`
	ErrMsg   string `json:"err_msg,omitempty"`
}

// Result is the full scenario execution result.
type Result struct {
	Success       bool      `json:"success"`
	ErrMsg        string    `json:"err_msg,omitempty"`
	FormulaResult float64   `json:"formula_result"`
	Steps         []StepLog `json:"steps"`
	CostMs        int64     `json:"cost_ms"`
}

// HTTPDoer performs one HTTP request.
type HTTPDoer interface {
	Do(ctx context.Context, method, url string, body []byte, headers map[string]string) (statusCode int, respBody []byte, err error)
}

type httpClientDoer struct {
	client *http.Client
}

// NewHTTPClient returns a standard-library HTTPDoer for production use.
//
// MaxIdleConnsPerHost is raised well above the net/http default of 2 so that
// concurrent load-test traffic against the same host reuses connections
// instead of repeatedly paying TCP/TLS handshake cost. MaxConnsPerHost is
// left unlimited so this pool never caps how many requests can be in flight.
func NewHTTPClient(timeout time.Duration) HTTPDoer {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}
	client := &http.Client{Transport: transport}
	if timeout > 0 {
		client.Timeout = timeout
	}
	return &httpClientDoer{client: client}
}

func (d *httpClientDoer) Do(ctx context.Context, method, url string, body []byte, headers map[string]string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}

	return resp.StatusCode, respBody, nil
}

func renderTemplate(name string, tpl string, vars map[string]any) (string, error) {
	parsed, err := template.New(name).Option("missingkey=error").Parse(tpl)
	if err != nil {
		return "", err
	}

	var out bytes.Buffer
	if err := parsed.Execute(&out, vars); err != nil {
		return "", err
	}

	return out.String(), nil
}

func renderHeaders(headers map[string]string, vars map[string]any) (map[string]string, error) {
	rendered := make(map[string]string, len(headers))
	for key, value := range headers {
		out, err := renderTemplate("header."+key, value, vars)
		if err != nil {
			return nil, fmt.Errorf("render header %q: %w", key, err)
		}
		rendered[key] = out
	}
	return rendered, nil
}

func formulaToFloat64(value any) (float64, error) {
	switch v := value.(type) {
	case int:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		if v > math.MaxInt64 {
			return 0, fmt.Errorf("formula result %d overflows float64 precision", v)
		}
		return float64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	case string:
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, fmt.Errorf("formula result %q is not numeric", v)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("formula result has non-numeric type %T", value)
	}
}

func shouldEvaluateFormula(formula string) bool {
	return strings.TrimSpace(formula) != ""
}
