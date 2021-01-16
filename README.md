# cola
Coke is a very simple microservice web framework

# 功能开发

- 支持Cookie操作
- 支持 html/template

# Handler 定义 自动注册方法

```go

// Handler Handler
type Handler struct {
	cola.Handler
}

// GetThemeParams get /theme/:param?
func (Handler) GetThemeParams(c *cola.Ctx) {
	theme := c.Params("param") // 获得路径参数
	c.Core.Views.Theme(theme) // 修改模版风格
}

// Get get /
func (Handler) Get(c *cola.Ctx) {
	c.Vars("title", "Hello world") // 设置全局变量
	c.Render("main") // 模版解析
}

```

# 启动服务

```go

func main() {
	debug := true
	view := cola.NewView("./views", ".html", debug).Layout("layout")
	app := cola.New(&cola.Options{
		Prefork: false,
		Debug:   debug,
		ETag:    true,
		Views:   view,
	})

	models.New("mysql", "root:caisong.com@tcp(127.0.0.1:3306)/example?charset=utf8mb4&parseTime=True&loc=UTC", debug)
	app.Use(new(handler.Handler))

	app.Serve(8080)
}

```