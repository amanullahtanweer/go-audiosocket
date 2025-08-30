package flow

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"
)

// SessionLogger writes structured JSONL session logs to a file
type SessionLogger struct {
    mu   sync.Mutex
    file *os.File
}

type logRecord struct {
    Timestamp   string            `json:"ts"`
    Event       string            `json:"event"`
    SessionID   string            `json:"session_id"`
    NodeID      string            `json:"node_id,omitempty"`
    NodeType    string            `json:"node_type,omitempty"`
    NodeContent string            `json:"node_content,omitempty"`
    Text        string            `json:"text,omitempty"`
    Classification string         `json:"classification,omitempty"`
    Interrupt   string            `json:"interrupt,omitempty"`
    NextNodeID  string            `json:"next_node_id,omitempty"`
    Details     map[string]string `json:"details,omitempty"`
}

// NewSessionLogger creates a logger under outputDir. Filename is timestamp + session id.
func NewSessionLogger(outputDir, sessionID string, started time.Time) (*SessionLogger, error) {
    if outputDir == "" {
        outputDir = "." // default current dir if not provided
    }
    if err := os.MkdirAll(outputDir, 0755); err != nil {
        return nil, err
    }
    shortID := sessionID
    if len(sessionID) > 8 {
        shortID = sessionID[:8]
    }
    filename := filepath.Join(outputDir, fmt.Sprintf("%s_session_%s.jsonl", started.Format("20060102_150405"), shortID))
    f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
    if err != nil {
        return nil, err
    }
    return &SessionLogger{file: f}, nil
}

func (sl *SessionLogger) Close() error {
    sl.mu.Lock()
    defer sl.mu.Unlock()
    if sl.file != nil {
        err := sl.file.Close()
        sl.file = nil
        return err
    }
    return nil
}

func (sl *SessionLogger) write(rec logRecord) {
    sl.mu.Lock()
    defer sl.mu.Unlock()
    if sl.file == nil {
        return
    }
    // sanitize text fields to keep lines compact
    rec.Text = strings.TrimSpace(rec.Text)
    enc := json.NewEncoder(sl.file)
    _ = enc.Encode(rec)
}

func (sl *SessionLogger) LogFlowStart(sessionID, name, version string, started time.Time) {
    sl.write(logRecord{Timestamp: started.Format(time.RFC3339Nano), Event: "flow_start", SessionID: sessionID, Details: map[string]string{"name": name, "version": version}})
}

func (sl *SessionLogger) LogFlowEnd(sessionID string, ended time.Time, reason string) {
    sl.write(logRecord{Timestamp: ended.Format(time.RFC3339Nano), Event: "flow_end", SessionID: sessionID, Details: map[string]string{"reason": reason}})
}

func (sl *SessionLogger) LogNodeStart(sessionID string, node *FlowNode) {
    sl.write(logRecord{Timestamp: time.Now().Format(time.RFC3339Nano), Event: "node_start", SessionID: sessionID, NodeID: node.ID, NodeType: node.Type, NodeContent: node.Content})
}

func (sl *SessionLogger) LogQnA(sessionID string, node *FlowNode, text, classification string) {
    sl.write(logRecord{Timestamp: time.Now().Format(time.RFC3339Nano), Event: "qna", SessionID: sessionID, NodeID: node.ID, NodeType: node.Type, NodeContent: node.Content, Text: text, Classification: classification})
}

func (sl *SessionLogger) LogInterrupt(sessionID string, node *FlowNode, text, interrupt string) {
    sl.write(logRecord{Timestamp: time.Now().Format(time.RFC3339Nano), Event: "interrupt", SessionID: sessionID, NodeID: node.ID, NodeType: node.Type, NodeContent: node.Content, Text: text, Interrupt: interrupt})
}

func (sl *SessionLogger) LogTransition(sessionID string, from, to *FlowNode, reason string) {
    toID := ""
    if to != nil {
        toID = to.ID
    }
    sl.write(logRecord{Timestamp: time.Now().Format(time.RFC3339Nano), Event: "transition", SessionID: sessionID, NodeID: from.ID, NodeType: from.Type, NodeContent: from.Content, NextNodeID: toID, Details: map[string]string{"reason": reason}})
}

func (sl *SessionLogger) LogTimeout(sessionID string, node *FlowNode) {
    sl.write(logRecord{Timestamp: time.Now().Format(time.RFC3339Nano), Event: "timeout", SessionID: sessionID, NodeID: node.ID, NodeType: node.Type, NodeContent: node.Content})
}

func (sl *SessionLogger) LogAPICall(sessionID string, endpoint, status string) {
    sl.write(logRecord{Timestamp: time.Now().Format(time.RFC3339Nano), Event: "api_call", SessionID: sessionID, Details: map[string]string{"endpoint": endpoint, "status": status}})
}

func (sl *SessionLogger) LogHangup(sessionID string) {
    sl.write(logRecord{Timestamp: time.Now().Format(time.RFC3339Nano), Event: "hangup", SessionID: sessionID})
}

func (sl *SessionLogger) LogTransfer(sessionID string, destination string) {
    sl.write(logRecord{Timestamp: time.Now().Format(time.RFC3339Nano), Event: "transfer", SessionID: sessionID, Details: map[string]string{"destination": destination}})
}

