// Package webhook implements outbound webhook delivery: SSRF-safe URL
// validation, HMAC-SHA256 request signing, and the delivery worker that
// polls published outbox events, fans them out to registered endpoints,
// and tracks per-endpoint delivery state (pending → delivered / failed /
// dead_letter).
package webhook

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// ErrBlockedURL is returned by ValidateURL when the URL points at a
// private, loopback, link-local, or cloud-metadata endpoint.
var ErrBlockedURL = errors.New("URL is blocked by SSRF protection")

// blockedPrefixes are the CIDR ranges that webhook delivery must never
// reach: private networks (RFC 1918), loopback, link-local, the AWS/GCP
// cloud metadata endpoint, and the IPv6 equivalents.
var blockedPrefixes = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"127.0.0.0/8",
	"169.254.0.0/16", // link-local + AWS/GCP metadata (169.254.169.254)
	"0.0.0.0/8",
	"100.64.0.0/10",  // CGNAT
	"::1/128",        // IPv6 loopback
	"fc00::/7",       // IPv6 ULA (private)
	"fe80::/10",      // IPv6 link-local
}

var blockedPrefixNets []netip.Prefix

func init() {
	for _, c := range blockedPrefixes {
		p, err := netip.ParsePrefix(c)
		if err != nil {
			panic(fmt.Sprintf("invalid blocked prefix %q: %v", c, err))
		}
		blockedPrefixNets = append(blockedPrefixNets, p)
	}
}

// ValidateURL validates that rawURL is safe for outbound webhook delivery:
//   - scheme must be http or https
//   - host must be present
//   - the resolved IP address(es) must not fall into a blocked range
//
// DNS resolution is performed at validation time. A hostname that resolves
// to both public and private IPs is rejected (any private IP blocks it).
// This prevents DNS-rebinding attacks where a hostname flips between a
// public IP (passes validation) and a private IP (hit at delivery time).
func ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%w: scheme %q must be http or https", ErrBlockedURL, u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: empty host", ErrBlockedURL)
	}

	// If the host is already an IP literal, check it directly without DNS.
	if addr, err := netip.ParseAddr(host); err == nil {
		if isBlocked(addr) {
			return fmt.Errorf("%w: IP %s is in a blocked range", ErrBlockedURL, addr)
		}
		return nil
	}

	// Hostname: resolve and check every returned address.
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("resolve host %q: %w", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("%w: host %q resolved to no addresses", ErrBlockedURL, host)
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		if isBlocked(addr) {
			return fmt.Errorf("%w: host %q resolves to %s which is in a blocked range", ErrBlockedURL, host, addr)
		}
	}
	return nil
}

// isBlocked reports whether addr falls into any blocked prefix.
func isBlocked(addr netip.Addr) bool {
	for _, p := range blockedPrefixNets {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// loopbackPrefixes are the CIDR ranges allowed when loopback is permitted
// (test mode only): IPv4 and IPv6 loopback.
var loopbackPrefixes = []string{
	"127.0.0.0/8",
	"::1/128",
}

var loopbackPrefixNets []netip.Prefix

func init() {
	for _, c := range loopbackPrefixes {
		p, err := netip.ParsePrefix(c)
		if err != nil {
			panic(fmt.Sprintf("invalid loopback prefix %q: %v", c, err))
		}
		loopbackPrefixNets = append(loopbackPrefixNets, p)
	}
}

// isLoopback reports whether addr is a loopback address.
func isLoopback(addr netip.Addr) bool {
	for _, p := range loopbackPrefixNets {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// ValidateURLAllowLoopback is like ValidateURL but permits loopback
// (127.0.0.0/8, ::1) addresses. It is intended for test environments where
// httptest servers bind to localhost. All other private ranges (RFC 1918,
// link-local, cloud metadata) remain blocked.
func ValidateURLAllowLoopback(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%w: scheme %q must be http or https", ErrBlockedURL, u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: empty host", ErrBlockedURL)
	}

	check := func(addr netip.Addr) error {
		if isLoopback(addr) {
			return nil // allowed in test mode
		}
		if isBlocked(addr) {
			return fmt.Errorf("%w: IP %s is in a blocked range", ErrBlockedURL, addr)
		}
		return nil
	}

	if addr, err := netip.ParseAddr(host); err == nil {
		return check(addr)
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("resolve host %q: %w", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("%w: host %q resolved to no addresses", ErrBlockedURL, host)
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		if err := check(addr); err != nil {
			return err
		}
	}
	return nil
}
