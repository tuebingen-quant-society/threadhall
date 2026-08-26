package agentd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/agentd/codex"
	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
)

const maxWorkerResponseBytes = 256 << 10

type Client struct {
	baseURL *url.URL
	token   string
	http    *http.Client
}

func NewClient(rawURL, token string, httpClient *http.Client) (*Client, error) {
	baseURL, err := url.Parse(rawURL)
	if err != nil || baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" ||
		(baseURL.Scheme != "https" && !(baseURL.Scheme == "http" && loopbackHost(baseURL.Hostname()))) || strings.TrimSpace(token) == "" {
		return nil, errors.New("worker URL must be HTTPS or loopback HTTP and include a token")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{baseURL: baseURL, token: token, http: httpClient}, nil
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *Client) Next(ctx context.Context) (agenttask.Work, bool, error) {
	request, err := c.request(ctx, http.MethodGet, "/api/v1/agent/work", nil)
	if err != nil {
		return agenttask.Work{}, false, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return agenttask.Work{}, false, fmt.Errorf("claim Threadhall work: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return agenttask.Work{}, false, nil
	}
	if response.StatusCode != http.StatusOK {
		return agenttask.Work{}, false, responseError("claim Threadhall work", response)
	}
	var work agenttask.Work
	if err := decodeBounded(response.Body, &work); err != nil || work.Task.ID <= 0 || strings.TrimSpace(work.Prompt) == "" {
		return agenttask.Work{}, false, errors.New("Threadhall returned invalid bounded work")
	}
	return work, true, nil
}

func (c *Client) Progress(ctx context.Context, taskID int64, summary string) error {
	return c.post(ctx, taskID, "progress", map[string]string{"summary": summary})
}

func (c *Client) Complete(ctx context.Context, taskID int64, output, runtimeID string) error {
	return c.post(ctx, taskID, "complete", map[string]string{"output": output, "runtime_thread_id": runtimeID})
}

func (c *Client) Fail(ctx context.Context, taskID int64, failure error) error {
	reason := "runtime_failed"
	if errors.Is(failure, codex.ErrInteractionRequired) {
		reason = "interaction_unsupported"
	}
	return c.post(ctx, taskID, "fail", map[string]string{"reason": reason})
}

func (c *Client) post(ctx context.Context, taskID int64, action string, payload any) error {
	if taskID <= 0 {
		return errors.New("positive task id is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	path := "/api/v1/agent/tasks/" + strconv.FormatInt(taskID, 10) + "/" + action
	request, err := c.request(ctx, http.MethodPost, path, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("post Threadhall task %s: %w", action, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return responseError("post Threadhall task "+action, response)
	}
	return nil
}

func (c *Client) request(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	target := c.baseURL.ResolveReference(&url.URL{Path: path})
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Accept", "application/json")
	return request, nil
}

func decodeBounded(reader io.Reader, destination any) error {
	decoder := json.NewDecoder(io.LimitReader(reader, maxWorkerResponseBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	return nil
}

func responseError(action string, response *http.Response) error {
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 8<<10))
	return fmt.Errorf("%s: HTTP %d", action, response.StatusCode)
}
