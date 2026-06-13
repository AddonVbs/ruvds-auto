package ruvds

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const BaseURL = "https://api.ruvds.com"

type Client struct {
	token string
	http  *http.Client
}

func New(token string) *Client {
	return &Client{
		token: token,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

type ServerCreateReq struct {
	Datacenter    int     `json:"datacenter"`
	TariffID      int     `json:"tariff_id"`
	OSID          int     `json:"os_id"`
	PaymentPeriod int     `json:"payment_period"`
	CPU           int     `json:"cpu"`
	RAM           float64 `json:"ram"`
	Drive         int     `json:"drive"`
	DriveTariffID int     `json:"drive_tariff_id"`
	IP            int     `json:"ip"`
	ComputerName  string  `json:"computer_name"`
	UserComment   string  `json:"user_comment,omitempty"`
}

type Action struct {
	ID       int    `json:"id"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
}

type ServerCreateResp struct {
	VirtualServerID int     `json:"virtual_server_id"`
	CostRub         float64 `json:"cost_rub"`
	Password        string  `json:"password"`
	Action          Action  `json:"action"`
}

type NetworkV4 struct {
	IPAddress string `json:"ip_address"`
	Netmask   string `json:"netmask"`
	Gateway   string `json:"gateway"`
}

type NetworksResp struct {
	V4 []NetworkV4 `json:"v4"`
}

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("ruvds api error %d: %s", e.StatusCode, e.Body)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, BaseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: string(raw)}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

type Datacenter struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	VPSTariffs    []int  `json:"vps_tariffs"`
	DriveTariffs  []int  `json:"drive_tariffs"`
}

func (c *Client) ListDatacenters(ctx context.Context) ([]Datacenter, error) {
	var out struct {
		Datacenters []Datacenter `json:"datacenters"`
	}
	if err := c.do(ctx, http.MethodGet, "/v2/datacenters", nil, &out); err != nil {
		return nil, err
	}
	return out.Datacenters, nil
}

func (c *Client) CreateServer(ctx context.Context, req ServerCreateReq) (*ServerCreateResp, error) {
	var out ServerCreateResp
	if err := c.do(ctx, http.MethodPost, "/v2/servers", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetAction(ctx context.Context, id int) (*Action, error) {
	var out Action
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/v2/actions/%d", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetNetworks(ctx context.Context, serverID int) (*NetworksResp, error) {
	var out NetworksResp
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/v2/servers/%d/networks", serverID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteServer(ctx context.Context, serverID int) (*Action, error) {
	var out Action
	if err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/v2/servers/%d", serverID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WaitAction polls the action until it reaches a terminal state or ctx is done.
func (c *Client) WaitAction(ctx context.Context, id int, poll time.Duration) (*Action, error) {
	t := time.NewTicker(poll)
	defer t.Stop()
	for {
		a, err := c.GetAction(ctx, id)
		if err != nil {
			return nil, err
		}
		switch a.Status {
		case "success", "error", "wait_user_action":
			return a, nil
		}
		select {
		case <-ctx.Done():
			return a, ctx.Err()
		case <-t.C:
		}
	}
}
