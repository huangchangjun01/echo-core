package dto

import "time"

// CreateRoleRequest 创建角色请求
type CreateRoleRequest struct {
	Name string `json:"name" binding:"required,min=1,max=64"`
	Desc string `json:"desc" binding:"omitempty,max=500"`
}

// UpdateRoleRequest 更新角色请求
type UpdateRoleRequest struct {
	Name string `json:"name" binding:"omitempty,min=1,max=64"`
	Desc string `json:"desc" binding:"omitempty,max=500"`
}

// RoleResponse 角色响应
type RoleResponse struct {
	ID        uint      `json:"id"`
	UserID    string    `json:"userId"`
	Name      string    `json:"name"`
	Desc      string    `json:"desc"`
	Status    int       `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
