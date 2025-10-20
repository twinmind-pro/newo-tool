package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"time"
)

const (
	defaultHTTPTimeout = 30 * time.Second
	maxErrorBodyBytes  = 512 << 10
)

var defaultTransport http.RoundTripper = http.DefaultTransport

// SetTransportForTesting overrides the transport used for outbound HTTP calls. The caller must invoke the returned
// cleanup function to restore the previous transport when finished.
func SetTransportForTesting(rt http.RoundTripper) func() {
	prev := defaultTransport
	if rt == nil {
		rt = http.DefaultTransport
	}
	defaultTransport = rt
	return func() {
		defaultTransport = prev
	}
}

// Client wraps HTTP access to the NEWO platform.
type Client struct {
	base *url.URL
	http *http.Client
}

// ClientOption customises the client behaviour.
type ClientOption func(*Client)

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(h *http.Client) ClientOption {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

// NewClient constructs a platform client using the supplied bearer token.
func NewClient(baseURL, token string, opts ...ClientOption) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("token is required")
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}

	client := &Client{
		base: u,
		http: &http.Client{
			Timeout: defaultHTTPTimeout,
			Transport: &authTransport{
				base:  defaultTransport,
				token: token,
			},
		},
	}

	for _, opt := range opts {
		opt(client)
	}

	// Ensure custom transport also wraps token
	if _, ok := client.http.Transport.(*authTransport); !ok {
		client.http.Transport = &authTransport{
			base:  client.http.Transport,
			token: token,
		}
	}

	return client, nil
}

type authTransport struct {
	base  http.RoundTripper
	token string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.base == nil {
		t.base = defaultTransport
	}
	req2 := cloneRequest(req)
	req2.Header.Set("Authorization", "Bearer "+t.token)
	req2.Header.Set("Accept", "application/json")
	return t.base.RoundTrip(req2)
}

func cloneRequest(r *http.Request) *http.Request {
	r2 := r.Clone(r.Context())
	if r.Body != nil {
		buf, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewBuffer(buf))
		r2.Body = io.NopCloser(bytes.NewBuffer(buf))
	}
	return r2
}

func (c *Client) buildURL(p string, query map[string]string) string {
	u := *c.base
	u.Path = path.Join(c.base.Path, p)
	if len(query) > 0 {
		q := u.Query()
		for k, v := range query {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func (c *Client) do(ctx context.Context, method, path string, query map[string]string, body any, dest any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.buildURL(path, query), reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call %s %s: %w", method, path, networkError(err))
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return &APIError{
			Method: method,
			Path:   path,
			Status: resp.StatusCode,
			Body:   string(bytes.TrimSpace(payload)),
		}
	}

	if dest == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

// ListProjects returns all projects visible to the customer.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var projects []Project
	if err := c.do(ctx, http.MethodGet, "/api/v1/designer/projects", nil, nil, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

// GetProject retrieves project metadata by ID.
func (c *Client) GetProject(ctx context.Context, projectID string) (Project, error) {
	var project Project
	if err := c.do(ctx, http.MethodGet, "/api/v1/designer/projects/by-id/"+projectID, nil, nil, &project); err != nil {
		return Project{}, err
	}
	return project, nil
}

// ListAgents returns agents for a project.
func (c *Client) ListAgents(ctx context.Context, projectID string) ([]Agent, error) {
	var agents []Agent
	if err := c.do(ctx, http.MethodGet, "/api/v1/bff/agents/list", map[string]string{"project_id": projectID}, nil, &agents); err != nil {
		return nil, err
	}
	return agents, nil
}

// ListFlowSkills returns skills in a flow.
func (c *Client) ListFlowSkills(ctx context.Context, flowID string) ([]Skill, error) {
	var skills []Skill
	if err := c.do(ctx, http.MethodGet, "/api/v1/designer/flows/"+flowID+"/skills", nil, nil, &skills); err != nil {
		return nil, err
	}
	return skills, nil
}

// GetSkill retrieves a skill by ID.
func (c *Client) GetSkill(ctx context.Context, skillID string) (Skill, error) {
	var skill Skill
	if err := c.do(ctx, http.MethodGet, "/api/v1/designer/skills/"+skillID, nil, nil, &skill); err != nil {
		return Skill{}, err
	}
	return skill, nil
}

// ListFlowEvents returns events attached to a flow.
func (c *Client) ListFlowEvents(ctx context.Context, flowID string) ([]FlowEvent, error) {
	var events []FlowEvent
	if err := c.do(ctx, http.MethodGet, "/api/v1/designer/flows/"+flowID+"/events", nil, nil, &events); err != nil {
		return nil, err
	}
	return events, nil
}

// ListFlowStates returns states attached to a flow.
func (c *Client) ListFlowStates(ctx context.Context, flowID string) ([]FlowState, error) {
	var states []FlowState
	if err := c.do(ctx, http.MethodGet, "/api/v1/designer/flows/"+flowID+"/states", nil, nil, &states); err != nil {
		return nil, err
	}
	return states, nil
}

// UpdateSkill updates a flow skill with new metadata and script.
func (c *Client) UpdateSkill(ctx context.Context, skillID string, payload UpdateSkillRequest) error {
	return c.do(ctx, http.MethodPut, "/api/v1/designer/flows/skills/"+skillID, nil, payload, nil)
}

// GetCustomerProfile returns information about the authenticated customer.
func (c *Client) GetCustomerProfile(ctx context.Context) (CustomerProfile, error) {
	var profile CustomerProfile
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/profile", nil, nil, &profile); err != nil {
		return CustomerProfile{}, err
	}
	return profile, nil
}

// GetCustomerAttributes fetches customer attributes.
func (c *Client) GetCustomerAttributes(ctx context.Context, includeHidden bool) (CustomerAttributesResponse, error) {
	query := map[string]string{}
	if includeHidden {
		query["include_hidden"] = "true"
	}
	var resp CustomerAttributesResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/bff/customer/attributes", query, nil, &resp); err != nil {
		return CustomerAttributesResponse{}, err
	}
	return resp, nil
}

// PublishFlow publishes a flow after updates.
func (c *Client) PublishFlow(ctx context.Context, flowID string, payload PublishFlowRequest) error {
	return c.do(ctx, http.MethodPost, "/api/v1/designer/flows/"+flowID+"/publish", nil, payload, nil)
}

func networkError(err error) error {
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return fmt.Errorf("request timeout: %w", err)
	}
	return err
}
