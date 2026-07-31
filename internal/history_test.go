package internal

import (
	"path/filepath"
	"testing"
)

func TestMaskCommand(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`echo "hi"`, "echo"},
		{`docker compose up -d`, "docker"},
		{`vim .env`, "vim"},
		{"sudo -S 'hunter2' deploy.sh", "sudo"},
		{`./deploy.sh --token abc`, "./deploy.sh"},
		{``, ""},
		{`   `, ""},
		{`curl -H "Authorization: Bearer xyz" https://host/api`, "curl"},
	}
	for _, c := range cases {
		if got := MaskCommand(c.in); got != c.want {
			t.Errorf("MaskCommand(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHistoryRecordMode(t *testing.T) {
	// Point DefaultConfigPath at a temp file so we don't touch any real
	// config on any platform (XDG_CONFIG_HOME only works on Linux).
	cfgPath := filepath.Join(t.TempDir(), "qssh", "config.json")
	t.Setenv("QSSH_CONFIG_PATH", cfgPath)

	// 1. No config, no profile opts -> masked.
	if got := HistoryRecordMode(nil); got != RecordMasked {
		t.Errorf("no config: got %q, want masked", got)
	}

	// 2. Profile override wins over global default.
	if err := OpenConfig(cfgPath).Set(historyRecordKey, RecordFull); err != nil {
		t.Fatalf("set global: %v", err)
	}
	if got := HistoryRecordMode(map[string]string{historyRecordKey: RecordOff}); got != RecordOff {
		t.Errorf("profile off: got %q, want off", got)
	}

	// 3. Global default applies when no profile override.
	if got := HistoryRecordMode(nil); got != RecordFull {
		t.Errorf("global full: got %q, want full", got)
	}

	// 4. Invalid profile value fails closed to masked (not global full).
	if got := HistoryRecordMode(map[string]string{historyRecordKey: "everything"}); got != RecordMasked {
		t.Errorf("invalid profile value: got %q, want masked (fail closed)", got)
	}

	// 5. Invalid global value fails closed to masked.
	if err := OpenConfig(cfgPath).Set(historyRecordKey, "all-the-things"); err != nil {
		t.Fatalf("set invalid global: %v", err)
	}
	if got := HistoryRecordMode(nil); got != RecordMasked {
		t.Errorf("invalid global value: got %q, want masked (fail closed)", got)
	}

	// 6. Valid global value round-trips.
	if err := OpenConfig(cfgPath).Set(historyRecordKey, RecordMasked); err != nil {
		t.Fatalf("set masked: %v", err)
	}
	if got := HistoryRecordMode(nil); got != RecordMasked {
		t.Errorf("valid masked: got %q, want masked", got)
	}

	// 7. Empty profile value clears the override and falls through to global.
	if err := OpenConfig(cfgPath).Set(historyRecordKey, RecordFull); err != nil {
		t.Fatalf("set global full: %v", err)
	}
	if got := HistoryRecordMode(map[string]string{historyRecordKey: ""}); got != RecordFull {
		t.Errorf("empty profile value: got %q, want global full (clear semantics)", got)
	}
	if got := HistoryRecordMode(map[string]string{historyRecordKey: " "}); got != RecordFull {
		t.Errorf("whitespace profile value: got %q, want global full", got)
	}
}

func TestHistoryRecordMode_AlwaysValid(t *testing.T) {
	// Ensure the function never panics and always returns one of the modes,
	// even pointing at a real (possibly unreadable) config path.
	mode := HistoryRecordMode(map[string]string{})
	switch mode {
	case RecordFull, RecordMasked, RecordOff:
	default:
		t.Errorf("HistoryRecordMode returned invalid mode %q", mode)
	}
}
