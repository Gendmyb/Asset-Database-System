// Package webhook — SSRF 防护 (私有网段拒绝)
// 对应架构文档 §7.4 SSRF 防护
package webhook

import (
	"net"
)

// deniedCIDRs 完整私有网段列表
var deniedCIDRs = []string{
	// IPv4
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",     // CGNAT
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"224.0.0.0/4",
	// Docker
	"172.17.0.0/16",
	"172.18.0.0/16",
	// IPv6
	"::1/128",
	"fe80::/10",
	"fc00::/7", // Unique Local Address
}

var deniedNetworks []*net.IPNet

func init() {
	for _, cidr := range deniedCIDRs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("invalid CIDR: " + cidr)
		}
		deniedNetworks = append(deniedNetworks, n)
	}
}

// IsPrivateIP 检查 IP 是否在私有网段
func IsPrivateIP(ip net.IP) bool {
	for _, n := range deniedNetworks {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// AllowedHosts 示例白名单
func AllowedHosts() []string {
	return []string{"hooks.slack.com", "api.github.com"}
}
