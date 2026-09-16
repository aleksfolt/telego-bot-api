package netpool

import (
	"context"
	"net"
	"sync/atomic"
)

// PoolDialer distributes outbound TCP connections across multiple local IP addresses.
// This avoids Linux ephemeral port exhaustion (EADDRNOTAVAIL) when handling tens of thousands
// of MTProto connections to Telegram DCs from a single machine.
type PoolDialer struct {
	ips     []net.IP
	counter uint64
}

// NewPoolDialer creates a dialer that rotates through available outbound IPs.
func NewPoolDialer(ipStrings []string) *PoolDialer {
	var ips []net.IP
	for _, s := range ipStrings {
		if ip := net.ParseIP(s); ip != nil {
			ips = append(ips, ip)
		}
	}
	return &PoolDialer{ips: ips}
}

// DialContext dials the target address, optionally binding to a local IP from the pool.
func (p *PoolDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := &net.Dialer{}

	if len(p.ips) > 0 {
		idx := atomic.AddUint64(&p.counter, 1) % uint64(len(p.ips))
		dialer.LocalAddr = &net.TCPAddr{
			IP: p.ips[idx],
		}
	}

	return dialer.DialContext(ctx, network, address)
}
