//go:build windows

package launcher

import (
	"net"
	"testing"
	"unsafe"
)

func TestServerDirectRoutePlan(t *testing.T) {
	if unsafe.Sizeof(directRoute{}) != 56 {
		t.Fatal("incorrect Windows ABI")
	}
	interfaces := []net.Interface{{Index: 48, Name: "Meta Tunnel", Flags: net.FlagUp}, {Index: 13, Name: "Ethernet", Flags: net.FlagUp, HardwareAddr: net.HardwareAddr{1, 2, 3, 4, 5, 6}}}
	physical := directRoute{Index: 13, Gateway: 0x0100a8c0, Metric: 25}
	rows := []directRoute{{Index: 48, Gateway: 0x020012c6}, physical}
	row, needed, err := planServerRoute(rows, interfaces, net.ParseIP("43.142.131.90"))
	if err != nil || !needed || row.Index != 13 || row.Gateway != physical.Gateway || row.Dest != 0x5a838e2b || row.Mask != 0xffffffff || row.Proto != 3 || row.Metric != 25 {
		t.Fatalf("wrong route %+v %t %v", row, needed, err)
	}
	for _, ip := range []string{"127.0.0.1", "192.168.0.8"} {
		if _, needed, err := planServerRoute(rows, interfaces, net.ParseIP(ip)); needed || err != nil {
			t.Fatalf("local address %s: %v", ip, err)
		}
	}
	if _, needed, err := planServerRoute([]directRoute{physical}, interfaces, net.ParseIP("43.142.131.90")); needed || err != nil {
		t.Fatal("changed non-TUN network")
	}
	if _, needed, err := planServerRoute(append(rows, row), interfaces, net.ParseIP("43.142.131.90")); needed || err != nil {
		t.Fatal("overwrote existing host route")
	}
	if _, _, err := planServerRoute(rows, interfaces, net.ParseIP("198.18.0.10")); err == nil {
		t.Fatal("accepted fake IP")
	}
	if _, _, err := planServerRoute(rows[:1], interfaces, net.ParseIP("43.142.131.90")); err == nil {
		t.Fatal("accepted missing real gateway")
	}
}
func TestReadNativeRoutingTable(t *testing.T) {
	rows, err := readDirectRoutes()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("empty routing table")
	}
	for _, r := range rows {
		if r.Dest == 0 && r.Mask == 0 {
			t.Logf("default if=%d gateway=%s metric=%d", r.Index, routeIPv4(r.Gateway), r.Metric)
		}
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	row, needed, err := planServerRoute(rows, interfaces, net.ParseIP("43.142.131.90"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Poem bypass needed=%t interface=%d gateway=%s", needed, row.Index, routeIPv4(row.Gateway))
}

func TestServerRouteLeaseCleanup(t *testing.T) {
	old := applyServerRoute
	defer func() { applyServerRoute = old; delete(serverRoutes.leases, 123) }()
	calls := 0
	applyServerRoute = func(name string, row directRoute) error {
		if name != "DeleteIpForwardEntry" {
			t.Fatal(name)
		}
		calls++
		return nil
	}
	serverRoutes.leases[123] = &routeLease{row: directRoute{Dest: 123}, refs: 2}
	logf := func(string, ...any) {}
	a, b := releaseServerRoute(123, logf), releaseServerRoute(123, logf)
	a()
	a()
	if calls != 0 || serverRoutes.leases[123].refs != 1 {
		t.Fatal("removed route while another game uses it")
	}
	b()
	b()
	if calls != 1 || serverRoutes.leases[123] != nil {
		t.Fatal("cleanup must happen exactly once after final game")
	}
}
