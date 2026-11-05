// Package typesafe implements the direct TypeSafe System One contract.
// It performs no model/provider fallback, source retrieval, or automatic retries.
package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/tphakala/jev-mcp/internal/evidence"
)

const (
	DefaultEndpoint      = "https://api.typesafe.ai"
	defaultHTTPTimeout   = 30 * time.Second
	defaultByteLimit     = 1 << 20
	maxIdentityBytes     = 128
	probabilityTolerance = 0.001
	errTransport         = "provider_transport"
	errRequest           = "invalid_provider_request"
	scoreLevelCount      = 3
)

type Config struct {
	APIKey           string
	Model            string
	Endpoint         string
	HTTPClient       *http.Client
	MaxRequestBytes  int
	MaxResponseBytes int
}

type Client struct {
	key, model, endpoint    string
	http                    *http.Client
	maxRequest, maxResponse int
}

// Error contains only an operational code and status, never provider text, URLs,
// credentials, or request content. Core owns retries under its total call cap.
type Error struct {
	Code       string
	HTTPStatus int
}

func (e *Error) Error() string { return e.Code }
func (e *Error) Temporary() bool {
	return e.Code == errTransport || e.HTTPStatus == http.StatusTooManyRequests || e.HTTPStatus >= http.StatusInternalServerError
}

// Retryable marks transient transport and HTTP failures for the core's bounded
// retry policy. An adapter call itself always makes exactly one HTTP attempt.
func (e *Error) Retryable() bool { return e.Temporary() }

func New(c Config) (*Client, error) {
	if c.Endpoint == "" {
		c.Endpoint = DefaultEndpoint
	}
	if !validEndpoint(c.Endpoint) {
		return nil, &Error{Code: "invalid_provider_endpoint"}
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, &Error{Code: "provider_key_missing"}
	}
	if c.Model != "" && (!safeIdentity(c.Model) || strings.Contains(c.Model, c.APIKey)) {
		return nil, &Error{Code: "invalid_provider_model"}
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	// Clone before disabling redirects: a redirect must never carry credentials.
	hc := *c.HTTPClient
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if c.MaxRequestBytes <= 0 {
		c.MaxRequestBytes = defaultByteLimit
	}
	if c.MaxResponseBytes <= 0 {
		c.MaxResponseBytes = defaultByteLimit
	}
	return &Client{c.APIKey, c.Model, strings.TrimRight(c.Endpoint, "/"), &hc, c.MaxRequestBytes, c.MaxResponseBytes}, nil
}

type wireQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria"`
}
type wireAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Noul          *float64           `json:"noul,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
}

// Evaluate submits one independent batch; dependent batches are core decisions.
func (c *Client) Evaluate(ctx context.Context, calls []evidence.Call) ([]evidence.Answer, error) {
	if c.model == "" {
		return nil, &Error{Code: "provider_model_missing"}
	}
	body, err := batchRequest(c.model, calls)
	if err != nil {
		return nil, err
	}
	if len(body) > c.maxRequest {
		return nil, &Error{Code: "provider_input_limit"}
	}
	response, err := c.request(ctx, http.MethodPost, "/v1/systemone", body)
	if err != nil {
		return nil, err
	}
	return c.decodeBatch(calls, response), nil
}

func invalidBatch(calls []evidence.Call) []evidence.Answer {
	results := make([]evidence.Answer, 0, len(calls))
	for _, call := range calls {
		results = append(results, evidence.Answer{ID: call.ID, Status: evidence.StatusFailed, Validity: evidence.ValidityInvalid, Error: evidence.OutcomeInvalidProvider})
	}
	return results
}

func unit(p *float64) bool {
	return p != nil && !math.IsNaN(*p) && !math.IsInf(*p, 0) && *p >= 0 && *p <= 1
}

func distribution(probs map[string]float64, options map[string]string) bool {
	if len(probs) != len(options) {
		return false
	}
	sum := 0.0
	for k, p := range probs {
		if _, ok := options[k]; !ok || p < 0 || p > 1 || math.IsNaN(p) || math.IsInf(p, 0) {
			return false
		}
		sum += p
	}
	return math.Abs(sum-1) <= probabilityTolerance
}

func safeIdentity(s string) bool {
	if len(s) > maxIdentityBytes {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._:/-", r) {
			return false
		}
	}
	return s != ""
}

func (c *Client) request(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, bytes.NewReader(body))
	if err != nil {
		return nil, &Error{Code: errRequest}
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &Error{Code: errTransport}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, &Error{Code: "provider_http", HTTPStatus: resp.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(c.maxResponse)+1))
	if err != nil {
		return nil, &Error{Code: errTransport}
	}
	if len(data) > c.maxResponse {
		return nil, &Error{Code: "provider_response_limit"}
	}
	return data, nil
}

// Models performs explicit authenticated discovery only when called. The server
// never calls it during startup/status, and it never selects a substitute model.
func (c *Client) Models(ctx context.Context) ([]string, error) {
	body, err := c.request(ctx, http.MethodGet, "/v1/models", nil)
	if err != nil {
		return nil, err
	}
	var r struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if json.Unmarshal(body, &r) != nil || r.Models == nil {
		return nil, &Error{Code: "invalid_provider_output"}
	}
	out := make([]string, 0, len(r.Models))
	for _, m := range r.Models {
		if !safeIdentity(m.Name) || strings.Contains(m.Name, c.key) {
			return nil, &Error{Code: "invalid_provider_output"}
		}
		out = append(out, m.Name)
	}
	slices.Sort(out)
	return out, nil
}

var (
	_ evidence.Provider = (*Client)(nil)
	_ error             = (*Error)(nil)
)

func validEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	return u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1")
}
