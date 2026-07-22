package service

import (
	"context"
	"echo-core/dto"
	"echo-core/models"
	"echo-core/repository"
	"echo-core/utils"
	"errors"
	"strings"
)

// 默认角色兜底名称（按用户 ID 区分，每个用户首次进入都会拿到一个"默认角色"）
const (
	DefaultRoleName = "默认角色"
	DefaultRoleDesc = "系统自动创建，用于老数据与首次进入时无缝使用"
)

// 业务错误
var (
	ErrRoleExists   = errors.New("角色名已被占用")
	ErrRoleNotFound = errors.New("角色不存在")
)

// RoleService 角色业务服务
type RoleService struct {
	repo *repository.RoleRepository
}

// NewRoleService 构造 RoleService
func NewRoleService() *RoleService {
	return &RoleService{repo: repository.NewRoleRepository()}
}

// Create 新建角色。同用户下名字唯一。
func (s *RoleService) Create(ctx context.Context, userId string, req dto.CreateRoleRequest) (*dto.RoleResponse, error) {
	name := strings.TrimSpace(req.Name)
	utils.LogWithCtx(ctx, "RoleService.Create", "入参 | userId=%s name=%s descLen=%d", userId, name, len(req.Desc))
	if userId == "" {
		return nil, errors.New("userId is required")
	}
	if name == "" {
		return nil, errors.New("name is required")
	}

	exists, err := s.repo.ExistsByUserIDName(ctx, userId, name)
	if err != nil {
		utils.LogWithCtx(ctx, "RoleService.Create", "同名检查失败 | userId=%s err=%v", userId, err)
		return nil, err
	}
	if exists {
		utils.LogWithCtx(ctx, "RoleService.Create", "重名 | userId=%s name=%s", userId, name)
		return nil, ErrRoleExists
	}

	role := &models.Role{
		UserId: userId,
		Name:   name,
		Desc:   strings.TrimSpace(req.Desc),
		Status: 1,
	}
	if err := s.repo.Create(ctx, role); err != nil {
		utils.LogWithCtx(ctx, "RoleService.Create", "创建失败 | userId=%s err=%v", userId, err)
		return nil, err
	}
	utils.LogWithCtx(ctx, "RoleService.Create", "创建成功 | userId=%s id=%d name=%s", userId, role.ID, role.Name)
	return s.toResponse(role), nil
}

// List 列出角色。若该用户无任何角色，自动创建一个"默认角色"再返回。
// 设计意图：老数据/老用户首次进入时无缝使用，避免前端进入后台一片空白。
func (s *RoleService) List(ctx context.Context, userId string) ([]dto.RoleResponse, error) {
	utils.LogWithCtx(ctx, "RoleService.List", "入参 | userId=%s", userId)
	if userId == "" {
		return nil, errors.New("userId is required")
	}

	roles, err := s.repo.GetList(ctx, userId)
	if err != nil {
		utils.LogWithCtx(ctx, "RoleService.List", "查询失败 | userId=%s err=%v", userId, err)
		return nil, err
	}

	if len(roles) == 0 {
		utils.LogWithCtx(ctx, "RoleService.List", "无角色，自动创建默认角色 | userId=%s", userId)
		defaultRole := &models.Role{
			UserId: userId,
			Name:   DefaultRoleName,
			Desc:   DefaultRoleDesc,
			Status: 1,
		}
		if err := s.repo.Create(ctx, defaultRole); err != nil {
			utils.LogWithCtx(ctx, "RoleService.List", "默认角色创建失败 | userId=%s err=%v", userId, err)
			return nil, err
		}
		roles = append(roles, *defaultRole)
	}

	out := make([]dto.RoleResponse, 0, len(roles))
	for i := range roles {
		out = append(out, *s.toResponse(&roles[i]))
	}
	utils.LogWithCtx(ctx, "RoleService.List", "完成 | userId=%s count=%d", userId, len(out))
	return out, nil
}

// Update 更新角色（名称/描述）。同用户下改名仍须保持唯一。
func (s *RoleService) Update(ctx context.Context, userId string, id uint, req dto.UpdateRoleRequest) (*dto.RoleResponse, error) {
	utils.LogWithCtx(ctx, "RoleService.Update", "入参 | userId=%s id=%d nameSet=%v descSet=%v",
		userId, id, req.Name != "", req.Desc != "")
	if userId == "" {
		return nil, errors.New("userId is required")
	}
	role, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrRoleNotFound
	}
	if role.UserId != userId {
		// 防止越权：别人的角色不能改
		return nil, ErrRoleNotFound
	}

	updates := map[string]interface{}{}
	if name := strings.TrimSpace(req.Name); name != "" && name != role.Name {
		exists, err := s.repo.ExistsByUserIDName(ctx, userId, name)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, ErrRoleExists
		}
		updates["name"] = name
	}
	if req.Desc != "" || (req.Desc == "" && role.Desc != "") {
		// 允许把 desc 改为空字符串。区分"未传"与"传了空串"不必要——前端不会主动清空，
		// 这里只在 desc 字段出现在请求体里时落库即可（此处 binding 已校验 max=500）。
		updates["desc"] = strings.TrimSpace(req.Desc)
	}

	if len(updates) > 0 {
		if err := s.repo.Update(ctx, id, updates); err != nil {
			utils.LogWithCtx(ctx, "RoleService.Update", "更新失败 | id=%d err=%v", id, err)
			return nil, err
		}
	}
	updated, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrRoleNotFound
	}
	utils.LogWithCtx(ctx, "RoleService.Update", "更新成功 | id=%d", id)
	return s.toResponse(updated), nil
}

// Delete 软删除角色。同用户下至少保留 1 个角色（不让前端出现"无角色"）。
func (s *RoleService) Delete(ctx context.Context, userId string, id uint) error {
	utils.LogWithCtx(ctx, "RoleService.Delete", "入参 | userId=%s id=%d", userId, id)
	if userId == "" {
		return errors.New("userId is required")
	}
	role, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return ErrRoleNotFound
	}
	if role.UserId != userId {
		return ErrRoleNotFound
	}
	count, err := s.repo.CountByUserID(ctx, userId)
	if err != nil {
		return err
	}
	if count <= 1 {
		return errors.New("至少需要保留一个角色")
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		utils.LogWithCtx(ctx, "RoleService.Delete", "删除失败 | id=%d err=%v", id, err)
		return err
	}
	utils.LogWithCtx(ctx, "RoleService.Delete", "删除成功 | id=%d", id)
	return nil
}

// toResponse 实体转响应 DTO
func (s *RoleService) toResponse(r *models.Role) *dto.RoleResponse {
	if r == nil {
		return nil
	}
	return &dto.RoleResponse{
		ID:        r.ID,
		UserID:    r.UserId,
		Name:      r.Name,
		Desc:      r.Desc,
		Status:    r.Status,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}
