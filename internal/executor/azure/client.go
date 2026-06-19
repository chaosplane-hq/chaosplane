package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type AzureClient struct {
	SubscriptionID string
	ResourceGroup  string
	BearerToken    string
	HTTP           *http.Client
}

func NewAzureClient(params map[string]string) *AzureClient {
	return &AzureClient{
		SubscriptionID: params["subscriptionId"],
		ResourceGroup:  params["resourceGroup"],
		BearerToken:    params["azureBearerToken"],
		HTTP:           &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *AzureClient) request(ctx context.Context, method, url string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("azure marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("azure create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.BearerToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("azure api request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("azure read response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("azure api error %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func (c *AzureClient) VMDeallocate(ctx context.Context, vmName string) error {
	url := fmt.Sprintf("https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s/deallocate?api-version=2024-03-01",
		c.SubscriptionID, c.ResourceGroup, vmName)
	_, err := c.request(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("azure vm deallocate %s: %w", vmName, err)
	}
	return nil
}

func (c *AzureClient) VMStart(ctx context.Context, vmName string) error {
	url := fmt.Sprintf("https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s/start?api-version=2024-03-01",
		c.SubscriptionID, c.ResourceGroup, vmName)
	_, err := c.request(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("azure vm start %s: %w", vmName, err)
	}
	return nil
}

func (c *AzureClient) AKSScaleNodePool(ctx context.Context, clusterName, nodePool string, count int) error {
	url := fmt.Sprintf("https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerService/managedClusters/%s/agentPools/%s?api-version=2024-01-01",
		c.SubscriptionID, c.ResourceGroup, clusterName, nodePool)
	body := map[string]interface{}{
		"properties": map[string]interface{}{
			"count": count,
		},
	}
	_, err := c.request(ctx, http.MethodPut, url, body)
	if err != nil {
		return fmt.Errorf("azure aks scale node pool %s/%s: %w", clusterName, nodePool, err)
	}
	return nil
}

func (c *AzureClient) CosmosDBFailover(ctx context.Context, accountName, targetRegion string) error {
	url := fmt.Sprintf("https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.DocumentDB/databaseAccounts/%s/failoverPriorityChange?api-version=2024-02-15-preview",
		c.SubscriptionID, c.ResourceGroup, accountName)
	body := map[string]interface{}{
		"failoverPolicies": []map[string]interface{}{
			{"locationName": targetRegion, "failoverPriority": 0},
		},
	}
	_, err := c.request(ctx, http.MethodPost, url, body)
	if err != nil {
		return fmt.Errorf("azure cosmosdb failover %s to %s: %w", accountName, targetRegion, err)
	}
	return nil
}
