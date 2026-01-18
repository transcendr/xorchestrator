package texthistory

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type claudeHistoryEntry struct {
	Display        string         `json:"display"`
	PastedContents map[string]any `json:"pastedContents"`
	Timestamp      int64          `json:"timestamp"`
	Project        string         `json:"project"`
	SessionID      string         `json:"sessionId,omitempty"`
}

// LoadClaudeHistory reads Claude's history.jsonl and returns a HistoryManager.
// Unknown or malformed lines are ignored.
func LoadClaudeHistory(path string, fallbackMax int) (*HistoryManager, error) {
	if fallbackMax <= 0 {
		fallbackMax = DefaultMaxHistory
	}
	h := NewHistoryManager(fallbackMax)

	if path == "" {
		return h, nil
	}

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return h, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Allow long lines (2MB)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		if text := extractClaudeText(raw); text != "" {
			h.Add(text)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return h, nil
}

// AppendClaudeHistory appends a Claude-compatible JSONL entry to history.jsonl.
func AppendClaudeHistory(path, entry string) error {
	if strings.TrimSpace(entry) == "" {
		return nil
	}
	if path == "" {
		return errors.New("claude history path is empty")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	project, _ := os.Getwd()
	if project == "" {
		project = ""
	}

	payload := claudeHistoryEntry{
		Display:        entry,
		PastedContents: map[string]any{},
		Timestamp:      time.Now().UnixMilli(),
		Project:        project,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = f.WriteString(string(data) + "\n")
	return err
}

func extractClaudeText(raw map[string]any) string {
	// Check "display" first (Claude Code format)
	if v, ok := raw["display"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}

	// Check legacy keys for backward compatibility
	for _, key := range []string{"text", "input", "prompt", "message", "content"} {
		if v, ok := raw[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}

	// Try messages array: [{role, content}]
	if msgs, ok := raw["messages"].([]any); ok {
		for i := len(msgs) - 1; i >= 0; i-- {
			if m, ok := msgs[i].(map[string]any); ok {
				role, _ := m["role"].(string)
				if role == "user" {
					if content, ok := m["content"].(string); ok {
						return content
					}
				}
			}
		}
	}

	return ""
}
