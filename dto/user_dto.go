package dto

import "time"

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=6,max=64"`
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=6,max=64"`
	Nickname string `json:"nickname" binding:"omitempty,max=64"`
	Email    string `json:"email" binding:"omitempty,email,max=128"`
}

// CheckAccountRequest 账号校验请求
type CheckAccountRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
}

// UserResponse 用户基础信息响应
type UserResponse struct {
	ID          uint       `json:"id"`
	Username    string     `json:"username"`
	Nickname    string     `json:"nickname"`
	Email       string     `json:"email"`
	Status      int        `json:"status"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	SessionID string       `json:"sessionId"`
	ExpireAt  time.Time    `json:"expireAt"`
	User      UserResponse `json:"user"`
}

// CheckAccountResponse 账号校验响应
type CheckAccountResponse struct {
	Username  string `json:"username"`
	Available bool   `json:"available"`
}

// CheckSessionRequest 会话校验请求
type CheckSessionRequest struct {
	SessionID string `json:"sessionId" binding:"required"`
}

// CheckSessionResponse 会话校验响应
type CheckSessionResponse struct {
	Valid    bool       `json:"valid"`
	Username string     `json:"username,omitempty"`
	UserID   uint       `json:"userId,omitempty"`
	ExpireAt *time.Time `json:"expireAt,omitempty"`
}

// LogoutRequest 注销请求
type LogoutRequest struct {
	SessionID string `json:"sessionId" binding:"required"`
}
