package models

import (
	"cola"

	"github.com/davecgh/go-spew/spew"
	model "github.com/xs23933/cola-model"
)

type User struct {
	model.Model
	User     string `gorm:"user"`
	Password string `gorm:"password"`
}

func Init() {
	spew.Dump(cola.DB)
	cola.DB.Set("gorm:table_options", "ENGINE=InnoDB").AutoMigrate(new(User))
	user := &User{
		User:     "admin",
		Password: "admin",
	}
	cola.DB.Save(user)
}
