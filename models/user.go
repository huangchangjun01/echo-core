package models

import (
	"time"
)

// User 用户实体
type User struct {
	ID           uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:用户ID"`
	Username     string    `json:"username" gorm:"column:username;size:64;uniqueIndex;not null;comment:用户账号"`
	PasswordHash string    `json:"-" gorm:"column:password_hash;size:128;not null;comment:密码哈希"`
	Salt         string    `json:"-" gorm:"column:salt;size:32;not null;comment:密码盐值"`
	Nickname     string    `json:"nickname" gorm:"column:nickname;size:64;comment:用户昵称"`
	Email        string    `json:"email" gorm:"column:email;size:128;index;comment:邮箱"`
	Status       int       `json:"status" gorm:"column:status;default:1;comment:1-正常，2-禁用"`
	LastLoginAt  *time.Time `json:"last_login_at" gorm:"column:last_login_at;comment:最近登录时间"`
	LastLoginIP  string    `json:"last_login_ip" gorm:"column:last_login_ip;size:64;comment:最近登录IP"`
	CreatedAt    time.Time `json:"created_at" gorm:"column:created_at;autoCreateTime;comment:创建时间"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"column:updated_at;autoUpdateTime;comment:更新时间"`
}

// TableName 指定表名
func (User) TableName() string {
	return "user"
}

// IsEnabled 判断用户是否可用
func (u *User) IsEnabled() bool {
	return u.Status == 1
}
