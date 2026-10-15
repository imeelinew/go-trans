package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Some local resolvers return no IPv4 address for newly created tunnel hosts.
// Resolve only that host via DoH; TLS still verifies the original hostname.
func tunnelHTTPClient(endpoint string) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if !strings.HasSuffix(host, ".trycloudflare.com") {
			return dialer.DialContext(ctx, network, address)
		}
		ips, err := resolveTunnel(ctx, endpoint, host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			conn, e := dialer.DialContext(ctx, "tcp4", net.JoinHostPort(ip, port))
			if e == nil {
				return conn, nil
			}
			err = e
		}
		return nil, err
	}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second}
}
func resolveTunnel(ctx context.Context, endpoint, host string) ([]string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("name", host)
	q.Set("type", "A")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/dns-json")
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tunnel DNS request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("tunnel DNS: HTTP %d", resp.StatusCode)
	}
	var answer struct {
		Status int
		Answer []struct {
			Type int
			Data string
		}
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&answer); err != nil {
		return nil, err
	}
	var ips []string
	if answer.Status == 0 {
		for _, rr := range answer.Answer {
			ip := net.ParseIP(rr.Data)
			if rr.Type == 1 && ip != nil && ip.To4() != nil && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsUnspecified() {
				ips = append(ips, rr.Data)
			}
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("tunnel DNS: no public IPv4 address")
	}
	return ips, nil
}
