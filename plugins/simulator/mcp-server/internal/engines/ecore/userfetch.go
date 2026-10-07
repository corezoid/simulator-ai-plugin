package ecore

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
)

// UserURLClient returns the HTTP client for URLs that come from tool arguments
// (imageUrl, fileUrl) rather than from the configured API base.
//
// In stateless (hosted) mode the server runs inside our network on behalf of
// remote callers, so such a URL must never reach anything but the public
// internet: the dialer resolves the host itself, refuses every non-public
// address (loopback, private, link-local — which includes cloud metadata —
// CGNAT, multicast, …) and connects to the vetted IP, so a DNS answer cannot
// change between the check and the connection. Redirects are re-dialed through
// the same check, and proxy environment variables are ignored.
//
// In local (stdio) mode the server acts for the user on their own machine, and
// the plain API client is returned unchanged.
func UserURLClient() *http.Client {
	if !IsStateless() {
		return APIHTTPClient()
	}
	userClientOnce.Do(func() {
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		tr := &http.Transport{
			Proxy:                 nil,
			DialContext:           publicOnlyDial(dialer, net.DefaultResolver),
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConns:          10,
			IdleConnTimeout:       30 * time.Second,
		}
		userClient = &http.Client{
			Timeout:   60 * time.Second,
			Transport: tr,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return errors.New("too many redirects")
				}
				return CheckUserURL(req.URL.String())
			},
		}
	})
	return userClient
}

var (
	userClientOnce sync.Once
	userClient     *http.Client
)

// CheckUserURL validates a tool-supplied URL before any request is made. In
// stateless mode only https with a host name or public IP is accepted; the
// dialer still checks every resolved address.
func CheckUserURL(raw string) error {
	if !IsStateless() {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" {
		return errors.New("only https URLs are accepted by the hosted server")
	}
	if u.User != nil {
		return errors.New("URLs with credentials are not accepted")
	}
	if u.Hostname() == "" {
		return errors.New("URL has no host")
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !isPublicAddr(ip) {
		return errors.New("URL points to a non-public address")
	}
	return nil
}

type resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// publicOnlyDial resolves addr, rejects it unless every address is public, and
// dials the first vetted address directly.
func publicOnlyDial(d *net.Dialer, r resolver) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		var ips []netip.Addr
		if ip, perr := netip.ParseAddr(host); perr == nil {
			ips = []netip.Addr{ip}
		} else {
			ips, err = r.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no addresses for %s", host)
		}
		// Every answer must be public: a mixed answer is how rebinding tricks
		// try to sneak an internal address past a first-match check.
		for _, ip := range ips {
			if !isPublicAddr(ip) {
				return nil, fmt.Errorf("refusing to connect to non-public address for %s", host)
			}
		}
		return d.DialContext(ctx, network, net.JoinHostPort(ips[0].Unmap().String(), port))
	}
}

// nonPublicPrefixes are special-purpose ranges beyond what the netip
// predicates cover (IANA IPv4/IPv6 special-purpose registries).
var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),   // CGNAT / shared address space
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved, incl. broadcast
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64 — can embed internal IPv4
	netip.MustParsePrefix("64:ff9b:1::/48"),  // local-use NAT64
	netip.MustParsePrefix("2001::/32"),       // Teredo — can embed internal IPv4
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
	netip.MustParsePrefix("2002::/16"),       // 6to4 — can embed internal IPv4
	netip.MustParsePrefix("fec0::/10"),       // deprecated site-local
}

func isPublicAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() {
		return false
	}
	for _, p := range nonPublicPrefixes {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}
