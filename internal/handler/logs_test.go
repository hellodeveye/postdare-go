package handler

import (
	"strings"
	"testing"
)

func TestSanitizeLogTextStripsANSIEscapeSequences(t *testing.T) {
	raw := "\x1b[2m22:20:54.747\x1b[0m \x1b[32mINFO\x1b[0m \x1b[1mxianhu-chaos listening\x1b[0m addr=\x1b[36m:18080\x1b[0m\n中文日志"

	got := sanitizeLogText(raw)
	if strings.Contains(got, "\x1b") {
		t.Fatalf("expected ANSI escapes to be stripped, got %q", got)
	}
	if !strings.Contains(got, "INFO xianhu-chaos listening addr=:18080") {
		t.Fatalf("expected readable log content to remain, got %q", got)
	}
	if !strings.Contains(got, "中文日志") {
		t.Fatalf("expected non-ASCII log content to remain, got %q", got)
	}
}
