package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// HistoryEntry represents a single connection record.
type HistoryEntry struct {
	Timestamp time.Time `json:"ts"`
	Profile   string    `json:"profile"`
	Duration  string    `json:"duration,omitempty"`  // human-readable, e.g. "42s"
	DurationMs int64    `json:"duration_ms"`
	Command   string    `json:"command,omitempty"`   // empty for interactive shell
	ExitCode  int       `json:"exit_code"`
}

// historyPath returns the path to the history file.
func HistoryPath() string {
	if p := os.Getenv("QSSH_HISTORY_PATH"); p != "" {
		return p
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "qssh", "history.jsonl")
}

// AppendHistory writes a single entry to the JSONL history file.
func AppendHistory(entry *HistoryEntry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	if entry.DurationMs == 0 && entry.Duration != "" {
		// Parse from string
		if d, err := time.ParseDuration(entry.Duration); err == nil {
			entry.DurationMs = d.Milliseconds()
		}
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal history: %w", err)
	}

	path := HistoryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("mkdir history: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open history: %w", err)
	}
	defer f.Close()

	_, err = f.Write(append(data, '\n'))
	return err
}

// ReadHistory reads all entries from the history file.
func ReadHistory() ([]HistoryEntry, error) {
	path := HistoryPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read history: %w", err)
	}

	entries := make([]HistoryEntry, 0)
	for _, line := range splitLines(string(data)) {
		if line == "" {
			continue
		}
		var entry HistoryEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // skip corrupt lines
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}