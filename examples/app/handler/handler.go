package handler

import "cola"

// Handler Handler
type Handler struct {
	cola.Handler
}

// GetThemeParams get /theme/:param?
func (Handler) GetThemeParams(c *cola.Ctx) {
	theme := c.Params("param")
	c.Core.Views.Theme(theme)
}

// Get get /
func (Handler) Get(c *cola.Ctx) {
	c.Vars("title", "Hello world")
	c.Render("main")
}
