package network

import (
	"github.com/r0madeus/veto/internal/core"
)

func GetFirewallProvider(ctx *core.SystemContext) FirewallProvider {
	// Default to UFW for now.
	// Future: detect firewalld or iptables
	return &UFWProvider{}
}
