package handlers

import (
	"echo-core/dto"
	"echo-core/middleware"
	"echo-core/service"
	"echo-core/utils"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RoleCoreHandler 角色内核(role_core)HTTP 适配层。
//
// 设计要点：
//   - 所有外部 API 在此层;service 层负责业务规则,此处只做入参校验 + 错误码映射。
//   - userId 一律从已鉴权 session 取(middleware.MustUserID),不接受请求体 userId。
//   - 错误码映射：service 哨兵 → HTTP code:
//     ErrPersonaNotFound       → 200 + data:null  // 由 GetPersona 单独处理,不走 mapRoleCoreErr
//     ErrTraitsNotFound        → 200 + data:null  // 由 GetTraits 单独处理,不走 mapRoleCoreErr
//     ErrInvalidOceanValue     → 400
//     ErrInvalidOceanDelta     → 400
//     ErrMoodNotFound          → 404
//     ErrRelationshipNotFound  → 404
//     ErrBeliefNotFound        → 404
//     ErrSuggestionNotFound    → 404
//     ErrInvalidSuggestionOp   → 400
type RoleCoreHandler struct {
	svc *service.RoleCoreService
}

// NewRoleCoreHandler 构造
func NewRoleCoreHandler() *RoleCoreHandler {
	return &RoleCoreHandler{svc: service.NewRoleCoreService()}
}

// requireUser 统一取 session userId
func (h *RoleCoreHandler) requireUser(c *gin.Context) (uint, bool) {
	uidStr, ok := middleware.MustUserID(c)
	if !ok || uidStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return 0, false
	}
	uid, err := strconv.ParseUint(uidStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "userId 无效"})
		return 0, false
	}
	return uint(uid), true
}

// mapRoleCoreErr service 哨兵 → HTTP 状态码 + 友好消息
//
// 注意：ErrPersonaNotFound / ErrTraitsNotFound 由 GetPersona / GetTraits 自行处理
// （返回 200 + data:null 以便前端走空态 UI），不在此函数中映射。
func mapRoleCoreErr(c *gin.Context, err error, defaultMsg string) {
	switch {
	case errors.Is(err, service.ErrMoodNotFound),
		errors.Is(err, service.ErrRelationshipNotFound),
		errors.Is(err, service.ErrBeliefNotFound),
		errors.Is(err, service.ErrSuggestionNotFound),
		errors.Is(err, service.ErrPresetNotFound):
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": err.Error()})
	case errors.Is(err, service.ErrInvalidOceanValue),
		errors.Is(err, service.ErrInvalidOceanDelta),
		errors.Is(err, service.ErrInvalidSuggestionOp),
		errors.Is(err, service.ErrPresetProtected),
		errors.Is(err, service.ErrPresetKeyConflict),
		errors.Is(err, service.ErrPresetInvalidKey):
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
	default:
		if defaultMsg == "" {
			defaultMsg = err.Error()
		}
		utils.LogWith(c, "RoleCore", "err=%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": defaultMsg})
	}
}

// ===== 人格（Persona）=====

// SavePersona 保存/更新人格 (POST /api/role-core/persona)
func (h *RoleCoreHandler) SavePersona(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	var req dto.SavePersonaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	resp, err := h.svc.SavePersona(ctx, userId, &req)
	if err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	utils.LogWith(c, "RoleCore", "SavePersona 成功 | roleId=%d", req.RoleID)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": resp})
}

// GetPersona 取人格 (GET /api/role-core/persona?roleId=N)
//
// 设计要点：人格尚未填写属于"正常业务态"，返回 HTTP 200 + data:null，
// 前端按空态 UI（"新建人格"按钮）展示，而不是把 404 当成错误弹出。
func (h *RoleCoreHandler) GetPersona(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	roleId, err := strconv.ParseUint(c.Query("roleId"), 10, 64)
	if err != nil || roleId == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "roleId 必填"})
		return
	}
	resp, err := h.svc.GetPersona(ctx, userId, uint(roleId))
	if err != nil {
		if errors.Is(err, service.ErrPersonaNotFound) {
			utils.LogWith(c, "RoleCore", "GetPersona 未填写 | roleId=%d", roleId)
			c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": nil})
			return
		}
		mapRoleCoreErr(c, err, "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": resp})
}

// ===== OCEAN 五维（Traits）=====

// SaveTraits 保存 OCEAN (POST /api/role-core/traits)
func (h *RoleCoreHandler) SaveTraits(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	var req dto.SaveTraitsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	resp, err := h.svc.SaveTraits(ctx, userId, &req)
	if err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	utils.LogWith(c, "RoleCore", "SaveTraits 成功 | roleId=%d preset=%s", req.RoleID, req.PresetType)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": resp})
}

// SetActivePreset 切换当前使用的 OCEAN 预设 (POST /api/role-core/active-preset)
//
// 与 SaveTraits 的职责拆分:
//   - SaveTraits:写入 OCEAN 五维(POST /traits)
//   - SetActivePreset:仅切换 active preset,不动 OCEAN 数据
//
// 前端对应「保存五维性格配置」+「使用性格」两个独立按钮。
//
// 失败:
//   - 400: 参数错误(binding 校验失败)
//   - 404: presetType 不存在(ErrPresetNotFound)
//   - 500: 写库失败
func (h *RoleCoreHandler) SetActivePreset(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	var req dto.SetActivePresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	active, err := h.svc.SetActivePreset(ctx, userId, req.RoleID, req.PresetType)
	if err != nil {
		// 复用 SaveTraits 的 preset-not-found 错误映射(语义相同)
		mapRoleCoreErr(c, err, "")
		return
	}
	utils.LogWith(c, "RoleCore", "SetActivePreset 成功 | roleId=%d preset=%s", req.RoleID, active)
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    dto.SetActivePresetResponse{ActivePresetType: active},
	})
}

// GetTraits 取 OCEAN (GET /api/role-core/traits?roleId=N[&presetType=xxx])
//
// 返回 dto.TraitsListResponse{List, ActivePresetType}:
//   - List: 该 (user, role) 下所有已保存预设的 OCEAN 五维
//   - ActivePresetType: 当前真正生效的预设 key
//
// 可选 query presetType:若指定则保持旧行为(单元素数组 + activePresetType),
// 便于调试面板单独查询某个 preset。
//
// 设计要点：OCEAN 五维尚未填写属于"正常业务态"，返回 HTTP 200 + data 完整结构体
// (list=[], activePresetType=companion_default)，前端按空态 UI 展示。
func (h *RoleCoreHandler) GetTraits(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	roleId, err := strconv.ParseUint(c.Query("roleId"), 10, 64)
	if err != nil || roleId == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "roleId 必填"})
		return
	}
	presetType := c.Query("presetType")

	// 指定 presetType → 单元素数组(保持旧行为,便于调试面板单独查)
	if presetType != "" {
		single, gerr := h.svc.GetTraitsByPreset(ctx, userId, uint(roleId), presetType)
		if gerr != nil {
			if errors.Is(gerr, service.ErrTraitsNotFound) {
				utils.LogWith(c, "RoleCore", "GetTraits 未填写 preset=%s | roleId=%d", presetType, roleId)
				c.JSON(http.StatusOK, gin.H{
					"code":    200,
					"message": "ok",
					"data": dto.TraitsListResponse{
						List:             []dto.TraitsResponse{},
						ActivePresetType: "", // 单 preset 查询时无意义,留空
					},
				})
				return
			}
			mapRoleCoreErr(c, gerr, "")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "ok",
			"data": dto.TraitsListResponse{
				List:             []dto.TraitsResponse{*single},
				ActivePresetType: "",
			},
		})
		return
	}

	resp, err := h.svc.GetTraits(ctx, userId, uint(roleId))
	if err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	if resp == nil {
		resp = &dto.TraitsListResponse{List: []dto.TraitsResponse{}, ActivePresetType: "companion_default"}
	}
	utils.LogWith(c, "RoleCore", "GetTraits 成功 | roleId=%d count=%d active=%s",
		roleId, len(resp.List), resp.ActivePresetType)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": resp})
}

// ===== Mood（M2 占位）=====

// GetMood 取心情快照 (GET /api/role-core/mood?roleId=N)
func (h *RoleCoreHandler) GetMood(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	roleId, err := strconv.ParseUint(c.Query("roleId"), 10, 64)
	if err != nil || roleId == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "roleId 必填"})
		return
	}
	resp, err := h.svc.GetMood(ctx, userId, uint(roleId))
	if err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": resp})
}

// UpdateMood 上报 mood事件 (POST /api/role-core/mood/event) — 内部接口
func (h *RoleCoreHandler) UpdateMood(c *gin.Context) {
	ctx := c.Request.Context()
	// 内部 token 鉴权
	if h.requireInternalToken(c) == false {
		return
	}
	var req dto.UpdateMoodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	resp, err := h.svc.UpdateMood(ctx, &req)
	if err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": resp})
}

// ===== Relationship（M3 占位）=====

// GetRelationship 取关系值 (GET /api/role-core/relationship?roleId=N)
func (h *RoleCoreHandler) GetRelationship(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	roleId, err := strconv.ParseUint(c.Query("roleId"), 10, 64)
	if err != nil || roleId == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "roleId 必填"})
		return
	}
	resp, err := h.svc.GetRelationship(ctx, userId, uint(roleId))
	if err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": resp})
}

// AccumulateRelationship 上报关系事件 (POST /api/role-core/relationship/event)
//
// 内部接口:userId 由 body 传入,不依赖 session。
func (h *RoleCoreHandler) AccumulateRelationship(c *gin.Context) {
	ctx := c.Request.Context()
	if h.requireInternalToken(c) == false {
		return
	}
	var req dto.RelationshipEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	resp, err := h.svc.AccumulateRelationship(ctx, req.UserID, &req)
	if err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	utils.LogWith(c, "RoleCore", "AccumulateRelationship 成功 | userId=%d roleId=%d event=%s",
		req.UserID, req.RoleID, req.EventType)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": resp})
}

// ===== Belief（M3 占位）=====

// ListBeliefs 列出已确认信念 (GET /api/role-core/beliefs?roleId=N)
func (h *RoleCoreHandler) ListBeliefs(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	roleId, err := strconv.ParseUint(c.Query("roleId"), 10, 64)
	if err != nil || roleId == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "roleId 必填"})
		return
	}
	resp, err := h.svc.ListBeliefs(ctx, userId, uint(roleId))
	if err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": resp})
}

// CreateBelief 新增信念 (POST /api/role-core/beliefs)
func (h *RoleCoreHandler) CreateBelief(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	var req dto.CreateBeliefRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	resp, err := h.svc.CreateBelief(ctx, userId, &req)
	if err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	utils.LogWith(c, "RoleCore", "CreateBelief 成功 | roleId=%d topic=%s", req.RoleID, req.Topic)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": resp})
}

// DeleteBelief 删除信念 (DELETE /api/role-core/beliefs/:id)
func (h *RoleCoreHandler) DeleteBelief(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "id 必填"})
		return
	}
	if err := h.svc.DeleteBelief(ctx, userId, uint(id)); err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	utils.LogWith(c, "RoleCore", "DeleteBelief 成功 | id=%d", id)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok"})
}

// UpdateBelief 修改信念 (PUT /api/role-core/beliefs/:id)
//
// 业务字段(topic/stance/confidence/source)全部可改。
// evidence_count / user_confirmed 由反馈/确认逻辑内部维护,不在此暴露。
func (h *RoleCoreHandler) UpdateBelief(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "id 必填"})
		return
	}
	var req dto.UpdateBeliefRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	resp, err := h.svc.UpdateBelief(ctx, userId, uint(id), &req)
	if err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	utils.LogWith(c, "RoleCore", "UpdateBelief 成功 | id=%d topic=%s", id, req.Topic)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": resp})
}

// ===== Suggestion（M6 占位）=====

// ListPendingSuggestions 列出 pending 建议 (GET /api/role-core/suggestions?roleId=N)
func (h *RoleCoreHandler) ListPendingSuggestions(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	roleId, err := strconv.ParseUint(c.Query("roleId"), 10, 64)
	if err != nil || roleId == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "roleId 必填"})
		return
	}
	resp, err := h.svc.ListPendingSuggestions(ctx, userId, uint(roleId))
	if err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": resp})
}

// CreateSuggestion 内部接口：echo-ai LLM 抽取后回写 (POST /api/role-core/suggestions)
func (h *RoleCoreHandler) CreateSuggestion(c *gin.Context) {
	ctx := c.Request.Context()
	if h.requireInternalToken(c) == false {
		return
	}
	var req dto.CreateSuggestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	resp, err := h.svc.CreateSuggestion(ctx, &req)
	if err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": resp})
}

// ResolveSuggestion 用户处理建议 (POST /api/role-core/suggestions/:id/:action)
// action: apply / skip / edit
func (h *RoleCoreHandler) ResolveSuggestion(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "id 必填"})
		return
	}
	action := c.Param("action")
	var req dto.ResolveSuggestionRequest
	if action == "edit" {
		_ = c.ShouldBindJSON(&req)
	}
	if err := h.svc.ResolveSuggestion(ctx, userId, uint(id), action, &req); err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	utils.LogWith(c, "RoleCore", "ResolveSuggestion 成功 | id=%d action=%s", id, action)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok"})
}

// ===== Feedback（M7 占位）=====

// SubmitFeedback 提交反馈 (POST /api/role-core/feedback)
func (h *RoleCoreHandler) SubmitFeedback(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	var req dto.SubmitFeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	if err := h.svc.SubmitFeedback(ctx, userId, &req); err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	utils.LogWith(c, "RoleCore", "SubmitFeedback 成功 | roleId=%d rating=%d", req.RoleID, req.Rating)

	// 反馈提交后 → 触发关系值累积(positive/negative_feedback 事件)。
	// OnFeedback 内部按 rating 分流:>0 → positive,<0 → negative,=0 → 跳过。
	// 失败仅记日志,不影响 feedback 主响应。
	h.svc.OnFeedback(ctx, userId, req.RoleID, req.Rating)

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok"})
}

// ===== Core Snapshot(调试面板用) =====

// GetCoreSnapshot 取完整快照 (GET /api/role-core/core?roleId=N)
func (h *RoleCoreHandler) GetCoreSnapshot(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	roleId, err := strconv.ParseUint(c.Query("roleId"), 10, 64)
	if err != nil || roleId == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "roleId 必填"})
		return
	}
	resp, err := h.svc.GetCoreSnapshot(ctx, userId, uint(roleId))
	if err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": resp})
}

// requireInternalToken 内部接口鉴权(可选)。
//
// 若 ECHO_CORE_INTERNAL_TOKEN 已配置,则必须匹配 X-Internal-Token;否则跳过。
func (h *RoleCoreHandler) requireInternalToken(c *gin.Context) bool {
	if utils.GetEnv("ECHO_CORE_INTERNAL_TOKEN", "") == "" {
		return true // 未配置则跳过
	}
	if c.GetHeader("X-Internal-Token") != utils.GetEnv("ECHO_CORE_INTERNAL_TOKEN", "") {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "internal token mismatch"})
		return false
	}
	return true
}

// ===== OCEAN 预设字典 CRUD =====

// ListPresets 列出某 (user, role) 的所有预设 (GET /api/role-core/presets?roleId=N)
//
// 角色隔离后必须传 roleId,首次访问时自动种子默认 6 个(标准陪伴等)。
// companion_default 标记 is_protected,前端据此禁用"删除"按钮。
func (h *RoleCoreHandler) ListPresets(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	roleId, err := strconv.ParseUint(c.Query("roleId"), 10, 64)
	if err != nil || roleId == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "roleId 必填"})
		return
	}
	list, err := h.svc.ListPresets(ctx, userId, uint(roleId))
	if err != nil {
		mapRoleCoreErr(c, err, "获取预设列表失败")
		return
	}
	utils.LogWith(c, "RoleCore", "ListPresets 成功 | roleId=%d count=%d", roleId, len(list))
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": list})
}

// CreatePreset 新增预设 (POST /api/role-core/presets)
//
// 角色隔离后,RoleID 从 body 里取(binding:"required")。
func (h *RoleCoreHandler) CreatePreset(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	var req dto.CreatePresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	resp, err := h.svc.CreatePreset(ctx, userId, req.RoleID, &req)
	if err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	utils.LogWith(c, "RoleCore", "CreatePreset 成功 | roleId=%d key=%s", req.RoleID, req.Key)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": resp})
}

// UpdatePreset 修改预设 (PUT /api/role-core/presets/:id)
//
// 不允许修改 key / isProtected(由前端隐藏对应输入)。
// companion_default 允许修改(用户诉求:"标准陪伴不允许删除只能修改")。
// RoleID 由 body 提供(binding:"required"),与 DB 行的 roleId 比对防越权。
func (h *RoleCoreHandler) UpdatePreset(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "id 必填"})
		return
	}
	var req dto.UpdatePresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	resp, err := h.svc.UpdatePreset(ctx, userId, req.RoleID, uint(id), &req)
	if err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	utils.LogWith(c, "RoleCore", "UpdatePreset 成功 | roleId=%d id=%d", req.RoleID, id)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": resp})
}

// DeletePreset 删除预设 (DELETE /api/role-core/presets/:id?roleId=N)
//
// is_protected=true 拒绝(service 返回 ErrPresetProtected → 400)。
// roleId 必填 —— 角色隔离后,删预设时必须明确「删的是哪个角色的」,
//
//	保证不会因为 path 里的 id 撞到其它角色而被误删。
func (h *RoleCoreHandler) DeletePreset(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "id 必填"})
		return
	}
	roleId, err := strconv.ParseUint(c.Query("roleId"), 10, 64)
	if err != nil || roleId == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "roleId 必填"})
		return
	}
	if err := h.svc.DeletePreset(ctx, userId, uint(roleId), uint(id)); err != nil {
		mapRoleCoreErr(c, err, "")
		return
	}
	utils.LogWith(c, "RoleCore", "DeletePreset 成功 | roleId=%d id=%d", roleId, id)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok"})
}
