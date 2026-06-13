package service

import (
	"time"

	"gorm.io/gorm"

	"modul/internal/model"
)

// Repository — обёртка над БД для серверов и их IP.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Save сохраняет сервер вместе со всеми его IP (одной транзакцией).
func (r *Repository) Save(s *model.Server) error {
	return r.db.Create(s).Error
}

// GetByVirtualServerID возвращает сервер по id из RuVDS.
func (r *Repository) GetByVirtualServerID(vsID int) (*model.Server, error) {
	var s model.Server
	err := r.db.Preload("IPs").Where("virtual_server_id = ?", vsID).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListActive возвращает все не помеченные удалёнными серверы.
func (r *Repository) ListActive() ([]model.Server, error) {
	var out []model.Server
	err := r.db.Preload("IPs").Where("deleted_at IS NULL").Order("created_at DESC").Find(&out).Error
	return out, err
}

// ListAll возвращает всю историю — и активные, и помеченные удалёнными.
func (r *Repository) ListAll(limit int) ([]model.Server, error) {
	var out []model.Server
	q := r.db.Preload("IPs").Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&out).Error
	return out, err
}

// MarkDeleted ставит метку удаления (фактической записи не удаляем — нужна история).
func (r *Repository) MarkDeleted(vsID int) error {
	now := time.Now()
	return r.db.Model(&model.Server{}).
		Where("virtual_server_id = ?", vsID).
		Update("deleted_at", &now).Error
}
