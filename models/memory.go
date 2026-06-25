package models

import (
	"time"
)

// SessionMessage 会话消息 - 存储会话级别的对话历史
type SessionMessage struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	SessionID  string    `json:"session_id" gorm:"index;size:64;not null"`
	UserID     string    `json:"user_id" gorm:"index;size:64;not null"`
	Role       string    `json:"role" gorm:"size:20;not null"` // user/assistant
	Content    string    `json:"content" gorm:"type:text"`
	CreatedAt  time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (SessionMessage) TableName() string {
	return "session_message"
}