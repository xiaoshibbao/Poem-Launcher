//go:build windows

package launcher

import "testing"

func TestLocalSystemProxyParsing(t *testing.T) {
	for _, v := range []struct {
		s string
		p int
	}{{"127.0.0.1:7890", 7890}, {"http=127.0.0.1:7890;https=127.0.0.1:7890", 7890}, {"http://localhost:7890", 7890}, {"1.2.3.4:7890", 0}, {"socks=127.0.0.1:7890", 0}, {"socks5://127.0.0.1:7890", 0}, {"http://user:pass@127.0.0.1:7890", 0}, {"127.0.0.1:99999", 0}} {
		if got := localHTTPProxyPort(v.s); got != v.p {
			t.Fatalf("%s: %d != %d", v.s, got, v.p)
		}
	}
}
