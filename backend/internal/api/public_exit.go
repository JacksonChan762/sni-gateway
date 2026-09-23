package api

import "gorm.io/gorm"

// publicExit keeps saved VPN credentials out of rule and subscription responses.
func publicExit(tx *gorm.DB) *gorm.DB {
	return tx.Omit("config_json")
}
