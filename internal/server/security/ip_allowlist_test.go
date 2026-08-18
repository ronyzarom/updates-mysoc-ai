package security

import (
	"net"
	"testing"
)

func TestNormalizeCIDR(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "bare ipv4 host", input: "203.0.113.7", want: "203.0.113.7"},
		{name: "bare ipv6 host", input: "::1", want: "::1"},
		{name: "ipv4 cidr canonicalized", input: "10.0.0.5/8", want: "10.0.0.0/8"},
		{name: "ipv4 cidr exact", input: "192.168.1.0/24", want: "192.168.1.0/24"},
		{name: "ipv6 cidr", input: "2001:db8::1/32", want: "2001:db8::/32"},
		{name: "trims whitespace", input: "  127.0.0.1  ", want: "127.0.0.1"},
		{name: "empty", input: "", wantErr: true},
		{name: "garbage", input: "not-an-ip", wantErr: true},
		{name: "bad cidr", input: "10.0.0.0/99", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeCIDR(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %q", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizeCIDR(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestMatches(t *testing.T) {
	tests := []struct {
		name   string
		target string
		ip     string
		want   bool
	}{
		{name: "host exact ipv4", target: "203.0.113.7", ip: "203.0.113.7", want: true},
		{name: "host mismatch ipv4", target: "203.0.113.7", ip: "203.0.113.8", want: false},
		{name: "cidr contains", target: "10.0.0.0/8", ip: "10.11.12.13", want: true},
		{name: "cidr excludes", target: "10.0.0.0/8", ip: "11.0.0.1", want: false},
		{name: "loopback host", target: "127.0.0.1", ip: "127.0.0.1", want: true},
		{name: "loopback range", target: "127.0.0.0/8", ip: "127.0.0.5", want: true},
		{name: "ipv6 host exact", target: "::1", ip: "::1", want: true},
		{name: "ipv6 cidr contains", target: "2001:db8::/32", ip: "2001:db8::5", want: true},
		{name: "ipv6 cidr excludes", target: "2001:db8::/32", ip: "2001:dead::5", want: false},
		{name: "invalid target", target: "garbage", ip: "10.0.0.1", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matches(tc.target, net.ParseIP(tc.ip))
			if got != tc.want {
				t.Fatalf("matches(%q, %q) = %v, want %v", tc.target, tc.ip, got, tc.want)
			}
		})
	}
}

func TestMatchesNilIP(t *testing.T) {
	if matches("10.0.0.0/8", nil) {
		t.Fatal("nil IP must never match")
	}
}
