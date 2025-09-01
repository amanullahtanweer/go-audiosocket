package flow

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// APIClient implements Vicidial-related API calls
type APIClient struct {
    serverURL   string
    adminDir    string
    apiUser     string
    apiPass     string
    sourceRA    string // e.g., "igent"
    sourceAdmin string // e.g., "test" or "igent"

    transferStatus     string // e.g., "LVXFER"
    transferPhone      string // e.g., "26000"

    httpClient *http.Client

    // Redis for session-scoped variables
    redis       *redis.Client
    redisPrefix string
}

// NewVicidialClient constructs a fully configured API client
func NewVicidialClient(serverURL, adminDir, apiUser, apiPass, sourceRA, sourceAdmin, transferStatus, transferPhone string) *APIClient {
    return &APIClient{
        serverURL:   strings.TrimRight(serverURL, "/"),
        adminDir:    strings.Trim(adminDir, "/"),
        apiUser:     apiUser,
        apiPass:     apiPass,
        sourceRA:    sourceRA,
        sourceAdmin: sourceAdmin,
        transferStatus: transferStatus,
        transferPhone:  transferPhone,
        httpClient: &http.Client{Timeout: 10 * time.Second},
    }
}

// SetRedis attaches a Redis client used to resolve session variables
func (api *APIClient) SetRedis(client *redis.Client, prefix string) {
    api.redis = client
    api.redisPrefix = prefix
}

func (api *APIClient) getVar(ctx context.Context, sessionID, key string) (string, error) {
    if api.redis == nil {
        return "", fmt.Errorf("redis client not configured")
    }
    redisKey := api.redisPrefix + sessionID
    val, err := api.redis.HGet(ctx, redisKey, key).Result()
    if err != nil || val == "" {
        if err != nil {
            return "", fmt.Errorf("redis HGET %s %s: %w", redisKey, key, err)
        }
        return "", fmt.Errorf("redis HGET %s %s: empty", redisKey, key)
    }
    return val, nil
}

// Convenience wrappers that resolve vars by session UUID
func (api *APIClient) UpdateRaCallControlBySession(sessionID, stage, status, phone string) error {
    ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
    defer cancel()
    // We no longer rely on agent_user in Redis; resolve via API using lead_id
    leadID, err := api.getVar(ctx, sessionID, "lead_id")
    if err != nil {
        return err
    }
    agentUser, err := api.GetAgentUserByLead(leadID)
    if err != nil {
        // If unavailable, proceed with empty agent user
        agentUser = ""
    }
    display, err := api.getVar(ctx, sessionID, "display")
    if err != nil {
        return err
    }
    return api.UpdateRaCallControl(agentUser, stage, status, display, phone)
}

func (api *APIClient) UpdateLeadStatusBySession(sessionID, status string) error {
    ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
    defer cancel()
    leadID, err := api.getVar(ctx, sessionID, "lead_id")
    if err != nil {
        return err
    }
    return api.UpdateLeadStatus(leadID, status)
}

func (api *APIClient) UpdateLogEntryBySession(sessionID, status string) error {
    ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
    defer cancel()
    campaignID, err := api.getVar(ctx, sessionID, "campaign_id")
    if err != nil {
        return err
    }
    callID, err := api.getVar(ctx, sessionID, "display")
    if err != nil {
        return err
    }
    return api.UpdateLogEntry(campaignID, callID, status)
}

// SetVicidialConfig updates client configuration
func (api *APIClient) SetVicidialConfig(serverURL, adminDir, apiUser, apiPass, sourceRA, sourceAdmin, transferStatus, transferPhone string) {
    api.serverURL = strings.TrimRight(serverURL, "/")
    api.adminDir = strings.Trim(adminDir, "/")
    api.apiUser = apiUser
    api.apiPass = apiPass
    api.sourceRA = sourceRA
    api.sourceAdmin = sourceAdmin
    api.transferStatus = transferStatus
    api.transferPhone = transferPhone
}

// makeRequest performs a GET request to a full URL with params
func (api *APIClient) makeRequest(fullURL string, params map[string]string) error {
    u, err := url.Parse(fullURL)
    if err != nil {
        return fmt.Errorf("failed to parse URL: %w", err)
    }
    q := u.Query()
    for k, v := range params {
        q.Set(k, v)
    }
    u.RawQuery = q.Encode()

    resp, err := api.httpClient.Get(u.String())
    if err != nil {
        return fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }
    return nil
}

// UpdateRaCallControl -> {SERVER_URL}/agc/api.php
func (api *APIClient) UpdateRaCallControl(agentUser, stage, status, display string, phoneNumber string) error {
    fullURL := api.serverURL + "/agc/api.php"
    params := map[string]string{
        "source":    api.sourceRA,
        "user":      api.apiUser,
        "pass":      api.apiPass,
        "agent_user": agentUser,
        "function":  "ra_call_control",
        "stage":     stage,
        "status":    status,
        "value":     display,
    }
    if phoneNumber != "" {
        params["phone_number"] = phoneNumber
    }
    return api.makeRequest(fullURL, params)
}

// UpdateLeadStatus -> {SERVER_URL}/{ADMIN_DIR}/non_agent_api.php
func (api *APIClient) UpdateLeadStatus(leadID, status string) error {
    fullURL := api.serverURL + "/" + path.Join(api.adminDir, "non_agent_api.php")
    params := map[string]string{
        "source":   api.sourceAdmin,
        "user":     api.apiUser,
        "pass":     api.apiPass,
        "function": "update_lead",
        "lead_id":  leadID,
        "status":   status,
    }
    return api.makeRequest(fullURL, params)
}

// UpdateLogEntry -> {SERVER_URL}/{ADMIN_DIR}/non_agent_api.php
func (api *APIClient) UpdateLogEntry(campaignID, callID, status string) error {
    fullURL := api.serverURL + "/" + path.Join(api.adminDir, "non_agent_api.php")
    params := map[string]string{
        "source":   api.sourceRA,
        "user":     api.apiUser,
        "pass":     api.apiPass,
        "function": "update_log_entry",
        "group":    campaignID,
        "call_id":  callID,
        "status":   status,
    }
    return api.makeRequest(fullURL, params)
}

// GetAgentUserByLead queries Vicidial for the agent (user) handling a lead
// Equivalent to the Python get_agent_user_info(lead_id)
func (api *APIClient) GetAgentUserByLead(leadID string) (string, error) {
    if strings.TrimSpace(leadID) == "" {
        return "", fmt.Errorf("leadID is empty")
    }
    fullURL := api.serverURL + "/" + path.Join(api.adminDir, "non_agent_api.php")
    u, err := url.Parse(fullURL)
    if err != nil {
        return "", fmt.Errorf("failed to parse URL: %w", err)
    }
    q := u.Query()
    q.Set("source", api.sourceAdmin)
    q.Set("user", api.apiUser)
    q.Set("pass", api.apiPass)
    q.Set("function", "lead_field_info")
    q.Set("lead_id", leadID)
    q.Set("field_name", "user")
    q.Set("custom_fields", "N")
    q.Set("archived_lead", "N")
    u.RawQuery = q.Encode()

    resp, err := api.httpClient.Get(u.String())
    if err != nil {
        return "", fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("read body: %w", err)
    }
    return strings.TrimSpace(string(body)), nil
}

// Helpers to expose configured transfer params
func (api *APIClient) TransferStatus() string { return api.transferStatus }
func (api *APIClient) TransferPhone() string  { return api.transferPhone }
