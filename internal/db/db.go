package db

import (
	"fmt"
	"log"
	"os"

	gv "github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Iint() (*gorm.DB, error) {

	err := gv.Load()
	if err != nil {
		log.Fatal("Env file not load", err)
		return nil, nil
	}

	pass := os.Getenv("Password")
	if pass == " " {
		log.Fatal("Env file not have Password for db ", err)
		return nil, nil
	}

	dsn := fmt.Sprintf(
		"host=localhost user=postgres password=%s dbname=ruvds port=8010 sslmode=disable",
		pass,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate()
	if err != nil {
		return nil, err
	}

	return db, nil

}
