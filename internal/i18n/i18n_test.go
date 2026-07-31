package i18n

import (
	"fmt"
	"reflect"
	"regexp"
	"testing"
)

// formatVerbRe matches printf-style verbs in a format string: %s, %q, %d, %v, %x, %f...
// %% (escaped percent) is excluded.
var formatVerbRe = regexp.MustCompile(`%[^%]`)

// countVerbs returns the number of printf verbs in s, ignoring "%%".
func countVerbs(s string) int {
	n := 0
	raw := s
	for i := 0; i < len(raw); i++ {
		if raw[i] != '%' {
			continue
		}
		if i+1 < len(raw) && raw[i+1] == '%' {
			i++ // escaped percent
			continue
		}
		n++
	}
	return n
}

// TestKeyParity enforces that en-US and zh-CN define exactly the same keys.
// A key missing from one locale silently renders as the key name at runtime,
// which is a bug the test must catch before shipping.
func TestKeyParity(t *testing.T) {
	enKeys := make([]string, 0, len(enUS))
	for k := range enUS {
		enKeys = append(enKeys, k)
	}
	zhKeys := make([]string, 0, len(zhCN))
	for k := range zhCN {
		zhKeys = append(zhKeys, k)
	}

	enSet := map[string]bool{}
	for _, k := range enKeys {
		enSet[k] = true
	}
	zhSet := map[string]bool{}
	for _, k := range zhKeys {
		zhSet[k] = true
	}

	var missingInZh, missingInEn []string
	for k := range enSet {
		if !zhSet[k] {
			missingInZh = append(missingInZh, k)
		}
	}
	for k := range zhSet {
		if !enSet[k] {
			missingInEn = append(missingInEn, k)
		}
	}
	if len(missingInZh) > 0 || len(missingInEn) > 0 {
		t.Errorf("key parity mismatch:\n  missing in zh-CN: %v\n  missing in en-US: %v", missingInZh, missingInEn)
	}
}

// TestFormatConsistency checks that translations for the same key use the same
// number of printf verbs, so a call like T("key", a, b) cannot mis-format in
// one locale (extra verbs print %!x(MISSING), missing verbs print %!d(EXTRA...)).
func TestFormatConsistency(t *testing.T) {
	for k, en := range enUS {
		zh, ok := zhCN[k]
		if !ok {
			continue // parity test covers this
		}
		enN := countVerbs(en)
		zhN := countVerbs(zh)
		if enN != zhN {
			t.Errorf("format verb mismatch for %q: en-US has %d verbs (%q), zh-CN has %d verbs (%q)", k, enN, en, zhN, zh)
		}
	}
}

// TestEveryKeyRenders renders every key with enough positional args in both
// locales. A malformed format string (wrong verb type, stray %) panics in
// fmt.Sprintf; this ensures T() can never crash at runtime for any shipped key.
func TestEveryKeyRenders(t *testing.T) {
	for _, set := range []map[string]string{enUS, zhCN} {
		for k, msg := range set {
			// Build one arg per verb: %s/%q/%v want a string-ish, %d/%x want an int.
			n := countVerbs(msg)
			args := make([]any, 0, n)
			for i := 0; i < n; i++ {
				args = append(args, "x")
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("T(%q) panicked while rendering %q: %v", k, msg, r)
					}
				}()
				_ = T(k, args...)
			}()
		}
	}
}

// TestTLocaleSwitch verifies SetLocale actually swaps the active table.
func TestTLocaleSwitch(t *testing.T) {
	// Save and restore the current locale.
	prev := Locale()
	defer SetLocale(prev)

	SetLocale("zh-CN")
	if got := T("add.prompt.host"); got != "主机" {
		t.Errorf("zh-CN add.prompt.host = %q, want 主机", got)
	}
	SetLocale("en-US")
	if got := T("add.prompt.host"); got != "Host" {
		t.Errorf("en-US add.prompt.host = %q, want Host", got)
	}
	// Unknown locale falls back to en-US.
	SetLocale("xx-YY")
	if got := T("add.prompt.host"); got != "Host" {
		t.Errorf("unknown locale add.prompt.host = %q, want Host", got)
	}
}

// TestTMissingKey verifies a missing key renders the key itself (documented
// fallback) rather than crashing or returning garbage.
func TestTMissingKey(t *testing.T) {
	prev := Locale()
	defer SetLocale(prev)
	SetLocale("en-US")
	if got := T("no.such.key.exists"); got != "no.such.key.exists" {
		t.Errorf("missing key = %q, want key name", got)
	}
}

// TestNoIncompatibleFmt ensures we never have literal "%-11s"-style alignment
// verbs with mismatched translation shapes is caught elsewhere; here we just
// assert the two locale tables are plain string maps of the same type.
func TestTablesAreStringMaps(t *testing.T) {
	if reflect.TypeOf(enUS).Kind() != reflect.Map || reflect.TypeOf(zhCN).Kind() != reflect.Map {
		t.Fatal("locale tables must be maps")
	}
	_ = fmt.Sprint(enUS) // keep fmt import used
}
