package gcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type GCPClient struct {
	ProjectID   string
	BearerToken string
	HTTP        *http.Client
}

func NewGCPClient(params map[string]string) *GCPClient {
	return &GCPClient{
		ProjectID:   params["projectId"],
		BearerToken: params["gcpBearerToken"],
		HTTP:        &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *GCPClient) request(ctx context.Context, method, url string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("gcp marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("gcp create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.BearerToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gcp api request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gcp read response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gcp api error %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func (c *GCPClient) GKESetNodePoolSize(ctx context.Context, zone, cluster, nodePool string, size int) error {
	url := fmt.Sprintf("https://container.googleapis.com/v1/projects/%s/zones/%s/clusters/%s/nodePools/%s/setSize",
		c.ProjectID, zone, cluster, nodePool)
	body := map[string]interface{}{"nodeCount": size}
	_, err := c.request(ctx, http.MethodPost, url, body)
	if err != nil {
		return fmt.Errorf("gcp gke set node pool size %s/%s: %w", cluster, nodePool, err)
	}
	return nil
}

func (c *GCPClient) CloudSQLFailover(ctx context.Context, instance string) error {
	url := fmt.Sprintf("https://sqladmin.googleapis.com/v1/projects/%s/instances/%s/failover",
		c.ProjectID, instance)
	body := map[string]interface{}{
		"failoverContext": map[string]interface{}{
			"kind": "sql#failoverContext",
		},
	}
	_, err := c.request(ctx, http.MethodPost, url, body)
	if err != nil {
		return fmt.Errorf("gcp cloudsql failover %s: %w", instance, err)
	}
	return nil
}

func (c *GCPClient) CloudRunUpdateTraffic(ctx context.Context, region, service string, percent int) error {
	url := fmt.Sprintf("https://run.googleapis.com/v2/projects/%s/locations/%s/services/%s",
		c.ProjectID, region, service)
	body := map[string]interface{}{
		"traffic": []map[string]interface{}{
			{"type": "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST", "percent": percent},
		},
	}
	_, err := c.request(ctx, http.MethodPatch, url, body)
	if err != nil {
		return fmt.Errorf("gcp cloudrun update traffic %s/%s: %w", region, service, err)
	}
	return nil
}
