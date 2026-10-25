package network

import (
	"fmt"
	"strconv"

	"github.com/r0madeus/veto/internal/core"
	"github.com/r0madeus/veto/internal/utils"
)

func init() {
	core.RegisterResource("firewall_rule", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewFirewallAdapter(name, params), nil
	})
}

type FirewallAdapter struct {
	core.BaseResource
	Port            int
	Proto           string // tcp, udp, any
	Action          string // allow, deny, reject
	From            string // source IP/CIDR (optional, default any)
	To              string // dest IP (optional, default any)
	State           string // present, absent
	ActionPerformed string
}

func NewFirewallAdapter(name string, params map[string]interface{}) core.Resource {
	port := 0
	if p, ok := params["port"].(int); ok {
		port = p
	} else if pStr, ok := params["port"].(string); ok {
		port, _ = strconv.Atoi(pStr)
	}

	proto, _ := params["proto"].(string)
	if proto == "" {
		proto = "any"
	}

	action, _ := params["action"].(string)
	if action == "" {
		action = "allow"
	}

	from, _ := params["from"].(string)
	if from == "" {
		from = "any"
	}

	to, _ := params["to"].(string)
	if to == "" {
		to = "any"
	}

	state, _ := params["state"].(string)
	if state == "" {
		state = "present"
	}

	return &FirewallAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "firewall_rule"},
		Port:         port,
		Proto:        proto,
		Action:       action,
		From:         from,
		To:           to,
		State:        state,
	}
}

func (r *FirewallAdapter) Validate(ctx *core.SystemContext) error {
	if !utils.IsValidPort(r.Port) {
		return fmt.Errorf("invalid port %d", r.Port)
	}
	if !utils.IsValidProtocol(r.Proto) {
		return fmt.Errorf("invalid protocol %s", r.Proto)
	}
	if !utils.IsOneOf(r.Action, "allow", "deny", "reject") {
		return fmt.Errorf("invalid action %s", r.Action)
	}
	if !utils.IsOneOf(r.State, "present", "absent") {
		return fmt.Errorf("invalid state %s", r.State)
	}
	return nil
}

func (r *FirewallAdapter) Check(ctx *core.SystemContext) (bool, error) {
	provider := GetFirewallProvider(ctx)
	return provider.CheckRule(ctx, r)
}

func (r *FirewallAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needs, err := r.Check(ctx)
	if err != nil {
		return core.Failure(err, "Check failed"), err
	}
	if !needs {
		return core.SuccessNoChange(fmt.Sprintf("Firewall rule %s is correct", r.Name)), nil
	}

	provider := GetFirewallProvider(ctx)
	return provider.ApplyRule(ctx, r)
}

func (r *FirewallAdapter) Revert(ctx *core.SystemContext) error {
	provider := GetFirewallProvider(ctx)
	return provider.RevertRule(ctx, r)
}
