package cola

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ModuleID Module id
type ModuleID string

func (mi ModuleInfo) String() string {
	return string(mi.ID)
}

// ModuleInfo ModuleInfo
type ModuleInfo struct {
	ID  ModuleID
	New func() Module
}

// Module module interface
type Module interface {
	Module() ModuleInfo
}

// RegisterModule register module
func RegisterModule(inst Module) {
	mod := inst.Module()
	modulesMu.Lock()
	defer modulesMu.Unlock()
	if _, ok := modules[mod.String()]; ok {
		Log.Debug("module already registered: %s\n", mod.String())
		return
	}

	modules[mod.String()] = mod
}

// GetModules load some scope modules
func GetModules(scope string) []ModuleInfo {
	modulesMu.RLock()
	defer modulesMu.RUnlock()
	scopeParts := strings.Split(scope, ".")

	if scope == "" {
		scopeParts = []string{}
	}

	mods := make([]ModuleInfo, 0)

iterateModules:
	for id, m := range modules {
		modParts := strings.Split(id, ".")
		if len(modParts) != len(scopeParts)+1 {
			continue
		}

		for i := range scopeParts {
			if modParts[i] != scopeParts[i] {
				continue iterateModules
			}
		}

		mods = append(mods, m)
	}

	sort.Slice(mods, func(i, j int) bool {
		return mods[i].ID < mods[j].ID
	})
	return mods
}

// GetModule get module by name
func GetModule(name string) (ModuleInfo, error) {
	modulesMu.RLock()
	defer modulesMu.RUnlock()
	m, ok := modules[name]
	if !ok {
		return ModuleInfo{}, fmt.Errorf("module not registered: %s", name)
	}
	return m, nil
}

var (
	modules   = make(map[string]ModuleInfo)
	modulesMu sync.RWMutex
)
