package handlers

import "testing"

func TestNormalizeSubmittedIP(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "ipv4", value: "203.0.113.7", want: "203.0.113.7"},
		{name: "x-forwarded-for", value: "198.51.100.9, 172.18.0.2", want: "198.51.100.9"},
		{name: "ipv4 with port", value: "203.0.113.7:45678", want: "203.0.113.7"},
		{name: "bracketed ipv6 with port", value: "[2001:db8::1]:443", want: "2001:db8::1"},
		{name: "invalid", value: "not-an-ip", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeSubmittedIP(tt.value); got != tt.want {
				t.Fatalf("normalizeSubmittedIP(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
