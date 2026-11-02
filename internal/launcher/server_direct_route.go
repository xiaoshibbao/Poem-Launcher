//go:build windows

package launcher

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Legacy IP Helper rows use network-order IPv4 DWORDs, represented here as
// little-endian integers to preserve their four bytes when passed to Windows.
type directRoute struct{ Dest, Mask, Policy, Gateway, Index, Type, Proto, Age, AS, Metric, Metric2, Metric3, Metric4, Metric5 uint32 }

var ipHelper = syscall.NewLazyDLL("iphlpapi.dll")
var applyServerRoute = changeDirectRoute

func readDirectRoutes() ([]directRoute, error) {
	p := ipHelper.NewProc("GetIpForwardTable")
	var size uint32
	result, _, _ := p.Call(0, uintptr(unsafe.Pointer(&size)), 0)
	if result != 122 {
		return nil, fmt.Errorf("GetIpForwardTable: %d", result)
	}
	for retry := 0; retry < 3; retry++ {
		b := make([]byte, size)
		result, _, _ = p.Call(uintptr(unsafe.Pointer(&b[0])), uintptr(unsafe.Pointer(&size)), 0)
		if result == 122 {
			continue
		}
		if result != 0 {
			return nil, fmt.Errorf("GetIpForwardTable: %d", result)
		}
		count := int(binary.LittleEndian.Uint32(b))
		if count > (len(b)-4)/56 {
			return nil, fmt.Errorf("invalid route table size")
		}
		rows := make([]directRoute, count)
		for i := range rows {
			rows[i] = *(*directRoute)(unsafe.Pointer(&b[4+i*56]))
		}
		return rows, nil
	}
	return nil, fmt.Errorf("route table kept changing")
}

func changeDirectRoute(name string, row directRoute) error {
	code, _, _ := ipHelper.NewProc(name).Call(uintptr(unsafe.Pointer(&row)))
	if code != 0 {
		return syscall.Errno(code)
	}
	return nil
}

func virtualRouteInterface(name string) bool {
	name = strings.ToLower(name)
	for _, word := range []string{"tunnel", "tun", "tap", "clash", "mihomo", "meta", "vpn", "wireguard", "virtual", "tailscale", "zerotier", "loopback"} {
		if strings.Contains(name, word) {
			return true
		}
	}
	return false
}

// Only bypass a detected virtual default route, and only for the configured
// server. Existing host routes are left alone, even when they use a VPN.
func planServerRoute(rows []directRoute, interfaces []net.Interface, ip net.IP) (directRoute, bool, error) {
	byIndex := map[uint32]net.Interface{}
	for _, i := range interfaces {
		byIndex[uint32(i.Index)] = i
	}
	tun := false
	var physical *directRoute
	for i := range rows {
		r := rows[i]
		iface, ok := byIndex[r.Index]
		if !ok || iface.Flags&net.FlagUp == 0 {
			continue
		}
		if r.Dest != 0 || r.Mask != 0 {
			continue
		}
		if virtualRouteInterface(iface.Name) {
			tun = true
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 || r.Gateway == 0 || len(iface.HardwareAddr) == 0 {
			continue
		}
		if physical == nil || r.Metric < physical.Metric {
			copy := r
			physical = &copy
		}
	}
	if !tun {
		return directRoute{}, false, nil
	}
	ip = ip.To4()
	if ip == nil {
		return directRoute{}, false, fmt.Errorf("TUN 直连目前需要 IPv4 服务器地址")
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return directRoute{}, false, nil
	}
	if ip[0] == 198 && (ip[1] == 18 || ip[1] == 19) {
		return directRoute{}, false, fmt.Errorf("服务器域名被 TUN 解析为 Fake-IP，请在自定义地址中填写服务器真实 IPv4 地址")
	}
	dest := binary.LittleEndian.Uint32(ip)
	for _, r := range rows {
		if r.Dest == dest && r.Mask == 0xffffffff {
			return directRoute{}, false, nil
		}
	}
	if physical == nil {
		return directRoute{}, false, fmt.Errorf("检测到 TUN，但未找到真实网卡的默认网关")
	}
	r := *physical
	r.Dest = dest
	r.Mask = 0xffffffff
	r.Policy = 0
	r.Type = 4
	r.Proto = 3
	r.Age = 0
	r.AS = 0
	r.Metric2 = 0xffffffff
	r.Metric3 = 0xffffffff
	r.Metric4 = 0xffffffff
	r.Metric5 = 0xffffffff
	return r, true, nil
}

type routeLease struct {
	row  directRoute
	refs int
}

var serverRoutes = struct {
	sync.Mutex
	leases map[uint32]*routeLease
}{leases: map[uint32]*routeLease{}}

func acquireServerDirectRoute(serverURL string, logf func(string, ...any)) (func(), error) {
	noop := func() {}
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(u.Hostname(), "localhost") {
		return noop, nil
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
		return noop, nil
	}
	// Avoid DNS/network work for loopback and ordinary non-TUN connections.
	rows, err := readDirectRoutes()
	if err != nil {
		return nil, err
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	hasTUN := false
	for _, r := range rows {
		for _, i := range interfaces {
			if r.Dest == 0 && r.Mask == 0 && int(r.Index) == i.Index && i.Flags&net.FlagUp != 0 && virtualRouteInterface(i.Name) {
				hasTUN = true
			}
		}
	}
	if !hasTUN {
		return noop, nil
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ips, lookupErr := net.DefaultResolver.LookupIP(ctx, "ip4", u.Hostname())
		if lookupErr != nil {
			return nil, fmt.Errorf("服务器解析失败：%w", lookupErr)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("服务器没有 IPv4 地址")
		}
		// All DNS answers must be routed, not just the first. Limit this feature to
		// literal IPv4 until the forwarder can pin DNS and route leases together.
		return nil, fmt.Errorf("检测到 TUN，请将自定义服务器地址改为真实 IPv4（解析结果 %s），以确保游戏和代理走同一条直连路由", ips[0])
	}
	serverRoutes.Lock()
	defer serverRoutes.Unlock()
	dest := uint32(0)
	if v := ip.To4(); v != nil {
		dest = binary.LittleEndian.Uint32(v)
	}
	if lease := serverRoutes.leases[dest]; lease != nil {
		lease.refs++
		return releaseServerRoute(dest, logf), nil
	}
	// A previous game can release its route while DNS/interface enumeration
	// runs; inspect again under the lease lock before borrowing an existing row.
	rows, err = readDirectRoutes()
	if err != nil {
		return nil, err
	}
	row, needed, err := planServerRoute(rows, interfaces, ip)
	if err != nil {
		return nil, err
	}
	if !needed {
		return noop, nil
	}
	if err := applyServerRoute("CreateIpForwardEntry", row); err != nil {
		return nil, fmt.Errorf("创建服务器直连路由失败（请以管理员权限启动）：%w", err)
	}
	serverRoutes.leases[dest] = &routeLease{row: row, refs: 1}
	logf("TUN bypass created server=%s interface=%d gateway=%s", ip, row.Index, routeIPv4(row.Gateway))
	return releaseServerRoute(dest, logf), nil
}
func routeIPv4(value uint32) net.IP {
	b := make(net.IP, 4)
	binary.LittleEndian.PutUint32(b, value)
	return b
}
func releaseServerRoute(dest uint32, logf func(string, ...any)) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			serverRoutes.Lock()
			defer serverRoutes.Unlock()
			lease := serverRoutes.leases[dest]
			if lease == nil {
				return
			}
			lease.refs--
			if lease.refs != 0 {
				return
			}
			if err := applyServerRoute("DeleteIpForwardEntry", lease.row); err != nil && err != syscall.Errno(1168) {
				logf("TUN bypass cleanup failed server=%s: %v", routeIPv4(dest), err)
				return // Retain ownership so a later session can retry cleanup.
			} else {
				logf("TUN bypass removed server=%s", routeIPv4(dest))
			}
			delete(serverRoutes.leases, dest)
		})
	}
}

// Read-only selection for socket pinning, including when a host route is already
// present and does not need creating. Never select a TUN/TAP or loopback adapter.
func serverPhysicalInterface(serverURL string) (uint32, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return 0, err
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || ip.To4() == nil || ip.IsLoopback() || ip.IsPrivate() {
		return 0, nil
	}
	rows, err := readDirectRoutes()
	if err != nil {
		return 0, err
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return 0, err
	}
	var best *directRoute
	for _, r := range rows {
		if r.Dest != 0 || r.Mask != 0 || r.Gateway == 0 {
			continue
		}
		for _, i := range interfaces {
			if uint32(i.Index) == r.Index && i.Flags&net.FlagUp != 0 && i.Flags&net.FlagLoopback == 0 && len(i.HardwareAddr) > 0 && !virtualRouteInterface(i.Name) {
				if best == nil || r.Metric < best.Metric {
					copy := r
					best = &copy
				}
			}
		}
	}
	if best == nil {
		return 0, fmt.Errorf("找不到服务器连接使用的真实网卡")
	}
	return best.Index, nil
}
