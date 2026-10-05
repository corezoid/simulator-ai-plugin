package ecore

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"
)

func TestIsPublicAddr(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8":                true,
		"1.1.1.1":                true,
		"2606:4700::1111":        true,
		"127.0.0.1":              false,
		"10.1.2.3":               false,
		"172.16.0.1":             false,
		"192.168.1.1":            false,
		"169.254.169.254":        false, // cloud metadata
		"100.64.0.1":             false, // CGNAT
		"0.0.0.0":                false,
		"0.1.2.3":                false,
		"224.0.0.1":              false,
		"255.255.255.255":        false,
		"198.18.0.1":             false,
		"::1":                    false,
		"::":                     false,
		"fc00::1":                false, // unique local
		"fe80::1":                false,
		"fd00:ec2::254":          false, // EC2 IPv6 metadata
		"::ffff:127.0.0.1":       false, // IPv4-mapped loopback
		"::ffff:169.254.169.254": false,
		"64:ff9b::a9fe:a9fe":     false, // NAT64 of 169.254.169.254
		"2002:a9fe:a9fe::":       false, // 6to4 of 169.254.169.254
		"2001::1":                false, // Teredo
	}
	for s, want := range cases {
		if got := isPublicAddr(netip.MustParseAddr(s)); got != want {
			t.Errorf("isPublicAddr(%s) = %v, want %v", s, got, want)
		}
	}
}

func withStateless(t *testing.T, on bool) {
	t.Helper()
	prev := IsStateless()
	SetStateless(on)
	t.Cleanup(func() { SetStateless(prev) })
}

func TestCheckUserURL(t *testing.T) {
	withStateless(t, true)
	for _, bad := range []string{
		"http://example.com/a.png",
		"ftp://example.com/a.png",
		"file:///etc/passwd",
		"https://127.0.0.1/a.png",
		"https://169.254.169.254/latest/meta-data/",
		"https://[::1]/a.png",
		"https://user:pass@example.com/a.png",
		"https:///nohost",
		"gopher://example.com",
	} {
		if err := CheckUserURL(bad); err == nil {
			t.Errorf("CheckUserURL(%q) accepted", bad)
		}
	}
	if err := CheckUserURL("https://example.com/a.png"); err != nil {
		t.Errorf("public https URL refused: %v", err)
	}

	withStateless(t, false)
	if err := CheckUserURL("http://localhost/a.png"); err != nil {
		t.Errorf("local (stdio) mode must not restrict URLs: %v", err)
	}
}

type fakeResolver map[string][]string

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	var out []netip.Addr
	for _, s := range f[host] {
		out = append(out, netip.MustParseAddr(s))
	}
	return out, nil
}

// The dialer is where the real guarantee lives: it checks the addresses it is
// about to connect to, so names that resolve inward are refused even though
// the URL itself looked harmless.
func TestPublicOnlyDialRefusesInternalAnswers(t *testing.T) {
	r := fakeResolver{
		"inward.example":   {"10.0.0.5"},
		"metadata.example": {"169.254.169.254"},
		"mixed.example":    {"93.184.216.34", "127.0.0.1"},
		"mapped.example":   {"::ffff:10.0.0.1"},
	}
	dial := publicOnlyDial(&net.Dialer{}, r)
	for _, host := range []string{"inward.example", "metadata.example", "mixed.example", "mapped.example", "127.0.0.1", "[::1]"} {
		conn, err := dial(context.Background(), "tcp", net.JoinHostPort(strings.Trim(host, "[]"), "443"))
		if err == nil {
			conn.Close()
			t.Errorf("dial %s succeeded; want refusal", host)
		} else if !strings.Contains(err.Error(), "non-public") {
			t.Errorf("dial %s: unexpected error %v", host, err)
		}
	}
}

func TestUserURLClientNeverReachesLoopback(t *testing.T) {
	withStateless(t, true)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	hit := make(chan struct{}, 1)
	go func() {
		if c, err := ln.Accept(); err == nil {
			hit <- struct{}{}
			c.Close()
		}
	}()

	_, err = UserURLClient().Get("https://" + ln.Addr().String() + "/x")
	if err == nil {
		t.Fatal("request to loopback succeeded")
	}
	select {
	case <-hit:
		t.Fatal("the internal listener received a connection")
	default:
	}
}
