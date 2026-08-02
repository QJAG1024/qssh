package internal

import (
	"fmt"
	"strings"
)

// EffectiveOption resolves a per-profile-overridable setting.
// Precedence: profile Options > global config > "" (caller applies default).
// An empty profile value is treated as unset (falls through to global),
// which lets `--set-option key=` clear a per-profile override.
func EffectiveOption(profileOpts map[string]string, key string) string {
	if profileOpts != nil {
		if v, ok := profileOpts[key]; ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	if cfg := OpenConfig(DefaultConfigPath()); cfg != nil {
		return cfg.Get(key)
	}
	return ""
}

// Per-profile option keys accepted by --set-option / interactive Options edit.
// These are the only keys that may be stored in a profile's Options map.
var perProfileOptionKeys = map[string]string{
	"ConnectTimeout":          "Connection timeout (e.g. 30s)",
	"SetEnv":                  "Environment variables, comma-separated KEY=VALUE",
	"term.mode":               "PTY TERM: passthrough|compat (per-profile override of global)",
	"hostkey.mode":            "Host key policy: tofu|strict (per-profile override of global)",
	"history.record_commands": "History recording: full|masked|off (per-profile override of global)",
	"sftp.bind":               "SFTP proxy bind address (per-profile override of global)",
	"webdav.bind":             "WebDAV bind address (per-profile override of global)",
	"webdav.token_mode":       "WebDAV token mode: auto|always (per-profile override of global)",
}

// globalOnlyOptionKeys are config keys that cannot be set per-profile.
// They exist globally; setting them via --set-option is a user error.
var globalOnlyOptionKeys = map[string]string{
	"sftp.allow_non_loopback": "sftp.allow_non_loopback is global-only: to bind a profile's SFTP proxy to a non-loopback address, set sftp.bind on the profile instead",
}

// ValidateOptionKeys checks --set-option keys against the per-profile
// whitelist. Unknown keys are rejected (no silent no-op config) so a typo
// like --set-option termmode=compat cannot appear to work. Returns a list of
// errors (one per invalid key); callers print them and exit.
func ValidateOptionKeys(keys []string) []string {
	var errs []string
	for _, k := range keys {
		if _, ok := perProfileOptionKeys[k]; ok {
			continue
		}
		if hint, ok := globalOnlyOptionKeys[k]; ok {
			errs = append(errs, hint)
			continue
		}
		errs = append(errs, fmt.Sprintf("unsupported per-profile option %q (supported: ConnectTimeout, SetEnv, term.mode, hostkey.mode, history.record_commands, sftp.bind, webdav.bind, webdav.token_mode)", k))
	}
	return errs
}

// ApplyOptionMap validates and merges a parsed --set-option map into a
// profile's Options. Returns an error listing every invalid key; on error the
// destination is left untouched so a partially-invalid request cannot apply
// half of its options.
func ApplyOptionMap(dst map[string]string, parsed map[string]string) (map[string]string, error) {
	keys := make([]string, 0, len(parsed))
	for k := range parsed {
		keys = append(keys, k)
	}
	if errs := ValidateOptionKeys(keys); len(errs) > 0 {
		return dst, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return MergeOptions(dst, parsed), nil
}

// MergeOptions applies parsed --set-option pairs onto a profile's Options map.
// SetEnv gets special merge semantics so adding one variable does not wipe the
// others: KEY=VALUE adds/updates a single variable, KEY= (empty) removes it.
// All other keys replace the existing value wholesale.
func MergeOptions(dst map[string]string, parsed map[string]string) map[string]string {
	if dst == nil {
		dst = make(map[string]string, len(parsed))
	}
	for k, v := range parsed {
		if k == "SetEnv" {
			dst[k] = mergeSetEnv(dst[k], v)
			if dst[k] == "" {
				delete(dst, k)
			}
			continue
		}
		dst[k] = v
	}
	return dst
}

// mergeSetEnv merges a SetEnv update string into an existing SetEnv string.
// Both are comma-separated KEY=VALUE lists. New pairs override same-key
// existing pairs; an empty value (KEY=) removes the variable.
func mergeSetEnv(existing, update string) string {
	env := parseEnvPairs(existing)
	for _, pair := range strings.Split(update, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue // malformed pair: skip
		}
		key := strings.TrimSpace(kv[0])
		if key == "" {
			continue
		}
		val := strings.TrimSpace(kv[1])
		if val == "" {
			delete(env, key) // empty value removes the variable
		} else {
			env[key] = val
		}
	}
	if len(env) == 0 {
		return ""
	}
	// Rebuild in deterministic (sorted) order.
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sortStrings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+env[k])
	}
	return strings.Join(parts, ",")
}

func parseEnvPairs(s string) map[string]string {
	m := map[string]string{}
	if s == "" {
		return m
	}
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		if key != "" {
			m[key] = strings.TrimSpace(kv[1])
		}
	}
	return m
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
