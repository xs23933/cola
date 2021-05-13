package cola

import "fmt"

// Map map[string]interface{}
type Map map[string]interface{}

type handle interface {
	Core(c ...*Core) *Core
	Prefix() string          // get Prefix
	Init()                   // init handler
	Preload(*Ctx)            // before hook
	SetHandName(name string) // get handlerName
	HandName() string        // 获得名称
}

// Handler Sample Handler
type Handler struct {
	prefix   string
	handName string
	core     *Core
	Handlers map[string]struct{}
}

func (h *Handler) PushPath(method, path string) {
	h.Handlers[fmt.Sprintf("%s|%s", method, path)] = struct{}{}
}

// Core 设置或获得Core
func (h *Handler) Core(c ...*Core) *Core {
	if len(c) > 0 {
		h.core = c[0]
	}
	h.Handlers = make(map[string]struct{})
	return h.core
}

// Preload 预处理使用 必须配置 Next结尾
func (h *Handler) Preload(c *Ctx) {
	c.Next()
}

// SetHandName 设置Handler 名称
func (h *Handler) SetHandName(name string) {
	h.handName = name
}

// HandName 获得HandName
func (h *Handler) HandName() string {
	return h.handName
}

// SetPrefix set prefix path
func (h *Handler) SetPrefix(prefix string) {
	h.prefix = prefix
}

// Init use once called
func (h *Handler) Init() {}

// Prefix set prefix path
func (h *Handler) Prefix() string {
	return h.prefix
}
