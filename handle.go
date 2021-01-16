package cola

type handle interface {
	Prefix() string // get Prefix
	Init()          // init handler
	Preload(*Ctx)   // before hook
}

// Handler Sample Handler
type Handler struct {
	prefix string
}

// Preload 预处理使用 必须配置 Next结尾
func (h *Handler) Preload(c *Ctx) {
	c.Next()
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
