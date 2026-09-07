package server

import (
	"net"
	"testing"
)

func TestIsVirtualAdapterName(t *testing.T) {
	virtual := []string{
		"vEthernet (WSL (Hyper-V firewall))",
		"Ethernet adapter vEthernet (Default Switch)",
		"Tailscale",
		"DockerNAT",
		"VirtualBox Host-Only Network",
	}
	for _, name := range virtual {
		if !isVirtualAdapterName(name) {
			t.Errorf("isVirtualAdapterName(%q) = false, want true", name)
		}
	}

	physical := []string{
		"Wi-Fi",
		"Wireless LAN adapter WiFi",
		"Ethernet",
		"Local Area Connection",
		"Wi-Fi 2",
	}
	for _, name := range physical {
		if isVirtualAdapterName(name) {
			t.Errorf("isVirtualAdapterName(%q) = true, want false", name)
		}
	}
}

func TestAdapterPreference(t *testing.T) {
	if adapterPreference("Wi-Fi") <= adapterPreference("Ethernet") {
		t.Fatalf("wireless should outrank wired")
	}
	if adapterPreference("Ethernet") <= adapterPreference("Other") {
		t.Fatalf("ethernet should outrank unnamed adapters")
	}
}

func TestIsPrivateLANIPv4(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"192.168.10.104", true},
		{"10.0.0.5", true},
		{"172.16.3.1", true},
		{"172.28.80.1", true},
		{"172.32.0.1", false},
		{"100.64.0.1", false},
		{"169.254.83.107", false},
		{"8.8.8.8", false},
	}
	for _, tc := range tests {
		if got := isPrivateLANIPv4(net.ParseIP(tc.ip)); got != tc.want {
			t.Errorf("isPrivateLANIPv4(%q) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestIsTailscaleIPRange(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"100.64.0.1", true},
		{"100.90.80.70", true},
		{"100.127.255.254", true},
		{"192.168.1.10", false},
		{"100.128.0.1", false},
	}
	for _, tc := range tests {
		if got := isTailscaleIP(net.ParseIP(tc.ip)); got != tc.want {
			t.Errorf("isTailscaleIP(%q) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}
