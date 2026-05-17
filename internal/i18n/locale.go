// Package i18n provides lightweight translation for user-facing strings.
package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

var (
	mu       sync.RWMutex
	messages map[string]string
)

func init() {
	detect()
}

// detect reads LANG from the environment and loads the matching locale.
func detect() {
	raw := os.Getenv("LANG")
	if raw == "" {
		raw = "en-US"
	}
	SetLocale(parseLang(raw))
}

// parseLang normalises a locale string like "zh_CN.UTF-8" to "zh-CN".
func parseLang(raw string) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.IndexByte(raw, '.'); idx >= 0 {
		raw = raw[:idx]
	}
	return strings.Replace(raw, "_", "-", 1)
}

// SetLocale overrides the active locale (e.g. "zh-CN", "en-US").
// Unknown locales fall back to en-US.
func SetLocale(lang string) {
	mu.Lock()
	defer mu.Unlock()

	switch lang {
	case "zh-CN":
		messages = zhCN
	default:
		// Normalise "en-US", "en_US", "C", "POSIX", etc. to en-US.
		messages = enUS
	}
}

// Locale returns the currently active locale code.
func Locale() string {
	mu.RLock()
	defer mu.RUnlock()
	// Best-effort: check a few known keys to identify locale.
	if _, ok := messages["locale.code"]; ok {
		return messages["locale.code"]
	}
	return "en-US"
}

// T looks up key in the active locale and formats it with optional args.
// If the key is missing, the key itself is returned as a fallback.
func T(key string, args ...interface{}) string {
	mu.RLock()
	msg := messages[key]
	mu.RUnlock()

	if msg == "" {
		return key
	}
	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}