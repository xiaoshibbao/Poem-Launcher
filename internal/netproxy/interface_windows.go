//go:build windows

package netproxy

import (
	"context"
	"fmt"
	"math/bits"
	"net"
	"net/http"
	"syscall"
	"time"
)

// BindServerInterface must be called before Start. Only connections to the
// configured literal IPv4 server are bound; unrelated proxy traffic is unchanged.
func (p *Proxy) BindServerInterface(index uint32, logf func(string, ...any)) error {
	target := net.ParseIP(p.target.Hostname())
	if target == nil || target.To4() == nil {
		return fmt.Errorf("server interface binding requires a literal IPv4 address")
	}
	transport, ok := p.client.Transport.(*http.Transport)
	if !ok {
		return fmt.Errorf("unsupported HTTP transport")
	}
	plain := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	bound := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	iface, err := net.InterfaceByIndex(int(index))
	if err != nil {
		return fmt.Errorf("server interface: %w", err)
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return err
	}
	for _, address := range addresses {
		ip, _, parseErr := net.ParseCIDR(address.String())
		if parseErr == nil && ip.To4() != nil && !ip.IsLinkLocalUnicast() {
			bound.LocalAddr = &net.TCPAddr{IP: ip.To4()}
			break
		}
	}
	if bound.LocalAddr == nil {
		return fmt.Errorf("真实网卡没有可用的 IPv4 地址")
	}
	bound.Control = func(network, address string, raw syscall.RawConn) error {
		if network != "tcp4" {
			return fmt.Errorf("server interface binding requires TCP/IPv4")
		}
		var socketErr error
		if err := raw.Control(func(fd uintptr) {
			// IP_UNICAST_IF expects the interface index in network byte order.
			socketErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, 31, int(bits.ReverseBytes32(index)))
		}); err != nil {
			return err
		}
		return socketErr
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil || !target.Equal(net.ParseIP(host)) {
			return plain.DialContext(ctx, network, address)
		}
		conn, err := bound.DialContext(ctx, "tcp4", address)
		if err != nil {
			logf("server socket bind failed interface=%d remote=%s: %v", index, address, err)
			return nil, err
		}
		logf("server socket connected interface=%d local=%s remote=%s", index, conn.LocalAddr(), conn.RemoteAddr())
		return conn, nil
	}
	return nil
}
