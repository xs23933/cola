package main

import (
	"cola"
	"cola/examples/app/handler"
	"cola/examples/app/models"
)

func main() {
	debug := true
	view := cola.NewView("./views", ".html", debug).Layout("layout")
	app := cola.New(&cola.Options{
		Prefork: false,
		Debug:   debug,
		ETag:    true,
		Views:   view,
		Db: &cola.Db{
			Driver: "mysql",
			DSN:    "root:caisong.com@tcp(127.0.0.1:3306)/example?charset=utf8mb4&parseTime=True&loc=UTC",
		},
	})
	app.Use(models.Models{})
	app.Use(new(handler.Handler))

	app.Serve(8080)
}
