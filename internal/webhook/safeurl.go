package webhook

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ErrBlockedAddress is returned when an endpoint resolves to a non-public IP.
var ErrBlockedAddress = errors.New("webhook endpoint must be a public internet address")

// Extra ranges beyond what netip's Is* helpers already cover.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),     // "this network"
	netip.MustParsePrefix("100.64.0.0/10"), // CGNAT — Tailscale lives here
	netip.MustParsePrefix("192.0.0.0/24"),  // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"), // benchmarking
	netip.MustParsePrefix("240.0.0.0/4"),   // reserved
	netip.MustParsePrefix("64:ff9b::/96"),  // NAT64 can map to private v4
}

// isBlockedIP reports whether outbound webhook traffic to ip must be refused:
// loopback, RFC1918/ULA, link-local (incl. cloud metadata 169.254.169.254),
// CGNAT, multicast and other non-routable space.
func isBlockedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// ValidateURL checks an endpoint before it is saved. It catches obvious
// mistakes early; the dialer re-checks every resolved IP at send time, which
// is what actually stops DNS-rebinding tricks.
func ValidateURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > 2048 {
		return "", fmt.Errorf("url is too long")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("url must be an absolute http(s) URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("url must use http or https")
	}
	if u.User != nil {
		return "", fmt.Errorf("url must not contain credentials")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if ip, err := netip.ParseAddr(host); err == nil {
		if isBlockedIP(ip) {
			return "", ErrBlockedAddress
		}
		return u.String(), nil
	}
	// Single-label names (home, mongo, caddy…) are Docker/LAN hosts.
	if !strings.Contains(host, ".") || host == "localhost" ||
		strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".lan") ||
		strings.HasSuffix(host, ".home.arpa") {
		return "", ErrBlockedAddress
	}
	return u.String(), nil
}

// safeDialControl runs after DNS resolution, on the exact IP being dialed.
func safeDialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || isBlockedIP(ip) {
		return ErrBlockedAddress
	}
	return nil
}

// newSafeClient builds the HTTP client for outbound webhooks: no proxy (an
// env proxy would bypass the dial check), no redirects (a public endpoint
// could bounce us to an internal one), every dialed IP checked.
func newSafeClient(timeout time.Duration, allowPrivate bool) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	if !allowPrivate {
		dialer.Control = safeDialControl
	}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          20,
		IdleConnTimeout:       30 * time.Second,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
