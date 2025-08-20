package models

import "time"

type UserRow struct {
	ID           uint64     `json:"id" db:"id"`
	Email        string     `json:"email" db:"email" validate:"required,email"`
	PasswordHash string     `json:"-" db:"password_hash" validate:"required,min=60"` // Never return in JSON
	Name         string     `json:"name" db:"name" validate:"required,min=2,max=50"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty" db:"last_login_at"`
	LastActiveAt *time.Time `json:"last_active_at,omitempty" db:"last_active_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
}

type ChatRow struct {
	ID           uint64    `json:"id" db:"id"`
	Name         string    `json:"name" db:"name" validate:"required,max=100"`
	Description  string    `json:"description" db:"description" validate:"required,max=500"`
	IsGroup      bool      `json:"is_group" db:"is_group"`
	IsPrivate    bool      `json:"is_private" db:"is_private"`
	IsInviteOnly bool      `json:"is_invite_only" db:"is_invite_only"`
	CreatedBy    uint64    `json:"created_by" db:"created_by" validate:"required,gt=0"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

type ChatMemberRow struct {
	UserID            uint64    `json:"user_id" db:"user_id" validate:"required,gt=0"`
	ChatID            uint64    `json:"chat_id" db:"chat_id" validate:"required,gt=0"`
	Role              string    `json:"role" db:"role" validate:"oneof=admin member guest"`
	CanAddUsers       bool      `json:"can_add_users" db:"can_add_users"`
	LastReadMessageID *uint64   `json:"last_read_message_id,omitempty" db:"last_read_message_id" validate:"omitempty,gte=0"`
	JoinedAt          time.Time `json:"joined_at" db:"joined_at"`
}

type MessageRow struct {
	ID        uint64     `json:"id" db:"id"`
	ChatID    uint64     `json:"chat_id" db:"chat_id" validate:"required,gt=0"`
	UserID    uint64     `json:"user_id" db:"user_id" validate:"required,gt=0"`
	Content   string     `json:"content" db:"content" validate:"required,max=2000"`
	Status    string     `json:"status" db:"status" validate:"oneof=active edited"`
	EditedAt  *time.Time `json:"edited_at,omitempty" db:"edited_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	SentAt    time.Time  `json:"sent_at" db:"sent_at"`
}

type ReadReceiptRow struct {
	ID        uint64    `json:"id" db:"id"`
	MessageID uint64    `json:"message_id" db:"message_id" validate:"required,gt=0"`
	UserID    uint64    `json:"user_id" db:"user_id" validate:"required,gt=0"`
	ReadAt    time.Time `json:"read_at" db:"read_at"`
}

type AuditLogRow struct {
	ID         uint64    `json:"id" db:"id"`
	UserID     uint64    `json:"user_id" db:"user_id" validate:"required,gt=0"`
	Action     string    `json:"action" db:"action" validate:"oneof=message_edited chat_created chat_updated chat_joined chat_left"`
	EntityType string    `json:"entity_type" db:"entity_type" validate:"oneof=chat message chat_member"`
	EntityID   uint64    `json:"entity_id" db:"entity_id" validate:"required,gt=0"`
	OldValue   *string   `json:"old_value,omitempty" db:"old_value" validate:"omitempty,max=1000"`
	NewValue   *string   `json:"new_value,omitempty" db:"new_value" validate:"omitempty,max=1000"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}
