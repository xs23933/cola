package cola

type handle interface {
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
