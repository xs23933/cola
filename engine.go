package cola

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Engine Module engine
type Engine struct {
	core *Core
}

var (
	modules   = make(map[string]ModuleInfo)
	modulesMu sync.RWMutex
)

// Module interface
type Module interface {
	Module() ModuleInfo
}

// ModuleInfo ModuleInfo
type ModuleInfo struct {
	ID  string
	New func() Module
}
type hasHand interface {
	Preload(*Ctx)
}

// NewEngine 创建
func NewEngine(opts *Options) *Engine {
	engine := &Engine{
		core: New(opts),
	}
	return engine
}

// Core 返回 core对象
func (e *Engine) Core() *Core {
	return e.core
}

// Serve 启动服务 如果modules 定义 prefix 为 module. 这里则加载
func (e *Engine) Serve(port interface{}) error {
	for _, m := range GetModules("module") {
		Log.Debug(m.ID)
		if mod, ok := m.New().(hasHand); ok { // 如果这个模块是handler 注册这个模块
			e.core.Use(mod)
		}
	}
	return e.core.Serve(port)
}

// RegisterModule 注册模块
func RegisterModule(inst Module) {
	mod := inst.Module()
	modulesMu.Lock()
	defer modulesMu.Unlock()

	if _, ok := modules[string(mod.ID)]; ok {
		Log.Info("module already registered: %s\n", mod.ID)
		return
	}
	modules[mod.ID] = mod
}

// GetModule 通过ID获得模块
//
// 获得后自己定义接口再调用某方法
func GetModule(name string) (ModuleInfo, error) {
	modulesMu.RLock()
	defer modulesMu.RUnlock()
	m, ok := modules[name]
	if !ok {
		return ModuleInfo{}, fmt.Errorf("module not register: %s", name)
	}
	return m, nil
}

// GetModules 获取指定开头的模块
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

		// match only the next level of nesting
		if len(modParts) < len(scopeParts) {
			continue
		}

		// specified parts must be exact matches
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
