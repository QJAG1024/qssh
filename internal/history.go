package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// defaultHistoryMaxSize is the default file-size cap (5 MB).
	defaultHistoryMaxSize = 5 * 1024 * 1024

	// configKeyHistoryMaxSize is the config key for overriding the cap.
	configKeyHistoryMaxSize = "history.max_size"
)

// HistoryEntry represents a single connection record.
type HistoryEntry struct {
	Timestamp  time.Time `json:"ts"`
	Profile    string    `json:"profile"`
	Duration   string    `json:"duration,omitempty"` // human-readable, e.g. "42s"
	DurationMs int64     `json:"duration_ms"`
	Command    string    `json:"command,omitempty"` // empty for interactive shell
	ExitCode   int       `json:"exit_code"`
}

// HistoryPath returns the path to the history file.
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

// historyMaxSize reads the configured size limit from config.
// Returns defaultHistoryMaxSize when not set or unparseable.
func historyMaxSize() int64 {
	cfg := OpenConfig(DefaultConfigPath())
	if cfg == nil {
		return defaultHistoryMaxSize
	}
	v := strings.TrimSpace(cfg.Get(configKeyHistoryMaxSize))
	if v == "" {
		return defaultHistoryMaxSize
	}
	return parseSize(v)
}

// parseSize parses a human-readable size string like "5M", "10M", "1G", "1048576".
func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultHistoryMaxSize
	}
	// Parse suffix.
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "G") || strings.HasSuffix(s, "g"):
		mult = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	case strings.HasSuffix(s, "M") || strings.HasSuffix(s, "m"):
		mult = 1024 * 1024
		s = s[:len(s)-1]
	case strings.HasSuffix(s, "K") || strings.HasSuffix(s, "k"):
		mult = 1024
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n <= 0 {
		return defaultHistoryMaxSize
	}
	return n * mult
}

// AppendHistory writes a single entry to the JSONL history file.
// When the file exceeds the configured history.max_size (default 5 MB),
// the oldest entries are trimmed.
func AppendHistory(entry *HistoryEntry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	if entry.DurationMs == 0 && entry.Duration != "" {
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

	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}

	// Best-effort trim: if the file exceeds the configured size cap,
	// drop the oldest entries until it fits.
	_ = trimHistoryBySize(path, historyMaxSize())
	return nil
}

// trimHistoryBySize rewrites the JSONL file, keeping only the tail-most
// entries that fit under maxBytes.
func trimHistoryBySize(path string, maxBytes int64) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if int64(len(raw)) <= maxBytes {
		return nil
	}

	lines := splitLines(string(raw))
	// Drop from the head until the remainder fits.
	keep := len(lines)
	for keep > 0 {
		total := 0
		for _, l := range lines[len(lines)-keep:] {
			total += len(l) + 1 // +1 for newline
		}
		if int64(total) <= maxBytes {
			break
		}
		keep--
	}
	if keep == 0 {
		// Even a single entry exceeds the cap — keep at least the last one.
		keep = 1
	}

	trimmed := lines[len(lines)-keep:]
	out := make([]byte, 0, 1024*keep)
	for _, l := range trimmed {
		out = append(out, l...)
		out = append(out, '\n')
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".history-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
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
