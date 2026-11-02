//go:build windows

package launcher

import (
	"golang.org/x/sys/windows/registry"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func localHTTPProxyPort(value string) int {
	values := strings.Split(value, ";")
	for _, prefix := range []string{"https=", "http=", ""} {
		for _, part := range values {
			part = strings.TrimSpace(part)
			if prefix != "" {
				if !strings.HasPrefix(strings.ToLower(part), prefix) {
					continue
				}
				part = part[len(prefix):]
			} else if strings.Contains(part, "=") {
				continue
			}
			if !strings.Contains(part, "://") {
				part = "http://" + part
			}
			u, err := url.Parse(part)
			if err != nil || u.Scheme != "http" || u.User != nil {
				continue
			}
			ip := net.ParseIP(u.Hostname())
			if !strings.EqualFold(u.Hostname(), "localhost") && (ip == nil || !ip.IsLoopback()) {
				continue
			}
			port, err := strconv.Atoi(u.Port())
			if err == nil && port > 0 && port <= 65535 {
				return port
			}
		}
	}
	return 0
}
func systemHTTPProxyPort() int {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return 0
	}
	defer key.Close()
	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil || enabled == 0 {
		return 0
	}
	server, _, err := key.GetStringValue("ProxyServer")
	if err != nil {
		return 0
	}
	return localHTTPProxyPort(server)
}
