// Package pb is a thin REST client for PocketBase (v0.23+). The API Server is
// the only component that talks to PocketBase: it authenticates as a superuser
// and performs all CRUD on behalf of users, enforcing ownership in app logic.
package pb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Record is a generic PocketBase record (a JSON object).
type Record map[string]any

// ListResult is the envelope returned by PocketBase list endpoints.
type ListResult struct {
	Page       int      `json:"page"`
	PerPage    int      `json:"perPage"`
	TotalItems int      `json:"totalItems"`
	TotalPages int      `json:"totalPages"`
	Items      []Record `json:"items"`
}

// APIError is returned for non-2xx PocketBase responses.
type APIError struct {
	Status  int
	Message string
	Body    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("pocketbase: %d %s", e.Status, e.Message)
}

// NotFound reports whether err is a 404 from PocketBase.
func NotFound(err error) bool {
	var ae *APIError
	if e, ok := err.(*APIError); ok {
		ae = e
	}
	return ae != nil && ae.Status == http.StatusNotFound
}

// Client is a PocketBase REST client with automatic superuser token refresh.
type Client struct {
	http *http.Client

	mu         sync.Mutex // guards baseURL/creds (retargetable at runtime) + cached token
	baseURL    string
	adminEmail string
	adminPass  string
	token      string
	tokenExp   time.Time
}

// New creates a PocketBase client.
func New(baseURL, adminEmail, adminPass string) *Client {
	return &Client{
		baseURL:    baseURL,
		adminEmail: adminEmail,
		adminPass:  adminPass,
		http:       &http.Client{Timeout: 15 * time.Second},
	}
}

// url returns the current base URL under lock.
func (c *Client) url() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.baseURL
}

// Configured reports whether service-account credentials are set. Every
// privileged flow (user management, admin CRUD) runs through them.
func (c *Client) Configured() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.adminEmail != "" && c.adminPass != ""
}

// Settings snapshots the current PocketBase connection for the settings panel.
func (c *Client) Settings() (baseURL, adminEmail, adminPass string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.baseURL, c.adminEmail, c.adminPass
}

// SetConfig retargets the client at a new PocketBase and/or new service-account
// credentials, invalidating any cached superuser token.
func (c *Client) SetConfig(baseURL, adminEmail, adminPass string) {
	c.mu.Lock()
	c.baseURL = baseURL
	c.adminEmail = adminEmail
	c.adminPass = adminPass
	c.token = "" // force re-auth against the new target
	c.tokenExp = time.Time{}
	c.mu.Unlock()
}

// AuthResult is returned by AuthWithPassword.
type AuthResult struct {
	Token  string `json:"token"`
	Record Record `json:"record"`
}

// authenticate logs in as a superuser and caches the token. It never holds the
// lock across the network call, so a concurrent SetConfig cannot deadlock it.
func (c *Client) authenticate(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		tok := c.token
		c.mu.Unlock()
		return tok, nil
	}
	email, pass := c.adminEmail, c.adminPass
	c.mu.Unlock()

	body := map[string]string{"identity": email, "password": pass}
	var res AuthResult
	if err := c.do(ctx, http.MethodPost,
		"/api/collections/_superusers/auth-with-password", "", body, &res); err != nil {
		return "", fmt.Errorf("superuser auth: %w", err)
	}

	c.mu.Lock()
	c.token = res.Token
	// PocketBase superuser tokens are long-lived; refresh conservatively.
	c.tokenExp = time.Now().Add(10 * time.Minute)
	tok := c.token
	c.mu.Unlock()
	return tok, nil
}

// Authenticate eagerly obtains a superuser token, surfacing bad credentials or
// an unreachable PocketBase early. Raw and the typed helpers also authenticate
// on demand, so calling this is optional.
func (c *Client) Authenticate(ctx context.Context) error {
	_, err := c.authenticate(ctx)
	return err
}

// AuthWithPassword verifies credentials against an auth collection (e.g. "users")
// and returns the user record. Used to validate logins; the API Server then
// issues its own JWT.
func (c *Client) AuthWithPassword(ctx context.Context, collection, identity, password string) (*AuthResult, error) {
	token, err := c.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	body := map[string]string{"identity": identity, "password": password}
	var res AuthResult
	path := "/api/collections/" + url.PathEscape(collection) + "/auth-with-password"
	if err := c.do(ctx, http.MethodPost, path, token, body, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// AuthRefresh validates a user's own auth token by asking PocketBase to refresh
// it, returning the user record and the HTTP status. A non-200 status (with a
// nil error) means the token is invalid or expired — callers branch on that
// rather than treating it as a transport failure. Because the token is resolved
// against PocketBase on every request, a role change or a deletion takes effect
// immediately instead of lingering until the token would expire.
func (c *Client) AuthRefresh(ctx context.Context, collection, userToken string) (*AuthResult, int, error) {
	path := "/api/collections/" + url.PathEscape(collection) + "/auth-refresh"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url()+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", userToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	var res AuthResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, resp.StatusCode, err
	}
	return &res, resp.StatusCode, nil
}

// SuperuserAuth performs a one-off superuser auth-with-password against an
// arbitrary PocketBase, returning the HTTP status. Used by the settings panel to
// probe a candidate connection without disturbing the live client.
func SuperuserAuth(ctx context.Context, client *http.Client, baseURL, email, password string) (int, error) {
	body, _ := json.Marshal(map[string]string{"identity": email, "password": password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(baseURL, "/")+"/api/collections/_superusers/auth-with-password", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("superuser auth failed: HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// Create inserts a record into a collection.
func (c *Client) Create(ctx context.Context, collection string, data any) (Record, error) {
	token, err := c.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	var out Record
	path := "/api/collections/" + url.PathEscape(collection) + "/records"
	if err := c.do(ctx, http.MethodPost, path, token, data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Update patches a record.
func (c *Client) Update(ctx context.Context, collection, id string, data any) (Record, error) {
	token, err := c.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	var out Record
	path := "/api/collections/" + url.PathEscape(collection) + "/records/" + url.PathEscape(id)
	if err := c.do(ctx, http.MethodPatch, path, token, data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Delete removes a record.
func (c *Client) Delete(ctx context.Context, collection, id string) error {
	token, err := c.authenticate(ctx)
	if err != nil {
		return err
	}
	path := "/api/collections/" + url.PathEscape(collection) + "/records/" + url.PathEscape(id)
	return c.do(ctx, http.MethodDelete, path, token, nil, nil)
}

// GetOne fetches a single record by id.
func (c *Client) GetOne(ctx context.Context, collection, id string) (Record, error) {
	token, err := c.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	var out Record
	path := "/api/collections/" + url.PathEscape(collection) + "/records/" + url.PathEscape(id)
	if err := c.do(ctx, http.MethodGet, path, token, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListOptions controls a list query.
type ListOptions struct {
	Filter  string
	Sort    string
	Expand  string
	Page    int
	PerPage int
}

// List queries records with optional filter/sort/pagination.
func (c *Client) List(ctx context.Context, collection string, opt ListOptions) (*ListResult, error) {
	token, err := c.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	if opt.Filter != "" {
		q.Set("filter", opt.Filter)
	}
	if opt.Sort != "" {
		q.Set("sort", opt.Sort)
	}
	if opt.Expand != "" {
		q.Set("expand", opt.Expand)
	}
	if opt.Page > 0 {
		q.Set("page", strconv.Itoa(opt.Page))
	}
	if opt.PerPage > 0 {
		q.Set("perPage", strconv.Itoa(opt.PerPage))
	}
	path := "/api/collections/" + url.PathEscape(collection) + "/records?" + q.Encode()
	var out ListResult
	if err := c.do(ctx, http.MethodGet, path, token, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FindFirst returns the first record matching filter, or (nil, nil) if none.
func (c *Client) FindFirst(ctx context.Context, collection, filter, sort string) (Record, error) {
	res, err := c.List(ctx, collection, ListOptions{Filter: filter, Sort: sort, PerPage: 1})
	if err != nil {
		return nil, err
	}
	if len(res.Items) == 0 {
		return nil, nil
	}
	return res.Items[0], nil
}

// Raw performs an authenticated request against PocketBase and returns the raw
// response body and HTTP status. Unlike do, a non-2xx status is not an error —
// the caller inspects the status itself. Used by the bootstrap package, which
// reconciles collections and tolerates the expected 4xx (e.g. a duplicate). On a
// 401 it drops the cached token, re-authenticates once, and retries.
func (c *Client) Raw(ctx context.Context, method, path string, payload any) ([]byte, int, error) {
	raw, status, err := c.rawAttempt(ctx, method, path, payload)
	if err != nil {
		return nil, 0, err
	}
	if status == http.StatusUnauthorized {
		c.mu.Lock()
		c.token = ""
		c.tokenExp = time.Time{}
		c.mu.Unlock()
		return c.rawAttempt(ctx, method, path, payload)
	}
	return raw, status, nil
}

func (c *Client) rawAttempt(ctx context.Context, method, path string, payload any) ([]byte, int, error) {
	token, err := c.authenticate(ctx)
	if err != nil {
		return nil, 0, err
	}
	var reader io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url()+path, reader)
	if err != nil {
		return nil, 0, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, nil
}

// do performs an HTTP request against PocketBase and decodes the JSON response.
func (c *Client) do(ctx context.Context, method, path, token string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.url()+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := http.StatusText(resp.StatusCode)
		var pbErr struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &pbErr) == nil && pbErr.Message != "" {
			msg = pbErr.Message
		}
		return &APIError{Status: resp.StatusCode, Message: msg, Body: string(raw)}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
