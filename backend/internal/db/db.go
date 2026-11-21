package db

import (
	"log"
	"sni-gateway/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Init(dsn string) error {
	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.New(log.Default(), logger.Config{LogLevel: logger.Warn, ParameterizedQueries: true}),
	})
	if err != nil {
		return err
	}

	log.Println("Database connected. Running migrations...")
	err = DB.AutoMigrate(
		&models.Exit{},
		&models.RuleFolder{},
		&models.Rule{},
		&models.Subscription{},
		&models.PublishHistory{},
		&models.ExitRuntimeEvent{},
	)
	if err != nil {
		return err
	}
	log.Println("Migrations complete.")
	return nil
}
