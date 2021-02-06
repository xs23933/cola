# cola
Coke is a very simple microservice web framework

# 功能开发

- 支持Cookie操作
- 支持 html/template

# How to use 如何使用（通过 go mod方式）

## Step 1: 创建项目
```sh
# 创建一个目录
mkdir yourapp 

# 进入目录 
cd yourapp

# 初始化 go mod
go mod init yourapp

# vscode 打开这个目录
code .
```
## Step 2: 编写一个网页服务器

### 写一个main 

创建一个文件名为 main.go 并写入如下代码
```go
package main

import (
	"github.com/xs23933/cola"
)

func main(){
	app := cola.New()
	app.Serve(8080)
}
```
以上代码启动了网页服务器并监听了 8080端口

### 编写一个 Handler (Controller)
创建一个目录 handler 并在里面创建一个你自己想起名的文件 例如 handler.go

handler.go
```go
package handler

import (
	"github.com/xs23933/cola"
)

type Handler struct {
	cola.Handler
}

// Get  这里就实现了 根Path Handler
//
// 同等于  get /
func (Handler) Get(c *cola.Ctx) {
	c.Write([]byte("i love china"))
}

// GetAbout 带/目录的方法 
//
// get /about
func (Handler) GetAbout(c *cola.Ctx) {
	c.Write([]byte("i'm Cola, it's very sample web module"))
}

// GetWelcomeParam 带参数的方法定义，这里的Param是关键字
// get /welcome/:param
func (Handler) GetWelcomeParam(c *cola.Ctx) {
	your := c.Params("param")
	c.Write([]byte("Welcome " + your))
}

// PostSignup 接收 Post 数据方式 及采用 json返回数据
//
// post /signup
func (Handler) PostSignup(c *cola.Ctx) {
	form := make(map[string]interface{})
	if err := c.ReadBody(&form); err != nil {
		c.ToJSON(nil, err) // 读取失败返回错误信息
		return
	}

	c.ToJSON(form, nil)
}
```

### 修改下原来的 main.go
main.go 改为如下
```go
package main

import (
	"yourapp/handler"
	"github.com/xs23933/cola"
)

func main(){
	app := cola.New()
	// 注册 handler
	app.Use(new(Handler))
	if err := app.Serve(8080); err != nil { // 友好的监视错误信息
		panic(err)
	}
}
```
ok 以上代码就能实现一个简单的网页服务器

------

# 更多高级使用方法

### 针对每个handler 可以设置 方法前缀  
```go
func (Handler) Init(){
	h.SetPrefix("/api")
}
```

### handler 前面挂钩子 比如识别网站，查询登录用户信息等放hook里面
```go
func (Handler) Preload(c *cola.Ctx) {
	token := c.Get("Authorization") // 获得 headder 或 通过 cookie 获得 登录令牌
	user := model.AuthUser(token) // 查询已登录用户信息 返回
	c.Vars("us", user) // 存在 Ctx(Context) 中 任何下一步插件使用
}
```
----

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