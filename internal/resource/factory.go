package resource

import (
	"github.com/r0madeus/veto/internal/core"

	// Register all adapters
	_ "github.com/r0madeus/veto/internal/adapters/bundle"
	_ "github.com/r0madeus/veto/internal/adapters/dconf"
	_ "github.com/r0madeus/veto/internal/adapters/docker"
	_ "github.com/r0madeus/veto/internal/adapters/file"
	_ "github.com/r0madeus/veto/internal/adapters/font"
	_ "github.com/r0madeus/veto/internal/adapters/git"
	_ "github.com/r0madeus/veto/internal/adapters/icon"
	_ "github.com/r0madeus/veto/internal/adapters/identity"
	_ "github.com/r0madeus/veto/internal/adapters/pkg"
	_ "github.com/r0madeus/veto/internal/adapters/service"
	_ "github.com/r0madeus/veto/internal/adapters/shell"
)

// CreateResourceWithParams uses the central registry to instantiate resources.
func CreateResourceWithParams(resType string, name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
	if params == nil {
		params = make(map[string]interface{})
	}

	// For backward compatibility or convenience, ensure state is set if possible
	if _, ok := params["state"]; !ok {
		params["state"] = "present"
	}

	return core.CreateResource(resType, name, params, ctx)
}
