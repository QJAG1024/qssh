package cmd

import (
	"bytes"
	"strings"
	"testing"

	"qssh/internal/i18n"
)

func TestRenderUsage_ContainsCommands(t *testing.T) {
	i18n.SetLocale("en-US")
	defer i18n.SetLocale("en-US")

	var buf bytes.Buffer
	RenderUsage(&buf, "test-ver")

	out := buf.String()
	// Banner with injected version.
	if !strings.Contains(out, "QSSH - SSH Credential Manager vtest-ver") {
		t.Errorf("banner missing version: %q", out[:60])
	}
	// Every top-level command must appear.
	for _, g := range usageGroups {
		for _, c := range g.cmds {
			if !strings.Contains(out, c.usage) {
				t.Errorf("usage output missing command %q", c.usage)
			}
		}
	}
	// Sub-parameter hints present.
	if !strings.Contains(out, "options: host port") {
		t.Errorf("sub-parameter hint missing for --add")
	}
	if !strings.Contains(out, "options: bind port") {
		t.Errorf("sub-parameter hint missing for --sftp-start")
	}
	// Group titles present.
	if !strings.Contains(out, "Create & modify profiles") {
		t.Errorf("group title missing")
	}
}

func TestRenderUsage_AllDescKeysExist(t *testing.T) {
	// Every desc/group key referenced by usageGroups must exist in the locale
	// tables — a missing key would render as the raw key name. This guards
	// against adding a command without adding its translation.
	i18n.SetLocale("en-US")
	defer i18n.SetLocale("en-US")

	for _, g := range usageGroups {
		// Rendering would silently show the key name if missing; check via T.
		if got := i18n.T(g.name); got == g.name && g.name != "" {
			t.Errorf("group key %q missing from locale table", g.name)
		}
		for _, c := range g.cmds {
			if got := i18n.T(c.desc); got == c.desc {
				t.Errorf("desc key %q missing from locale table", c.desc)
			}
		}
	}
}
