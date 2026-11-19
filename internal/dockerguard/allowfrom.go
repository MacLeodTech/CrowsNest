package dockerguard

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// ParseAllowFrom reads a comma-separated list of CIDRs (or single addresses) for AllowFrom. An empty string is no
// restriction.
func ParseAllowFrom(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if p, err := netip.ParsePrefix(part); err == nil {
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(part)
		if err != nil {
			return nil, fmt.Errorf("%q is not a CIDR or an address", part)
		}
		out = append(out, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
	}
	return out, nil
}

// AllowFrom wraps l so that only clients whose address is inside one of allow are served; the rest are closed at
// once. The guard has no sign-in of its own, so this is what keeps another container on the same network from
// using its power. With no prefixes l is returned as it is. logf, if set, hears each refused address.
func AllowFrom(l net.Listener, allow []netip.Prefix, logf func(format string, args ...any)) net.Listener {
	if len(allow) == 0 {
		return l
	}
	return &allowListener{Listener: l, allow: allow, logf: logf}
}

type allowListener struct {
	net.Listener
	allow []netip.Prefix
	logf  func(format string, args ...any)
}

func (a *allowListener) Accept() (net.Conn, error) {
	for {
		c, err := a.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if a.permitted(c.RemoteAddr()) {
			return c, nil
		}
		if a.logf != nil {
			a.logf("DENY - -: client %s is not in GUARD_ALLOW_FROM", c.RemoteAddr())
		}
		_ = c.Close()
	}
}

func (a *allowListener) permitted(addr net.Addr) bool {
	var ap netip.AddrPort
	switch t := addr.(type) {
	case *net.TCPAddr:
		ap = t.AddrPort()
	default:
		var err error
		if ap, err = netip.ParseAddrPort(addr.String()); err != nil {
			return false
		}
	}
	ip := ap.Addr().Unmap()
	for _, p := range a.allow {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}
