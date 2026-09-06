package utils

import "testing"

func TestWrapWords(t *testing.T) {
	// wrapWords enforces a minimum width of 16 for panel readability.
	lines := wrapWords("alpha bravo charlie delta echo foxtrot", 16)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped lines, got %#v", lines)
	}
	for _, line := range lines {
		if visibleWidth(line) > 16 {
			t.Fatalf("line too wide: %q", line)
		}
	}
}

func TestVisibleWidthStripsANSI(t *testing.T) {
	colored := ansiBold + "abc" + ansiReset
	if visibleWidth(colored) != 3 {
		t.Fatalf("width=%d", visibleWidth(colored))
	}
}
