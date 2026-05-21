package sizeparse

import "testing"

func TestParseByteSize(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"16B", 16},
		{"1KB", 1000},
		{"1KiB", 1024},
		{"2MiB", 2 << 20},
		{"42", 42},
	}
	for _, tt := range tests {
		got, err := ParseByteSize(tt.in)
		if err != nil {
			t.Fatalf("ParseByteSize(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("ParseByteSize(%q)=%d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestParseByteSizeRejectsInvalidInput(t *testing.T) {
	if _, err := ParseByteSize("64XB"); err == nil {
		t.Fatal("expected invalid suffix error")
	}
}
