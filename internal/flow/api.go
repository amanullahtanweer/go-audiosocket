package flow

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// APIClient handles HTTP requests to external services
type APIClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAPIClient creates a new API client
func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// MakeRequest makes an HTTP GET request with query parameters
func (api *APIClient) MakeRequest(endpoint string, params map[string]string) error {
	// Build URL with query parameters
	u, err := url.Parse(api.baseURL + endpoint)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %w", err)
	}
	
	// Add query parameters
	q := u.Query()
	for key, value := range params {
		q.Add(key, value)
	}
	u.RawQuery = q.Encode()
	
	// Make request
	resp, err := api.httpClient.Get(u.String())
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	
	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("API request failed with status: %d", resp.StatusCode)
	}
	
	return nil
}

// ReportCallStatus reports call status to the dialer
func (api *APIClient) ReportCallStatus(sessionID, status, reason string) error {
	params := map[string]string{
		"session_id": sessionID,
		"status":     status,
		"reason":     reason,
	}
	
	return api.MakeRequest("/call-status", params)
}

// AddToDNC adds a number to the do-not-call list
func (api *APIClient) AddToDNC(sessionID string) error {
	params := map[string]string{
		"session_id": sessionID,
		"action":     "add_to_dnc",
	}
	
	return api.MakeRequest("/add_to_dnc", params)
}

// MarkNotInterested marks a call as not interested
func (api *APIClient) MarkNotInterested(sessionID string) error {
	params := map[string]string{
		"session_id": sessionID,
		"action":     "mark_not_interested",
	}
	
	return api.MakeRequest("/mark_not_interested", params)
}

// ScheduleCallback schedules a callback for later
func (api *APIClient) ScheduleCallback(sessionID string) error {
	params := map[string]string{
		"session_id": sessionID,
		"action":     "schedule_callback",
	}
	
	return api.MakeRequest("/schedule_callback", params)
}

// TransferCall initiates a call transfer
func (api *APIClient) TransferCall(sessionID string) error {
	params := map[string]string{
		"session_id": sessionID,
		"action":     "transfer_call",
	}
	
	return api.MakeRequest("/transfer_call", params)
}

// EndCall ends the call
func (api *APIClient) EndCall(sessionID string) error {
	params := map[string]string{
		"session_id": sessionID,
		"action":     "end_call",
	}
	
	return api.MakeRequest("/end_call", params)
}
