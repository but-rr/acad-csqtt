package client

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

// ResolverDNS selects the DNS passed to the existing proxy/TUN helper.
// Server configuration remains separate so repeated CONFIG events compare
// against the original server values rather than the user override.
func (p Profile) ResolverDNS(serverDNS string) (string, error) {
	switch p.DNSMode {
	case "", "server":
		return serverDNS, nil
	case "google":
		return "8.8.8.8,8.8.4.4", nil
	case "cloudflare":
		return "1.1.1.1,1.0.0.1", nil
	case "custom":
		primary, e := dnsAddress(p.DNSPrimary, "Основной DNS")
		if e != nil {
			return "", e
		}
		if strings.TrimSpace(p.DNSSecondary) == "" {
			return primary, nil
		}
		secondary, e := dnsAddress(p.DNSSecondary, "Резервный DNS")
		if e != nil {
			return "", e
		}
		if primary == secondary {
			return primary, nil
		}
		return primary + "," + secondary, nil
	default:
		return "", errors.New("Выберите режим DNS в настройках.")
	}
}

func dnsAddress(value, label string) (string, error) {
	ip, e := netip.ParseAddr(strings.TrimSpace(value))
	if e != nil || !ip.Is4() || !ip.IsGlobalUnicast() {
		return "", fmt.Errorf("%s: укажите IPv4-адрес, например 8.8.8.8, без порта и схемы.", label)
	}
	return ip.String(), nil
}
