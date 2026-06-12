package model

import "time"

// Server — созданный через бота VDS на RuVDS.
type Server struct {
	ID              uint   `gorm:"primaryKey"`
	VirtualServerID int    `gorm:"uniqueIndex;not null"` // id из RuVDS
	Datacenter      int    `gorm:"not null"`
	Password        string `gorm:"not null"`
	ComputerName    string

	CreatedAt time.Time
	DeletedAt *time.Time `gorm:"index"` // помечается при удалении из бота

	IPs []IP `gorm:"foreignKey:ServerID;constraint:OnDelete:CASCADE"`
}

// IP — IPv4 адрес, привязанный к серверу, с результатом первичной проверки.
type IP struct {
	ID       uint   `gorm:"primaryKey"`
	ServerID uint   `gorm:"index;not null"`
	Address  string `gorm:"not null"`
	Alive    bool
	Port     int // порт, ответивший на TCP probe (0 если никто не ответил)
}
