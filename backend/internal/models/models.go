package models

import (
	"time"
)

type Exit struct {
	ID         uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Name       string    `json:"name" gorm:"uniqueIndex;not null"`
	Runtime    string    `json:"runtime" gorm:"default:'sing-box'"`
	Protocol   string    `json:"protocol"` // wireguard / openvpn
	Provider   string    `json:"provider"`
	Country    string    `json:"country"`
	City       string    `json:"city"`
	Status     string    `json:"status" gorm:"default:'stopped'"` // running/stopped/unhealthy
	PublicIP   string    `json:"public_ip"`
	Enabled    bool      `json:"enabled" gorm:"default:true"`
	ConfigJSON string    `json:"config_json,omitempty" gorm:"type:jsonb"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Rule struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Pattern   string    `json:"pattern" gorm:"not null"`
	Type      string    `json:"type" gorm:"not null"` // exact / suffix
	ExitID    *uint     `json:"exit_id"`
	Exit      *Exit     `json:"exit,omitempty" gorm:"foreignKey:ExitID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	Priority  int       `json:"priority" gorm:"default:100"`
	Source    string    `json:"source" gorm:"default:'manual'"` // manual / subscription
	SubID     *uint     `json:"sub_id,omitempty"`
	Enabled   bool      `json:"enabled" gorm:"default:true"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Subscription struct {
	ID              uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	Name            string     `json:"name" gorm:"not null"`
	URL             string     `json:"url" gorm:"not null"`
	ExitID          *uint      `json:"exit_id"`
	Exit            *Exit      `json:"exit,omitempty" gorm:"foreignKey:ExitID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	Enabled         bool       `json:"enabled" gorm:"default:true"`
	IntervalMinutes int        `json:"interval_minutes" gorm:"default:360"`
	LastSyncAt      *time.Time `json:"last_sync_at"`
	LastSyncStatus  string     `json:"last_sync_status"`
	LastError       string     `json:"last_error"`
	RuleCount       int        `json:"rule_count" gorm:"-"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type PublishHistory struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Version   string    `json:"version"`
	Status    string    `json:"status"` // success/failed/rollback
	Message   string    `json:"message"`
	Config    string    `json:"config,omitempty" gorm:"type:text"`
	CreatedAt time.Time `json:"created_at"`
}

type ExitRuntimeEvent struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	ExitID    uint      `json:"exit_id"`
	Exit      *Exit     `json:"exit,omitempty" gorm:"foreignKey:ExitID"`
	Action    string    `json:"action"` // create/start/stop/restart/delete
	Status    string    `json:"status"` // success/failed
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
