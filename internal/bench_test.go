package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkHistoryAppend measures the cost of appending one entry to a JSONL
// history file of a given size (within and near the trim cap).
func BenchmarkHistoryAppend(b *testing.B) {
	for _, size := range []int{1000, 100000, 1000000} {
		b.Run(fmt.Sprintf("file_%dB", size), func(b *testing.B) {
			dir := b.TempDir()
			path := filepath.Join(dir, "history.jsonl")
			b.Setenv("QSSH_HISTORY_PATH", path)
			// Pre-grow the file to the target size with dummy lines.
			pad := make([]byte, 0, size)
			line := []byte(`{"ts":"2026-01-01T00:00:00Z","profile":"p","command":"echo hi","exit_code":0}` + "\n")
			for len(pad) < size {
				pad = append(pad, line...)
			}
			os.WriteFile(path, pad, 0600)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				AppendHistory(&HistoryEntry{
					Profile:  "bench",
					Command:  "echo hi",
					ExitCode: 0,
				})
			}
		})
	}
}
