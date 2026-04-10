package probe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
)

type httpProbe struct {
	spec   v1alpha1.HTTPProbe
	client *http.Client
}

func (p *httpProbe) Run(ctx context.Context) (bool, error) {
	if p.client == nil {
		p.client = http.DefaultClient
	}

	method := p.spec.Method
	if method == "" {
		method = http.MethodGet
	}

	req, err := http.NewRequestWithContext(ctx, method, p.spec.URL, nil)
	if err != nil {
		return false, fmt.Errorf("building request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if p.spec.ExpectedStatus != 0 && resp.StatusCode != p.spec.ExpectedStatus {
		return false, nil
	}

	if p.spec.ExpectedBody != "" {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return false, fmt.Errorf("reading body: %w", err)
		}
		matched, err := regexp.MatchString(p.spec.ExpectedBody, string(body))
		if err != nil {
			return false, fmt.Errorf("matching body regex: %w", err)
		}
		if !matched {
			return false, nil
		}
	}

	return true, nil
}
