package cli

import "testing"

func TestIsNonLoopbackListenHost_Table(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"127.0.0.1", false}, {"::1", false}, {"localhost", false},
		{"0.0.0.0", true}, {"::", true}, {"192.168.1.10", true}, {"example.internal", true},
	}
	for _, tt := range tests {
		if got := isNonLoopbackListenHost(tt.host); got != tt.want {
			t.Errorf("%q: got %v want %v", tt.host, got, tt.want)
		}
	}
}

func TestAdminRemoteWarningDecision(t *testing.T) {
	for _, tt := range []struct{ nonLoopback, enabled, allow, want bool }{
		{false, true, true, false}, {true, false, true, false}, {true, true, false, false}, {true, true, true, true},
	} {
		got := tt.nonLoopback && tt.enabled && tt.allow
		if got != tt.want {
			t.Errorf("decision=%v want %v", got, tt.want)
		}
	}
}
