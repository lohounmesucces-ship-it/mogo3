package sysdig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	Endpoint string
	Timeout  time.Duration
}

func New(endpoint string) *Client {
	return &Client{
		Endpoint: endpoint,
		Timeout:  25 * time.Second,
	}
}

func (c *Client) CreateAlert(ctx context.Context, ibmInstanceID, teamID, bearer string, alertJSON []byte) (map[string]any, error) {
	url := strings.TrimRight(c.Endpoint, "/") + "/api/v2/alerts"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(alertJSON))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", bearer)
	req.Header.Set("IBMInstanceID", ibmInstanceID)
	req.Header.Set("SysdigTeamID", teamID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	hc := &http.Client{Timeout: c.Timeout}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sysdig create alert failed: %s - %s", resp.Status, string(body))
	}

	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return map[string]any{"raw": string(body)}, nil
	}
	return out, nil
}
