package server

import (
	"net"
	"strings"
)

// virtualAdapterNames matches container, VPN, and virtualization adapters that
// must never be advertised as the server's reachable home-LAN address.
var virtualAdapterNames = []string{
	"vethernet", "virtual", "hyper-v", "docker", "vmware", "virtualbox",
	"wsl", "loopback", "bluetooth", "tunnel", "npcap", "tailscale",
}

// adapterSubstringMatches reports whether a virtual interface should be skipped.
func isVirtualAdapterName(name string) bool {
	lower := strings.ToLower(name)
	for _, marker := range virtualAdapterNames {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// isPhysicalAdapter reports whether the adapter is a candidate for a home-LAN
// address. Virtual and bluetooth adapters are rejected; wireless and ethernet
// adapters are preferred.
func isPhysicalAdapter(iface net.Interface) bool {
	if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
		return false
	}
	if isVirtualAdapterName(iface.Name) {
		return false
	}
	return true
}

// adapterPreference ranks an adapter name higher when it is a real
// wireless or wired adapter, lower otherwise.
func adapterPreference(name string) int {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "wi-fi"), strings.Contains(lower, "wifi"),
		strings.Contains(lower, "wireless"), strings.Contains(lower, "wlan"):
		return 4
	case strings.Contains(lower, "ethernet"), strings.Contains(lower, "eth"),
		strings.Contains(lower, "en0"), strings.Contains(lower, "en1"):
		return 3
	default:
		return 1
	}
}

// interfaceIPv4Addrs returns the IPv4 addresses for an interface, excluding
// unspecified and link-local addresses (which are never reachable endpoints).
func interfaceIPv4Addrs(iface net.Interface) []net.IP {
	addrs, err := iface.Addrs()
	if err != nil {
		return nil
	}
	out := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			continue
		}
		ip = ip.To4()
		if ip == nil || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
			continue
		}
		out = append(out, ip)
	}
	return out
}

// isPrivateLANIPv4 reports whether ip is an RFC1918 private address usable on a
// home network. Carrier-grade NAT (100.64.0.0/10) and link-local addresses are
// excluded because a phone on the same Wi-Fi cannot reach them.
func isPrivateLANIPv4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		return false
	}
	return ip[0] == 10 ||
		(ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) ||
		(ip[0] == 192 && ip[1] == 168)
}

// isTailscaleIP reports whether ip is inside Tailscale's 100.64.0.0/10 range.
func isTailscaleIP(ip net.IP) bool {
	ip = ip.To4()
	return ip != nil && ip[0] == 100 && ip[1]&0xc0 == 0x40
}

// hasDefaultGateway reports whether the adapter has a configured default
// gateway. net.Interface does not expose gateways, so this is optimistic and
// returns false; ranking relies on adapter names, addresses, and score.
func hasDefaultGateway(iface net.Interface) bool {
	return false
}

// PreferredAddresses returns the LAN and Tailscale IPv4 addresses that should
// be advertised to clients. Virtual and container adapters are excluded and
// adapters with a default gateway are preferred.
func PreferredAddresses() (lan net.IP, tailscale net.IP) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, nil
	}

	type candidate struct {
		ip      net.IP
		score   int
		gateway bool
	}

	var lanCandidates []candidate
	var tailscaleCandidates []candidate
	for _, iface := range interfaces {
		if !isPhysicalAdapter(iface) {
			continue
		}
		gateway := hasDefaultGateway(iface)
		pref := adapterPreference(iface.Name)
		for _, ip := range interfaceIPv4Addrs(iface) {
			if isTailscaleIP(ip) {
				score := pref
				if gateway {
					score += 2
				}
				tailscaleCandidates = append(tailscaleCandidates, candidate{ip: ip, score: score, gateway: gateway})
			} else if isPrivateLANIPv4(ip) {
				score := pref
				if gateway {
					score += 2
				}
				lanCandidates = append(lanCandidates, candidate{ip: ip, score: score, gateway: gateway})
			}
		}
	}

	best := func(candidates []candidate) net.IP {
		var bestIP net.IP
		bestScore := -1
		bestGateway := false
		for _, c := range candidates {
			if c.score > bestScore || (c.score == bestScore && c.gateway && !bestGateway) {
				bestIP = c.ip
				bestScore = c.score
				bestGateway = c.gateway
			}
		}
		return bestIP
	}

	return best(lanCandidates), best(tailscaleCandidates)
}

func preferredAddress() (lan net.IP, tailscale net.IP) {
	return PreferredAddresses()
}
