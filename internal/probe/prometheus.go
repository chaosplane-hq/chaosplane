package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
)

type prometheusProbe struct {
	spec   v1alpha1.PrometheusProbe
	client *http.Client
}

type promQueryResponse struct {
	Status string        `json:"status"`
	Data   promQueryData `json:"data"`
}

type promQueryData struct {
	ResultType string            `json:"resultType"`
	Result     []promQueryResult `json:"result"`
}

type promQueryResult struct {
	Value []json.RawMessage `json:"value"`
}

func (p *prometheusProbe) Run(ctx context.Context) (bool, error) {
	if p.client == nil {
		p.client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.spec.URL+"/api/v1/query", nil)
	if err != nil {
		return false, fmt.Errorf("building request: %w", err)
	}
	q := req.URL.Query()
	q.Set("query", p.spec.Query)
	req.URL.RawQuery = q.Encode()

	resp, err := p.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("executing query: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("prometheus returned status %d: %s", resp.StatusCode, string(body))
	}

	var promResp promQueryResponse
	if err := json.Unmarshal(body, &promResp); err != nil {
		return false, fmt.Errorf("parsing response: %w", err)
	}

	if promResp.Status != "success" {
		return false, fmt.Errorf("query status: %s", promResp.Status)
	}

	if len(promResp.Data.Result) == 0 {
		return false, nil
	}

	values := promResp.Data.Result[0].Value
	if len(values) < 2 {
		return false, fmt.Errorf("unexpected value format")
	}

	var valStr string
	if err := json.Unmarshal(values[1], &valStr); err != nil {
		return false, fmt.Errorf("parsing value: %w", err)
	}

	var val float64
	if _, err := fmt.Sscanf(valStr, "%f", &val); err != nil {
		return false, fmt.Errorf("converting value %q: %w", valStr, err)
	}

	return evaluateCondition(val, p.spec.Condition), nil
}

func evaluateCondition(val float64, cond v1alpha1.ProbeCondition) bool {
	switch cond.Operator {
	case ">":
		return val > cond.Threshold
	case ">=":
		return val >= cond.Threshold
	case "<":
		return val < cond.Threshold
	case "<=":
		return val <= cond.Threshold
	case "==":
		return val == cond.Threshold
	case "!=":
		return val != cond.Threshold
	default:
		return false
	}
}
