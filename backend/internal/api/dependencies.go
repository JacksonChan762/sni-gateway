package api

import (
	"sni-gateway/internal/models"

	"gorm.io/gorm"
)

type exitDependencySummary struct {
	ActiveRuleCount           int64 `json:"active_rule_count"`
	InactiveRuleCount         int64 `json:"inactive_rule_count"`
	ActiveSubscriptionCount   int64 `json:"active_subscription_count"`
	InactiveSubscriptionCount int64 `json:"inactive_subscription_count"`
}

func ensureExitExists(tx *gorm.DB, exitID *uint) error {
	if exitID == nil {
		return nil
	}

	var count int64
	if err := tx.Model(&models.Exit{}).Where("id = ?", *exitID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

func getExitDependencySummary(tx *gorm.DB, exitID uint) (exitDependencySummary, error) {
	summary := exitDependencySummary{}

	if err := tx.Model(&models.Rule{}).Where("exit_id = ? AND enabled = true", exitID).Count(&summary.ActiveRuleCount).Error; err != nil {
		return summary, err
	}
	if err := tx.Model(&models.Rule{}).Where("exit_id = ? AND enabled = false", exitID).Count(&summary.InactiveRuleCount).Error; err != nil {
		return summary, err
	}
	if err := tx.Model(&models.Subscription{}).Where("exit_id = ? AND enabled = true", exitID).Count(&summary.ActiveSubscriptionCount).Error; err != nil {
		return summary, err
	}
	if err := tx.Model(&models.Subscription{}).Where("exit_id = ? AND enabled = false", exitID).Count(&summary.InactiveSubscriptionCount).Error; err != nil {
		return summary, err
	}

	return summary, nil
}

func exitHasActiveDependencies(summary exitDependencySummary) bool {
	return summary.ActiveRuleCount > 0 || summary.ActiveSubscriptionCount > 0
}
