// Command api serves the user settings HTTP API.
//
//	DATABASE_URL  postgres://settings:settings@localhost:5432/settings?sslmode=disable
//	PORT          8080
//
// The database schema is owned by Liquibase (db/changelog); this process never
// creates or alters tables.
package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/chrismguevara/config-example/backend/internal/settings"
)

func main() {
	dsn := getenv("DATABASE_URL", "postgres://settings:settings@localhost:5432/settings?sslmode=disable")
	port := getenv("PORT", "8080")

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
	if err != nil {
		log.Fatalf("connect to postgres: %v", err)
	}

	validator, err := settings.NewValidator()
	if err != nil {
		log.Fatalf("compile settings schemas: %v", err)
	}
	svc := settings.NewService(settings.NewGormStore(db), validator)

	r := gin.Default()
	settings.RegisterRoutes(r, svc, validator)

	log.Printf("settings api listening on :%s (schema v%d)", port, settings.CurrentVersion)
	if err := r.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
