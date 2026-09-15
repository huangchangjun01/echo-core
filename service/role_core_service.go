package service

import (
	"context"
	"echo-core/dto"
	"echo-core/models"
	"echo-core/repository"
	"echo-core/utils"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"
)

// Role Core 业务错误哨兵变量
var (
	ErrPersonaNotFound      = errors.New("人格不存在")
	ErrTraitsNotFound       = errors.New("OCEAN五维不存在")
	ErrInvalidOceanValue    = errors.New("OCEAN五维值越界 [-1,1]")
	ErrInvalidOceanDelta    = errors.New("OCEAN 单步变更超出硬上限 0.05")
	ErrMoodNotFound         = errors.New("心情不存在")
	ErrRelationshipNotFound = errors.New("关系值不存在")
	ErrBeliefNotFound       = errors.New("信念不存在")
	ErrSuggestionNotFound   = errors.New("建议不存在")
	ErrInvalidSuggestionOp  = errors.New("无效的建议操作")
	ErrPersonaJsonCorrupt   = errors.New("persona JSON 字段损坏")
	ErrOceanHardLimitBreach = errors.New("OCEAN 周变化总量超 0.8")
	// 预设字典相关
	ErrPresetNotFound    = errors.New("OCEAN 预设不存在")
	ErrPresetProtected   = errors.New("该预设受保护,不可删除")
	ErrPresetKeyConflict = errors.New("预设 key 已存在")
	ErrPresetInvalidKey  = errors.New("预设 key 非法")
)

// OCEAN 单步变更硬上限（PRD 03_PRD_change_engine.md）
const (
	oceanStepHardLimit = 0.05
	oceanWeekHardLimit = 0.8
)

// RoleCoreService 角色内核(role_core)业务编排。
//
// 职责：
//  1. 人格 persona + OCEAN traits 的读写（乐观锁 / 范围校验 / 硬上限 / 审计）
//  2. 三桶心情 mood 的写入 + 衰减（M2 占位接口,M0 先建好桩）
//  3. 三维关系值 relationship 的累积（M3 占位）
//  4. 信念 belief CRUD（M3 占位）
//  5. 反馈 feedback 写入 + 累积触发（M7 占位）
//  6. 演化建议 suggestion 流转：pending → applied/skipped/edited（M6 占位）
type RoleCoreService struct {
	personaRepo  *repository.RolePersonaRepository
	traitsRepo   *repository.RoleTraitsRepository
	moodRepo     *repository.RoleMoodRepository
	relRepo      *repository.RoleRelationshipRepository
	beliefRepo   *repository.RoleBeliefRepository
	auditRepo    *repository.RoleCoreAuditLogRepository
	feedbackRepo *repository.RoleFeedbackRepository
	suggRepo     *repository.RoleEvolutionSuggestionRepository
	presetRepo   *repository.RoleOceanPresetRepository
	// activePresetRepo 当前生效预设的持久化(每角色一条)。
	// 取代早期硬编码「取 companion_default」,让 chat 真正按用户选择读 OCEAN。
	activePresetRepo *repository.RoleActivePresetRepository
}

// NewRoleCoreService 构造
func NewRoleCoreService() *RoleCoreService {
	return &RoleCoreService{
		personaRepo:      repository.NewRolePersonaRepository(),
		traitsRepo:       repository.NewRoleTraitsRepository(),
		moodRepo:         repository.NewRoleMoodRepository(),
		relRepo:          repository.NewRoleRelationshipRepository(),
		beliefRepo:       repository.NewRoleBeliefRepository(),
		auditRepo:        repository.NewRoleCoreAuditLogRepository(),
		feedbackRepo:     repository.NewRoleFeedbackRepository(),
		suggRepo:         repository.NewRoleEvolutionSuggestionRepository(),
		presetRepo:       repository.NewRoleOceanPresetRepository(),
		activePresetRepo: repository.NewRoleActivePresetRepository(),
	}
}

// ===== 人格（Persona）=====

// SavePersona 保存人格：JSON 序列化 + 乐观锁 + 审计 + 写入。
func (s *RoleCoreService) SavePersona(ctx context.Context, userId uint, req *dto.SavePersonaRequest) (*dto.PersonaResponse, error) {
	if userId == 0 || req.RoleID == 0 {
		return nil, errors.New("userId/roleId required")
	}
	utils.LogWithCtx(ctx, "RoleCoreService.SavePersona",
		"入参 | userId=%d roleId=%d traitsTags=%d values=%d examples=%d",
		userId, req.RoleID, len(req.TraitsTags), len(req.Values), len(req.ExampleDialogs))

	// 1. JSON 序列化数组字段
	traitsTagsJSON, _ := json.Marshal(req.TraitsTags)
	valuesJSON, _ := json.Marshal(req.Values)
	taboosJSON, _ := json.Marshal(req.Taboos)
	examplesJSON, _ := json.Marshal(req.ExampleDialogs)

	// 2. 拉取旧值用于 audit_log
	old, _ := s.personaRepo.GetByUserRole(ctx, userId, req.RoleID)
	beforeJSON := ""
	if old != nil {
		// 反序列化旧数据快照
		oldSnap := struct {
			Identity       string                  `json:"identity"`
			Background     string                  `json:"background"`
			TraitsTags     []string                `json:"traitsTags"`
			Values         []string                `json:"values"`
			SpeakingStyle  string                  `json:"speakingStyle"`
			Taboos         []string                `json:"taboos"`
			ExampleDialogs []dto.ExampleDialogItem `json:"exampleDialogs"`
			Version        int                     `json:"version"`
		}{
			Identity: old.Identity, Background: old.Background, SpeakingStyle: old.SpeakingStyle,
			Version: old.Version,
		}
		_ = json.Unmarshal([]byte(orEmpty(old.TraitsTags)), &oldSnap.TraitsTags)
		_ = json.Unmarshal([]byte(orEmpty(old.Values)), &oldSnap.Values)
		_ = json.Unmarshal([]byte(orEmpty(old.Taboos)), &oldSnap.Taboos)
		_ = json.Unmarshal([]byte(orEmpty(old.ExampleDialogs)), &oldSnap.ExampleDialogs)
		beforeJSON = mustMarshal(oldSnap)
	}

	// 3. 写入（带乐观锁）
	p := &models.RolePersona{
		UserID:         userId,
		RoleID:         req.RoleID,
		Identity:       req.Identity,
		Background:     req.Background,
		TraitsTags:     string(traitsTagsJSON),
		Values:         string(valuesJSON),
		SpeakingStyle:  req.SpeakingStyle,
		Taboos:         string(taboosJSON),
		ExampleDialogs: string(examplesJSON),
	}
	err := s.personaRepo.UpsertWithTx(ctx, nil, p)
	if err != nil {
		utils.LogWithCtx(ctx, "RoleCoreService.SavePersona", "写入失败 | err=%v", err)
		return nil, err
	}

	// 4. 审计日志(best-effort,失败不阻塞主业务)
	afterJSON := mustMarshal(map[string]interface{}{
		"identity":       p.Identity,
		"background":     p.Background,
		"traitsTags":     req.TraitsTags,
		"values":         req.Values,
		"speakingStyle":  p.SpeakingStyle,
		"taboos":         req.Taboos,
		"exampleDialogs": req.ExampleDialogs,
		"version":        p.Version,
	})
	delta := computePersonaDelta(beforeJSON, afterJSON)
	_ = s.auditRepo.Create(ctx, &models.RoleCoreAuditLog{
		UserID:       userId,
		RoleID:       req.RoleID,
		Action:       models.AuditActionUpdate,
		TargetType:   models.AuditTargetPersona,
		BeforeJSON:   beforeJSON,
		AfterJSON:    afterJSON,
		DeltaSummary: delta,
		TriggerEvent: models.AuditTriggerUserEdit,
	})

	return s.GetPersona(ctx, userId, req.RoleID)
}

// GetPersona 取人格(无则返回 ErrPersonaNotFound 哨兵，由 handler 映射为 200 + data:null)
func (s *RoleCoreService) GetPersona(ctx context.Context, userId, roleId uint) (*dto.PersonaResponse, error) {
	p, err := s.personaRepo.GetByUserRole(ctx, userId, roleId)
	if err != nil {
		if err.Error() == "record not found" {
			return nil, ErrPersonaNotFound
		}
		return nil, err
	}
	return personaToResponse(p), nil
}

// ===== OCEAN 五维（Traits）=====

// SaveTraits 保存某预设的 OCEAN 五维：范围校验 + preset 存在性校验 + 软告警 + 审计。
//
// 软告警(不再以硬上限拒绝):
//   - 单步变更不再"硬上限拒绝"。0.05 的限制原本是为防止 LLM bug 一次性把 OCEAN
//     推飞,但当前 OCEAN 唯一的写入路径就是用户主动 SaveTraits —— 用户切换预设
//     是正常业务行为,不该被拦截。
//   - 大变更改为软告警:本次 max_delta 写入响应的 maxDelta/warning 字段,
//     前端弹窗让用户确认意图,但后端不阻断。
//   - 硬上限规则(若需要)留给未来的内部 API,如 LLM-driven 反馈累积。
//
// 范围校验 [-2,2] 与 presetType 存在性校验保留 —— 这两类是真的"非法输入"。
//
// 关键设计:每个 (user, role, preset_type) 独立一行,切预设保存互不影响。
//
// presetType 由 hardcoded oneof 改为 DB 校验:调用 ListPresets 复用种子逻辑,
// 保证首次保存 traits 时预设表已就绪。
func (s *RoleCoreService) SaveTraits(ctx context.Context, userId uint, req *dto.SaveTraitsRequest) (*dto.TraitsResponse, error) {
	if userId == 0 || req.RoleID == 0 {
		return nil, errors.New("userId/roleId required")
	}
	// 1. 范围校验(真正的非法输入: [-2,2] 之外)
	if err := validateOceanRange(req); err != nil {
		return nil, err
	}
	// 2. 预设存在性校验(同时确保预设字典已 seed)
	if _, err := s.ListPresets(ctx, userId); err != nil {
		return nil, err
	}
	preset, err := s.presetRepo.GetByUserAndKey(ctx, userId, req.PresetType)
	if err != nil {
		return nil, err
	}
	if preset == nil {
		return nil, fmt.Errorf("%w: presetType=%s", ErrPresetNotFound, req.PresetType)
	}
	utils.LogWithCtx(ctx, "RoleCoreService.SaveTraits",
		"入参 | userId=%d roleId=%d preset=%s O=%.3f C=%.3f E=%.3f A=%.3f N=%.3f",
		userId, req.RoleID, req.PresetType,
		req.Openness, req.Conscientiousness, req.Extraversion, req.Agreeableness, req.Neuroticism)

	// 3. 计算 5 维 delta(软告警用,不阻断保存)—— 仅与同 preset 的历史值比较
	old, _ := s.traitsRepo.GetByUserRolePreset(ctx, userId, req.RoleID, req.PresetType)
	maxDelta := 0.0
	if old != nil {
		deltas := []float64{
			math.Abs(req.Openness - old.Openness),
			math.Abs(req.Conscientiousness - old.Conscientiousness),
			math.Abs(req.Extraversion - old.Extraversion),
			math.Abs(req.Agreeableness - old.Agreeableness),
			math.Abs(req.Neuroticism - old.Neuroticism),
		}
		for _, d := range deltas {
			if d > maxDelta {
				maxDelta = d
			}
		}
	}

	// 4. 写入(preset_type 作为主键一段,不同 preset 写不同行)
	t := &models.RoleTraits{
		UserID:            userId,
		RoleID:            req.RoleID,
		PresetType:        req.PresetType,
		Openness:          req.Openness,
		Conscientiousness: req.Conscientiousness,
		Extraversion:      req.Extraversion,
		Agreeableness:     req.Agreeableness,
		Neuroticism:       req.Neuroticism,
	}
	err = s.traitsRepo.UpsertWithTx(ctx, nil, t)
	if err != nil {
		utils.LogWithCtx(ctx, "RoleCoreService.SaveTraits", "写入失败 | err=%v", err)
		return nil, err
	}

	// 注意:SaveTraits 不再自动切 active —— 保存 OCEAN 五维与切换"当前使用性格"
	// 是两件事。前端拆为两个按钮:
	//   - 「保存五维性格配置」:落库五维值(本方法)
	//   - 「使用性格」:仅切 active preset(SetActivePreset)
	// 让用户在不改 OCEAN 的情况下也能切换当前使用的预设。

	// 5. 审计(把 maxDelta 也写进审计日志,便于将来追溯大变更)
	beforeJSON := ""
	if old != nil {
		beforeJSON = mustMarshal(traitsToMap(old))
	}
	afterJSON := mustMarshal(traitsToMap(t))
	delta := "preset=" + req.PresetType
	if maxDelta > oceanStepHardLimit {
		delta = fmt.Sprintf("preset=%s max_delta=%.3f(>0.05)", req.PresetType, maxDelta)
	}
	_ = s.auditRepo.Create(ctx, &models.RoleCoreAuditLog{
		UserID:       userId,
		RoleID:       req.RoleID,
		Action:       models.AuditActionUpdate,
		TargetType:   models.AuditTargetTraits,
		BeforeJSON:   beforeJSON,
		AfterJSON:    afterJSON,
		DeltaSummary: delta,
		TriggerEvent: models.AuditTriggerUserEdit,
	})

	// 6. 拉回最新行 + 附带软告警(maxDelta/warning 给前端弹窗)
	tFresh, ferr := s.traitsRepo.GetByUserRolePreset(ctx, userId, req.RoleID, req.PresetType)
	if ferr != nil {
		return nil, ferr
	}
	resp := traitsToResponse(tFresh)
	resp.MaxDelta = maxDelta
	if maxDelta > oceanStepHardLimit {
		resp.Warning = fmt.Sprintf("本次最大单步变更 %.3f(>0.05),已落库但请确认意图", maxDelta)
	}
	return resp, nil
}

// SetActivePreset 切换某 (user, role) 当前生效的 OCEAN 预设。
//
// 与 SaveTraits 的职责拆分:
//   - SaveTraits:写入 OCEAN 五维值(可能改得很细)
//   - SetActivePreset:仅切换 chat 回复时读哪个 preset,不写 OCEAN
//
// 这样前端可以拆为「保存五维性格配置」+「使用性格」两个独立按钮,
// 用户在不改 OCEAN 的情况下也能切 active 预设。
//
// 校验:
//   - roleId / presetType 必填
//   - presetType 必须存在于用户的预设字典中(否则 400 ErrPresetNotFound)
//
// 返回新 activePresetType。
func (s *RoleCoreService) SetActivePreset(ctx context.Context, userId, roleId uint, presetType string) (string, error) {
	if userId == 0 || roleId == 0 {
		return "", errors.New("userId/roleId required")
	}
	if presetType == "" {
		return "", errors.New("presetType required")
	}
	// 校验 preset 存在 + 同时确保预设字典已 seed
	if _, err := s.ListPresets(ctx, userId); err != nil {
		return "", err
	}
	preset, err := s.presetRepo.GetByUserAndKey(ctx, userId, presetType)
	if err != nil {
		return "", err
	}
	if preset == nil {
		return "", fmt.Errorf("%w: presetType=%s", ErrPresetNotFound, presetType)
	}

	if err := s.activePresetRepo.Upsert(ctx, userId, roleId, presetType); err != nil {
		utils.LogWithCtx(ctx, "RoleCoreService.SetActivePreset", "upsert 失败 | err=%v", err)
		return "", err
	}
	utils.LogWithCtx(ctx, "RoleCoreService.SetActivePreset",
		"成功 | userId=%d roleId=%d preset=%s", userId, roleId, presetType)
	return presetType, nil
}

// GetTraits 取某 (user, role) 下所有预设的 OCEAN 五维 + 当前生效预设
//
// 返回 dto.TraitsListResponse{List, ActivePresetType}:
//   - List: 所有用户已保存的预设 OCEAN 行(每个 preset 一条)
//   - ActivePresetType: 当前真正生效的预设 key(决定 chat 回复时读的 OCEAN)
//
// 设计要点：切预设展示时,前端按 presetType 在 List 中查找:
//   - 找到 → 使用该行作为用户自定义值
//   - 未找到 → 套用预设基线
//
// ActivePresetType 与 List 的语义不同:List 是「已保存的 OCEAN」,active 是
// 「正在被 chat 读的那一个」。
//
// 历史演进:
//   - 早期返回单条 *TraitsResponse → 切换预设互窜 bug
//   - 改为返回 []TraitsResponse → 修复合并存,但没「当前使用」概念
//   - 改为返回 TraitsListResponse → 增加 active preset 跟踪
func (s *RoleCoreService) GetTraits(ctx context.Context, userId, roleId uint) (*dto.TraitsListResponse, error) {
	list, err := s.traitsRepo.ListByUserRole(ctx, userId, roleId)
	if err != nil {
		return nil, err
	}
	out := make([]dto.TraitsResponse, 0, len(list))
	for i := range list {
		out = append(out, *traitsToResponse(&list[i]))
	}
	active, err := s.resolveActivePresetType(ctx, userId, roleId, list)
	if err != nil {
		// 解析失败时回退到默认,不阻断 list 返回
		utils.LogWithCtx(ctx, "RoleCoreService.GetTraits", "resolveActivePresetType 失败 | err=%v", err)
		active = models.PresetCompanionDefault
	}
	return &dto.TraitsListResponse{
		List:             out,
		ActivePresetType: active,
	}, nil
}

// resolveActivePresetType 决定某 (user, role) 当前真正生效的 preset。
//
// 优先级:
//  1. role_active_preset 表有记录 且 该 preset 在传入的 traitsList 中存在
//     → 返回该 preset(用户上次保存的)
//  2. role_active_preset 表有记录 但 traitsList 里没有(可能 preset 被删了
//     或 trait 行被清空) → 回退到 companion_default
//  3. role_active_preset 表无记录(全新角色或迁移前数据) → 回退到 companion_default
//
// 设计要点：始终返回 preset key 字符串(不是 *models.RoleTraits),调用方需要
// OCEAN 五维时再用 GetByUserRolePreset 取。
func (s *RoleCoreService) resolveActivePresetType(ctx context.Context, userId, roleId uint, traitsList []models.RoleTraits) (string, error) {
	ap, err := s.activePresetRepo.GetByUserRole(ctx, userId, roleId)
	if err != nil {
		return models.PresetCompanionDefault, err
	}
	if ap == nil {
		return models.PresetCompanionDefault, nil
	}
	// active preset 在 traitsList 里存在 → 用它
	for i := range traitsList {
		if traitsList[i].PresetType == ap.PresetType {
			return ap.PresetType, nil
		}
	}
	// active preset 指向的 trait 行已不存在(可能 preset 被删 / trait 被清)
	// 兜底到默认,避免 chat 读到空数据
	utils.LogWithCtx(ctx, "RoleCoreService.resolveActivePresetType",
		"active preset %s 不在 traitsList 中,回退到默认 | userId=%d roleId=%d",
		ap.PresetType, userId, roleId)
	return models.PresetCompanionDefault, nil
}

// GetTraitsByPreset 取某 (user, role) 下指定 preset 的 OCEAN 五维(单条)
//
// 返回 *TraitsResponse + nil 表示找到;返回 nil + ErrTraitsNotFound 表示未保存过(走预设基线)。
func (s *RoleCoreService) GetTraitsByPreset(ctx context.Context, userId, roleId uint, presetType string) (*dto.TraitsResponse, error) {
	t, err := s.traitsRepo.GetByUserRolePreset(ctx, userId, roleId, presetType)
	if err != nil {
		if err.Error() == "record not found" {
			return nil, ErrTraitsNotFound
		}
		return nil, err
	}
	return traitsToResponse(t), nil
}

// ===== OCEAN 预设字典 =====

// 6 个默认预设的 OCEAN 基线值 (PRD 03_PRD_change_engine.md)。
// 种子仅在用户首次访问时插入一次,与默认前端的 PRESET_OPTIONS 保持一致。
var defaultPresets = []models.RoleOceanPreset{
	{
		Key: models.PresetKeyCompanionDefault, Label: "标准陪伴",
		Description: "温暖、亲和、低神经质",
		Openness:    0.4, Conscientiousness: -0.1, Extraversion: 0.2, Agreeableness: 0.7, Neuroticism: -0.4,
		IsProtected: true, // 标准陪伴不可删
	},
	{
		Key: models.PresetKeyRationalAdvisor, Label: "理性顾问",
		Description: "逻辑清晰、克制情绪",
		Openness:    0.2, Conscientiousness: 0.7, Extraversion: -0.3, Agreeableness: 0.2, Neuroticism: -0.5,
	},
	{
		Key: models.PresetKeyEnergeticGirl, Label: "元气少女",
		Description: "活力充沛、乐观开朗",
		Openness:    0.6, Conscientiousness: -0.3, Extraversion: 0.8, Agreeableness: 0.6, Neuroticism: -0.6,
	},
	{
		Key: models.PresetKeyGentleSenior, Label: "温柔学长",
		Description: "稳重体贴、温和有礼",
		Openness:    0.3, Conscientiousness: 0.4, Extraversion: 0.1, Agreeableness: 0.6, Neuroticism: -0.3,
	},
	{
		Key: models.PresetKeySarcasticFriend, Label: "毒舌损友",
		Description: "直率犀利、敢说实话",
		Openness:    0.5, Conscientiousness: -0.2, Extraversion: 0.4, Agreeableness: -0.2, Neuroticism: -0.1,
	},
	{
		Key: models.PresetKeyPsychCounselor, Label: "心理咨询师",
		Description: "倾听共情、专业支持",
		Openness:    0.7, Conscientiousness: 0.6, Extraversion: 0, Agreeableness: 0.9, Neuroticism: -0.7,
	},
}

// ListPresets 列出用户的所有预设。首次访问时自动种子默认 6 个。
func (s *RoleCoreService) ListPresets(ctx context.Context, userId uint) ([]dto.OceanPresetResponse, error) {
	n, err := s.presetRepo.CountByUser(ctx, userId)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		// 首次访问,种子默认 6 个
		seeds := make([]models.RoleOceanPreset, 0, len(defaultPresets))
		for _, p := range defaultPresets {
			seeds = append(seeds, models.RoleOceanPreset{
				UserID:            userId,
				Key:               p.Key,
				Label:             p.Label,
				Description:       p.Description,
				Openness:          p.Openness,
				Conscientiousness: p.Conscientiousness,
				Extraversion:      p.Extraversion,
				Agreeableness:     p.Agreeableness,
				Neuroticism:       p.Neuroticism,
				IsProtected:       p.IsProtected,
			})
		}
		if err := s.presetRepo.BatchCreate(ctx, seeds); err != nil {
			return nil, err
		}
	}
	list, err := s.presetRepo.ListByUser(ctx, userId)
	if err != nil {
		return nil, err
	}
	out := make([]dto.OceanPresetResponse, 0, len(list))
	for i := range list {
		out = append(out, presetToResponse(&list[i]))
	}
	return out, nil
}

// CreatePreset 新增一个用户自定义预设。Key 必须唯一(用户范围内)。
func (s *RoleCoreService) CreatePreset(ctx context.Context, userId uint, req *dto.CreatePresetRequest) (*dto.OceanPresetResponse, error) {
	if !isValidPresetKey(req.Key) {
		return nil, ErrPresetInvalidKey
	}
	// 先确认无同名 key
	existing, err := s.presetRepo.GetByUserAndKey(ctx, userId, req.Key)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrPresetKeyConflict
	}
	p := &models.RoleOceanPreset{
		UserID:            userId,
		Key:               req.Key,
		Label:             req.Label,
		Description:       req.Description,
		Openness:          req.Openness,
		Conscientiousness: req.Conscientiousness,
		Extraversion:      req.Extraversion,
		Agreeableness:     req.Agreeableness,
		Neuroticism:       req.Neuroticism,
		IsProtected:       false, // 用户自建永远非 protected
	}
	if err := s.presetRepo.Create(ctx, p); err != nil {
		return nil, err
	}
	resp := presetToResponse(p)
	return &resp, nil
}

// UpdatePreset 修改预设的 label / description / OCEAN。Key 不可改。
//
// 注意:is_protected=true 的预设(如标准陪伴)允许修改数值/label,
// 只是不允许删除。这与用户的"标准陪伴不允许删除只能修改"诉求相符。
func (s *RoleCoreService) UpdatePreset(ctx context.Context, userId, id uint, req *dto.UpdatePresetRequest) (*dto.OceanPresetResponse, error) {
	p, err := s.presetRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrPresetNotFound
	}
	if p.UserID != userId {
		// 越权 → 一律返回 not found,不暴露存在性
		return nil, ErrPresetNotFound
	}
	p.Label = req.Label
	p.Description = req.Description
	p.Openness = req.Openness
	p.Conscientiousness = req.Conscientiousness
	p.Extraversion = req.Extraversion
	p.Agreeableness = req.Agreeableness
	p.Neuroticism = req.Neuroticism
	if err := s.presetRepo.Update(ctx, p); err != nil {
		return nil, err
	}
	resp := presetToResponse(p)
	return &resp, nil
}

// DeletePreset 删除预设。is_protected=true 拒绝。
//
// 顺带清理：如果该 preset 被设为某个角色的 active preset,清理对应 role_active_preset
// 记录(避免悬空引用)。下一次访问该角色时会自动回退到 companion_default。
func (s *RoleCoreService) DeletePreset(ctx context.Context, userId, id uint) error {
	p, err := s.presetRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p == nil {
		return ErrPresetNotFound
	}
	if p.UserID != userId {
		return ErrPresetNotFound
	}
	if p.IsProtected {
		return ErrPresetProtected
	}
	// 先删 preset 字典,再清 active 记录(顺序无所谓,都是 idempotent)
	if err := s.presetRepo.Delete(ctx, id); err != nil {
		return err
	}
	// 这里不查 role 表(避免引入额外依赖);用 preset_key 模糊匹配所有
	// (user_id, role_id) 中 preset_type 等于该 key 的 active 记录。
	// 用户级 — 但通常一个用户的 preset key 唯一,所以只会匹配到该用户自己的记录。
	// 用 application 层校验 userId 即可。
	if perr := s.activePresetRepo.DeleteByUserAndKey(ctx, userId, p.Key); perr != nil {
		utils.LogWithCtx(ctx, "RoleCoreService.DeletePreset",
			"清理 active preset 失败(非致命) | key=%s err=%v", p.Key, perr)
	}
	return nil
}

// presetToResponse model → dto
func presetToResponse(p *models.RoleOceanPreset) dto.OceanPresetResponse {
	return dto.OceanPresetResponse{
		ID:                p.ID,
		UserID:            p.UserID,
		Key:               p.Key,
		Label:             p.Label,
		Description:       p.Description,
		Openness:          p.Openness,
		Conscientiousness: p.Conscientiousness,
		Extraversion:      p.Extraversion,
		Agreeableness:     p.Agreeableness,
		Neuroticism:       p.Neuroticism,
		IsProtected:       p.IsProtected,
		CreatedAt:         p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         p.UpdatedAt.Format(time.RFC3339),
	}
}

// isValidPresetKey 校验 key 字符集:小写字母/数字/下划线/短横线,1~64。
//
// 限制字符集的目的:key 会写入 role_traits.presetType,后续如果做 SQL 检索或
// URL 路径,需要可移植的稳定标识符;同时避免特殊字符引发注入/解析问题。
func isValidPresetKey(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

// ===== Mood（M2 完整实现）=====

// GetMood 取心情快照(无记录时返回中性默认, 不报错)
func (s *RoleCoreService) GetMood(ctx context.Context, userId, roleId uint) (*dto.MoodSnapshot, error) {
	m, err := s.moodRepo.GetByUserRole(ctx, userId, roleId)
	if err != nil {
		if err.Error() == "record not found" {
			// 冷启动返回中性默认值
			return defaultMoodSnapshot(), nil
		}
		return nil, err
	}
	return &dto.MoodSnapshot{
		InstantVal:        m.InstantVal,
		InstantIntensity:  m.InstantIntensity,
		InstantEmotion:    m.InstantEmotion,
		ShortVal:          m.ShortVal,
		ShortIntensity:    m.ShortIntensity,
		ShortEmotion:      m.ShortEmotion,
		BaselineVal:       m.BaselineVal,
		BaselineIntensity: m.BaselineIntensity,
		BaselineEmotion:   m.BaselineEmotion,
	}, nil
}

// UpdateMood 完整实现 M2 三桶心情耦合(PRD 03_PRD_change_engine.md)。
//
// 公式：
//
//	new_instant = clamp(
//	    baseline*0.20 + short*0.30 + event_impact*event_intensity +
//	    personality_bias + relationship_influence + noise(σ=0.02))
//
// short / baseline 同步迁移(本次事件不衰减,衰减由定时器触发)。
func (s *RoleCoreService) UpdateMood(ctx context.Context, req *dto.UpdateMoodRequest) (*dto.MoodSnapshot, error) {
	utils.LogWithCtx(ctx, "RoleCoreService.UpdateMood",
		"入参 | userId=%d roleId=%d impact=%.3f intensity=%.3f emotion=%s trigger=%s",
		req.UserID, req.RoleID, req.EventImpact, req.EventIntensity, req.Emotion, req.TriggerEvent)

	// 1. 拉取旧值(无则用中性默认)
	old, err := s.moodRepo.GetByUserRole(ctx, req.UserID, req.RoleID)
	if err != nil && err.Error() != "record not found" {
		return nil, err
	}
	if old == nil {
		old = &models.RoleMood{
			UserID: req.UserID, RoleID: req.RoleID,
			InstantVal: 0, InstantIntensity: 0.2, InstantEmotion: "neutral",
			ShortVal: 0, ShortIntensity: 0.3, ShortEmotion: "neutral",
			BaselineVal: 0, BaselineIntensity: 0.4, BaselineEmotion: "neutral",
		}
	}

	// 2. 从 OCEAN 读 personality_bias
	//
	// 历史演进:曾硬编码优先取 companion_default,不能反映用户实际选择。
	// 修复:改用 active preset —— 用户最近一次点「设置当前性格」保存的 preset。
	// 这样 chat 心情事件的人格底色与用户在 OCEAN 页选的一致,体验一致。
	// resolveActivePresetType 已处理:
	//   - active 表有记录 → 用该 preset 的 OCEAN
	//   - 没记录/preset 已被删 → 兜底到 companion_default
	personalityBias := 0.0
	relationshipInfluence := 0.0
	if traitsList, terr := s.traitsRepo.ListByUserRole(ctx, req.UserID, req.RoleID); terr == nil && len(traitsList) > 0 {
		activePreset, _ := s.resolveActivePresetType(ctx, req.UserID, req.RoleID, traitsList)
		var t *models.RoleTraits
		for i := range traitsList {
			if traitsList[i].PresetType == activePreset {
				t = &traitsList[i]
				break
			}
		}
		if t == nil && len(traitsList) > 0 {
			// active preset 的 trait 行不存在(理论上 resolveActivePresetType
			// 已保证 activePreset 在 list 中,但兜底一下)
			t = &traitsList[0]
		}
		if t != nil {
			// personality_bias = 0.10*O + 0.05*A - 0.15*N
			personalityBias = 0.10*t.Openness + 0.05*t.Agreeableness - 0.15*t.Neuroticism
		}
	}
	if r, rerr := s.relRepo.GetByUserRole(ctx, req.UserID, req.RoleID); rerr == nil && r != nil {
		// relationship_influence = intimacy*0.05 (信任感正影响心情)
		relationshipInfluence = r.Intimacy * 0.05
	}

	// 3. 三桶耦合公式(noise σ=0.02 暂不引入,简化避免不可重现)
	newInstant := clampValue(
		old.BaselineVal*0.20+
			old.ShortVal*0.30+
			req.EventImpact*req.EventIntensity+
			personalityBias+
			relationshipInfluence,
		-1.0, 1.0,
	)

	// 4. emotion 推断(若请求未带 emotion)
	emotion := req.Emotion
	if emotion == "" {
		emotion = classifyEmotionFromValence(newInstant)
	}
	intensity := clampValue(req.EventIntensity, 0, 1)
	// intensity = max(已有, 当前事件强度) — 避免情绪瞬间回落
	if intensity < old.InstantIntensity {
		intensity = old.InstantIntensity
	}

	// 5. 写入
	now := time.Now()
	m := &models.RoleMood{
		UserID:            req.UserID,
		RoleID:            req.RoleID,
		InstantVal:        newInstant,
		InstantIntensity:  intensity,
		InstantEmotion:    emotion,
		ShortVal:          old.ShortVal, // 本次事件不衰减,衰减由定时器触发
		ShortIntensity:    old.ShortIntensity,
		ShortEmotion:      old.ShortEmotion,
		BaselineVal:       old.BaselineVal,
		BaselineIntensity: old.BaselineIntensity,
		BaselineEmotion:   old.BaselineEmotion,
		LastEventAt:       now,
	}
	if err := s.moodRepo.UpsertWithTx(ctx, nil, m); err != nil {
		utils.LogWithCtx(ctx, "RoleCoreService.UpdateMood", "写入失败 | err=%v", err)
		return nil, err
	}

	// 6. 审计
	_ = s.auditRepo.Create(ctx, &models.RoleCoreAuditLog{
		UserID:       req.UserID,
		RoleID:       req.RoleID,
		Action:       models.AuditActionUpdate,
		TargetType:   models.AuditTargetMood,
		BeforeJSON:   mustMarshal(map[string]interface{}{"instantVal": old.InstantVal, "instantEmotion": old.InstantEmotion}),
		AfterJSON:    mustMarshal(map[string]interface{}{"instantVal": newInstant, "instantEmotion": emotion}),
		DeltaSummary: fmt.Sprintf("instant=%.3f emotion=%s", newInstant, emotion),
		TriggerEvent: req.TriggerEvent,
	})

	utils.LogWithCtx(ctx, "RoleCoreService.UpdateMood",
		"完成 | newInstant=%.3f emotion=%s", newInstant, emotion)
	return &dto.MoodSnapshot{
		InstantVal:        newInstant,
		InstantIntensity:  intensity,
		InstantEmotion:    emotion,
		ShortVal:          old.ShortVal,
		ShortIntensity:    old.ShortIntensity,
		ShortEmotion:      old.ShortEmotion,
		BaselineVal:       old.BaselineVal,
		BaselineIntensity: old.BaselineIntensity,
		BaselineEmotion:   old.BaselineEmotion,
	}, nil
}

// TickMoodDecay 触发衰减(M2 定时器调用,30min / 24h)。
//
// 根据 last_event_at 距今时长决定是否执行衰减：
// - 距今 > 30min：执行 short = 0.7*short + 0.3*instant
// - 距今 > 24h：执行 baseline = 0.7*baseline + 0.3*short
func (s *RoleCoreService) TickMoodDecay(ctx context.Context, userId, roleId uint) error {
	old, err := s.moodRepo.GetByUserRole(ctx, userId, roleId)
	if err != nil || old == nil {
		return err
	}
	now := time.Now()
	elapsed := now.Sub(old.LastEventAt)
	newShort := old.ShortVal
	newShortIntensity := old.ShortIntensity
	newBaseline := old.BaselineVal
	newBaselineIntensity := old.BaselineIntensity
	if elapsed > 30*time.Minute {
		newShort = old.ShortVal*0.7 + old.InstantVal*0.3
		newShortIntensity = old.ShortIntensity*0.7 + old.InstantIntensity*0.3
	}
	if elapsed > 24*time.Hour {
		newBaseline = old.BaselineVal*0.7 + old.ShortVal*0.3
		newBaselineIntensity = old.BaselineIntensity*0.7 + old.ShortIntensity*0.3
	}
	if newShort == old.ShortVal && newBaseline == old.BaselineVal {
		return nil // 无需衰减
	}
	old.LastEventAt = now
	m := *old
	m.ShortVal = newShort
	m.ShortIntensity = newShortIntensity
	m.BaselineVal = newBaseline
	m.BaselineIntensity = newBaselineIntensity
	return s.moodRepo.UpsertWithTx(ctx, nil, &m)
}

// ListMoodStale 列出距 last_event_at 超过 minElapsed 的心情记录(供后台调度)。
func (s *RoleCoreService) ListMoodStale(ctx context.Context, minElapsed time.Duration) ([]models.RoleMood, error) {
	before := time.Now().Add(-minElapsed)
	return s.moodRepo.ListAllWithLastEventBefore(ctx, before)
}

// ===== Relationship（M3 接口,M0 占位）=====

// GetRelationship 取关系值快照(无记录时返回零值)
func (s *RoleCoreService) GetRelationship(ctx context.Context, userId, roleId uint) (*dto.RelationshipSnapshot, error) {
	r, err := s.relRepo.GetByUserRole(ctx, userId, roleId)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return &dto.RelationshipSnapshot{}, nil
	}
	return &dto.RelationshipSnapshot{
		Intimacy:     r.Intimacy,
		Trust:        r.Trust,
		Satisfaction: r.Satisfaction,
		CoDays:       r.CoDays,
	}, nil
}

// AccumulateRelationship M3 完整实现:按事件类型累积三维关系值。
//
// 事件类型(对应 PRD 02 数据模型 + 03 累积规则):
//   - self_disclosure     用户自我披露 → 亲密度 +
//   - turn                普通对话轮次 → 亲密度 +
//   - positive_feedback   收到用户正向反馈 → 信任 + / 满意度 +
//   - negative_feedback   收到用户负向反馈 → 信任 - / 满意度 -
//   - co_days_7           陪伴 7 天里程碑 → 关系值全维度 +
//
// 单步上限(避免反馈累积/披露过激):
//   - intimacy:     ≤ 0.05
//   - trust:        ≤ 0.02
//   - satisfaction: ≤ 0.02(由具体事件决定方向)
//
// 设计:整数累积 → 即同一轮多条事件也能渐进叠加,但单事件有上限。
func (s *RoleCoreService) AccumulateRelationship(ctx context.Context, userId uint, req *dto.RelationshipEventRequest) (*dto.RelationshipSnapshot, error) {
	utils.LogWithCtx(ctx, "RoleCoreService.AccumulateRelationship",
		"入参 | userId=%d roleId=%d event=%s source=%s",
		userId, req.RoleID, req.EventType, req.Source)

	// 1. 拉取旧值(无记录视为全 0)
	old, _ := s.relRepo.GetByUserRole(ctx, userId, req.RoleID)
	if old == nil {
		old = &models.RoleRelationship{
			UserID: userId, RoleID: req.RoleID,
			Intimacy: 0, Trust: 0, Satisfaction: 0, CoDays: 0,
		}
	}

	// 2. 计算 delta
	dIntimacy, dTrust, dSatisfaction := 0.0, 0.0, 0.0
	switch req.EventType {
	case "self_disclosure":
		// 用户自我披露,亲密度大幅上升(但受 0.05 上限)
		dIntimacy = 0.05
	case "turn":
		// 普通对话轮次,亲密度小幅
		dIntimacy = 0.01
	case "positive_feedback":
		// 正向反馈:信任 + 满意度
		dTrust = 0.02
		dSatisfaction = 0.02
	case "negative_feedback":
		// 负向反馈:信任 - 满意度 -(信任下调稍大,表示信任更难修复)
		dTrust = -0.02
		dSatisfaction = -0.02
	case "co_days_7":
		// 7 天里程碑:三个维度都小幅 +
		dIntimacy = 0.03
		dTrust = 0.02
		dSatisfaction = 0.02
	default:
		// DTO oneof 已校验,理论上不会进来
		return nil, ErrInvalidSuggestionOp
	}

	// 3. clamp 到 [0, 1](累积型指标不允负)
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 1 {
			return 1
		}
		return v
	}
	newIntimacy := clamp(old.Intimacy + dIntimacy)
	newTrust := clamp(old.Trust + dTrust)
	newSatisfaction := clamp(old.Satisfaction + dSatisfaction)

	utils.LogWithCtx(ctx, "RoleCoreService.AccumulateRelationship",
		"累积 | userId=%d roleId=%d event=%s ΔI=%.3f ΔT=%.3f ΔS=%.3f → I=%.3f T=%.3f S=%.3f",
		userId, req.RoleID, req.EventType,
		dIntimacy, dTrust, dSatisfaction,
		newIntimacy, newTrust, newSatisfaction)

	// 4. 落库(co_days 由 IncrementCoDaysWithTx 单独走,这里不动)
	updated := &models.RoleRelationship{
		UserID:       userId,
		RoleID:       req.RoleID,
		Intimacy:     newIntimacy,
		Trust:        newTrust,
		Satisfaction: newSatisfaction,
		CoDays:       old.CoDays,
	}
	if err := s.relRepo.UpsertWithTx(ctx, nil, updated); err != nil {
		return nil, err
	}

	// 5. 审计
	_ = s.auditRepo.Create(ctx, &models.RoleCoreAuditLog{
		UserID:       userId,
		RoleID:       req.RoleID,
		Action:       models.AuditActionUpdate,
		TargetType:   models.AuditTargetRelationship,
		BeforeJSON:   mustMarshal(old),
		AfterJSON:    mustMarshal(updated),
		DeltaSummary: fmt.Sprintf("event=%s ΔI=%.3f ΔT=%.3f ΔS=%.3f", req.EventType, dIntimacy, dTrust, dSatisfaction),
		TriggerEvent: models.AuditTriggerUserEdit,
	})

	return &dto.RelationshipSnapshot{
		Intimacy:     newIntimacy,
		Trust:        newTrust,
		Satisfaction: newSatisfaction,
		CoDays:       old.CoDays,
	}, nil
}

// ===== Relationship 事件触发钩子 =====
//
// 历史:AccumulateRelationship 实现了完整的累积算法,但没有任何调用方。
// 早期 chat 路径透传到 Python 后就直接返回 SSE,没有把「用户发了一条消息」
// 翻译成后端的 turn 事件 —— 这就是用户报「聊了很多但关系值不变」的根因。
//
// 下面是 chat 完成的回调入口,由 chat handler 在 sync/stream 成功路径调用。
// 失败仅记日志(关系值是辅助指标,不能因为它出错就阻断 chat 主流程)。

// OnChatTurn 一次对话轮次完成后调用:触发 turn 关系事件。
//
// 入参 userId/roleId 是字符串(对齐 chat DTO),内部 parse 成 uint。
// 无角色聊天(roleId 为空)→ 直接跳过,不累积。
//
// 注意:此方法不抛错给调用方 —— 失败仅记日志,chat 主流程不该被关系值
// 写入失败影响。
func (s *RoleCoreService) OnChatTurn(ctx context.Context, userIdStr, roleIdStr string) {
	userId, err := strconv.ParseUint(userIdStr, 10, 64)
	if err != nil || userId == 0 {
		utils.LogWithCtx(ctx, "RoleCoreService.OnChatTurn", "跳过(userId 解析失败或为 0) | raw=%q err=%v", userIdStr, err)
		return
	}
	roleId, err := strconv.ParseUint(roleIdStr, 10, 64)
	if err != nil || roleId == 0 {
		utils.LogWithCtx(ctx, "RoleCoreService.OnChatTurn", "跳过(roleId 解析失败或为 0) | raw=%q err=%v", roleIdStr, err)
		return
	}
	if _, err := s.AccumulateRelationship(ctx, uint(userId), &dto.RelationshipEventRequest{
		RoleID:    uint(roleId),
		EventType: "turn",
		Source:    "chat",
	}); err != nil {
		utils.LogWithCtx(ctx, "RoleCoreService.OnChatTurn", "累积失败(忽略) | err=%v", err)
	}
}

// OnFeedback 提交反馈后调用:根据 rating 触发 positive/negative_feedback 事件。
//
//   - rating > 0 → positive_feedback(信任 +0.02,满意度 +0.02)
//   - rating < 0 → negative_feedback(信任 -0.02,满意度 -0.02)
//   - rating == 0 → 不触发(中立反馈不驱动关系值)
//
// 同 OnChatTurn:失败仅记日志,不抛给调用方。
func (s *RoleCoreService) OnFeedback(ctx context.Context, userId uint, roleID uint, rating int) {
	if userId == 0 || roleID == 0 {
		return
	}
	var eventType string
	switch {
	case rating > 0:
		eventType = "positive_feedback"
	case rating < 0:
		eventType = "negative_feedback"
	default:
		// rating == 0 中立反馈,不驱动关系值
		return
	}
	if _, err := s.AccumulateRelationship(ctx, userId, &dto.RelationshipEventRequest{
		RoleID:    roleID,
		EventType: eventType,
		Source:    "feedback",
	}); err != nil {
		utils.LogWithCtx(ctx, "RoleCoreService.OnFeedback", "累积失败(忽略) | err=%v", err)
	}
}

// ===== Belief（M3 接口,M0 占位）=====

// ListBeliefs 列出已确认信念
func (s *RoleCoreService) ListBeliefs(ctx context.Context, userId, roleId uint) ([]dto.BeliefItem, error) {
	list, err := s.beliefRepo.ListByUserRole(ctx, userId, roleId, false)
	if err != nil {
		return nil, err
	}
	out := make([]dto.BeliefItem, 0, len(list))
	for _, b := range list {
		out = append(out, dto.BeliefItem{
			ID:            b.ID,
			Topic:         b.Topic,
			Stance:        b.Stance,
			Confidence:    b.Confidence,
			EvidenceCount: b.EvidenceCount,
			Source:        b.Source,
			UserConfirmed: b.UserConfirmed,
			CreatedAt:     b.CreatedAt.Format(time.RFC3339),
			UpdatedAt:     b.UpdatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

// CreateBelief 新增信念(M0 直接可用,M3 增加危险过滤)
func (s *RoleCoreService) CreateBelief(ctx context.Context, userId uint, req *dto.CreateBeliefRequest) (*dto.BeliefItem, error) {
	source := req.Source
	if source == "" {
		source = models.BeliefSourceUserEdit
	}
	b := &models.RoleBelief{
		UserID:        userId,
		RoleID:        req.RoleID,
		Topic:         req.Topic,
		Stance:        req.Stance,
		Confidence:    req.Confidence,
		Source:        source,
		UserConfirmed: true, // 用户手动录入默认已确认
	}
	if err := s.beliefRepo.Create(ctx, b); err != nil {
		return nil, err
	}
	_ = s.auditRepo.Create(ctx, &models.RoleCoreAuditLog{
		UserID:       userId,
		RoleID:       req.RoleID,
		Action:       models.AuditActionCreate,
		TargetType:   models.AuditTargetBelief,
		AfterJSON:    mustMarshal(b),
		DeltaSummary: "topic=" + b.Topic,
		TriggerEvent: models.AuditTriggerUserEdit,
	})
	return &dto.BeliefItem{
		ID:            b.ID,
		Topic:         b.Topic,
		Stance:        b.Stance,
		Confidence:    b.Confidence,
		EvidenceCount: b.EvidenceCount,
		Source:        b.Source,
		UserConfirmed: b.UserConfirmed,
		CreatedAt:     b.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     b.UpdatedAt.Format(time.RFC3339),
	}, nil
}

// DeleteBelief 删除信念
func (s *RoleCoreService) DeleteBelief(ctx context.Context, userId uint, id uint) error {
	b, err := s.beliefRepo.GetByID(ctx, id)
	if err != nil {
		return ErrBeliefNotFound
	}
	if b.UserID != userId {
		return ErrBeliefNotFound // 404 防越权泄露
	}
	if err := s.beliefRepo.Delete(ctx, id); err != nil {
		return err
	}
	_ = s.auditRepo.Create(ctx, &models.RoleCoreAuditLog{
		UserID:       userId,
		RoleID:       b.RoleID,
		Action:       models.AuditActionDelete,
		TargetType:   models.AuditTargetBelief,
		BeforeJSON:   mustMarshal(b),
		DeltaSummary: "topic=" + b.Topic,
		TriggerEvent: models.AuditTriggerUserEdit,
	})
	return nil
}

// UpdateBelief 修改信念(topic/stance/confidence/source)
//
// 设计要点：
//   - 与 Create/Delete 一样做 ownership 检查：b.UserID != userId → ErrBeliefNotFound 防越权泄露
//   - 走 audit_log 的 update 动作,把 before/after 都记下来
//   - evidence_count / user_confirmed 不允许外部修改(只能由 M3 反馈/确认逻辑触发)
func (s *RoleCoreService) UpdateBelief(ctx context.Context, userId uint, id uint, req *dto.UpdateBeliefRequest) (*dto.BeliefItem, error) {
	b, err := s.beliefRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrBeliefNotFound
	}
	if b.UserID != userId {
		return nil, ErrBeliefNotFound
	}
	// 记录修改前快照(审计)
	before := mustMarshal(b)
	b.Topic = req.Topic
	b.Stance = req.Stance
	b.Confidence = req.Confidence
	if req.Source != "" {
		b.Source = req.Source
	}
	if err := s.beliefRepo.Update(ctx, id, map[string]interface{}{
		"topic":      b.Topic,
		"stance":     b.Stance,
		"confidence": b.Confidence,
		"source":     b.Source,
	}); err != nil {
		return nil, err
	}
	_ = s.auditRepo.Create(ctx, &models.RoleCoreAuditLog{
		UserID:       userId,
		RoleID:       b.RoleID,
		Action:       models.AuditActionUpdate,
		TargetType:   models.AuditTargetBelief,
		BeforeJSON:   before,
		AfterJSON:    mustMarshal(b),
		DeltaSummary: "topic=" + b.Topic,
		TriggerEvent: models.AuditTriggerUserEdit,
	})
	return &dto.BeliefItem{
		ID:            b.ID,
		Topic:         b.Topic,
		Stance:        b.Stance,
		Confidence:    b.Confidence,
		EvidenceCount: b.EvidenceCount,
		Source:        b.Source,
		UserConfirmed: b.UserConfirmed,
		CreatedAt:     b.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     b.UpdatedAt.Format(time.RFC3339),
	}, nil
}

// ===== Suggestion（M6 接口,M0 占位）=====

// ListPendingSuggestions 列出 pending 建议
func (s *RoleCoreService) ListPendingSuggestions(ctx context.Context, userId, roleId uint) ([]dto.SuggestionItem, error) {
	list, err := s.suggRepo.ListPendingByUserRole(ctx, userId, roleId)
	if err != nil {
		return nil, err
	}
	out := make([]dto.SuggestionItem, 0, len(list))
	for _, s := range list {
		out = append(out, suggestionToItem(&s))
	}
	return out, nil
}

// CreateSuggestion 内部接口:echo-ai LLM 抽取后写入建议
func (s *RoleCoreService) CreateSuggestion(ctx context.Context, req *dto.CreateSuggestionRequest) (*dto.SuggestionItem, error) {
	sug := &models.RoleEvolutionSuggestion{
		UserID:         req.UserID,
		RoleID:         req.RoleID,
		TargetType:     req.TargetType,
		SuggestionJSON: req.SuggestionJSON,
		Source:         req.Source,
		Confidence:     req.Confidence,
		Reason:         req.Reason,
		Status:         models.SuggestionStatusPending,
	}
	if err := s.suggRepo.Create(ctx, sug); err != nil {
		return nil, err
	}
	item := suggestionToItem(sug)
	return &item, nil
}

// ResolveSuggestion 用户处理建议(apply/skip/edit)
//
//   - action: apply / skip / edit
//   - req.EditedJSON: 仅 edit 路径使用
//
// M6 完整实现:apply 路径根据 target_type 把 suggestion_json 内容写入对应主表:
//   - persona → role_persona(整段替换)
//   - traits  → role_traits(整段替换)
//   - belief  → role_belief(insert 新行,user_confirmed=1)
//   - mood    → role_mood(UpsertWithTx 走三桶累积 schema)
func (s *RoleCoreService) ResolveSuggestion(ctx context.Context, userId uint, id uint, action string, req *dto.ResolveSuggestionRequest) error {
	sug, err := s.suggRepo.GetByID(ctx, id)
	if err != nil {
		return ErrSuggestionNotFound
	}
	if sug.UserID != userId {
		return ErrSuggestionNotFound // 404 防越权
	}

	var newStatus string
	switch action {
	case "apply":
		newStatus = models.SuggestionStatusApplied
		// 落地 suggestion 内容到对应主表(M6 完整实现)
		if err := s.applySuggestionToMainTable(ctx, sug); err != nil {
			return err
		}
	case "skip":
		newStatus = models.SuggestionStatusSkipped
	case "edit":
		newStatus = models.SuggestionStatusEdited
		// edit 时更新 suggestion_json
		if req != nil && req.EditedJSON != "" {
			sug.SuggestionJSON = req.EditedJSON
			if err := s.suggRepo.UpdateStatus(ctx, id, models.SuggestionStatusPending); err != nil {
				return err
			}
			// 再设为 edited
			newStatus = models.SuggestionStatusEdited
		}
	default:
		return ErrInvalidSuggestionOp
	}

	if err := s.suggRepo.UpdateStatus(ctx, id, newStatus); err != nil {
		return err
	}

	// 审计
	_ = s.auditRepo.Create(ctx, &models.RoleCoreAuditLog{
		UserID:       userId,
		RoleID:       sug.RoleID,
		Action:       models.AuditActionSuggestionApply,
		TargetType:   models.AuditTargetSuggestion,
		BeforeJSON:   mustMarshal(sug),
		DeltaSummary: "action=" + action + " target=" + sug.TargetType,
		TriggerEvent: models.AuditTriggerUserEdit,
	})
	return nil
}

// applySuggestionToMainTable M6 完整实现:把 suggestion_json 内容写入对应主表。
//
// JSON shape(LLM 抽取侧按此约定):
//   - persona: {identity, background, traitsTags, values, speakingStyle, taboos, exampleDialogs}
//   - traits:  {openness, conscientiousness, extraversion, agreeableness, neuroticism, presetType}
//   - belief:  {topic, stance, confidence, source?, userConfirmed?}
//   - mood:    {instant_val, instant_intensity, instant_emotion,
//     short_val, short_intensity, short_emotion,
//     baseline_val, baseline_intensity, baseline_emotion}
func (s *RoleCoreService) applySuggestionToMainTable(ctx context.Context, sug *models.RoleEvolutionSuggestion) error {
	payload := sug.SuggestionJSON
	if payload == "" {
		return fmt.Errorf("%w: suggestion_json is empty", ErrInvalidSuggestionOp)
	}

	switch sug.TargetType {
	case "persona":
		var p dto.SavePersonaRequest
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return fmt.Errorf("%w: persona payload 解析失败: %v", ErrInvalidSuggestionOp, err)
		}
		p.RoleID = sug.RoleID
		_, err := s.SavePersona(ctx, sug.UserID, &p)
		return err

	case "traits":
		var t dto.SaveTraitsRequest
		if err := json.Unmarshal([]byte(payload), &t); err != nil {
			return fmt.Errorf("%w: traits payload 解析失败: %v", ErrInvalidSuggestionOp, err)
		}
		t.RoleID = sug.RoleID
		_, err := s.SaveTraits(ctx, sug.UserID, &t)
		return err

	case "belief":
		var b struct {
			Topic         string  `json:"topic"`
			Stance        string  `json:"stance"`
			Confidence    float64 `json:"confidence"`
			Source        string  `json:"source"`
			UserConfirmed *bool   `json:"userConfirmed"`
		}
		if err := json.Unmarshal([]byte(payload), &b); err != nil {
			return fmt.Errorf("%w: belief payload 解析失败: %v", ErrInvalidSuggestionOp, err)
		}
		source := b.Source
		if source == "" {
			source = models.BeliefSourceMemoryExtract
		}
		_, err := s.CreateBelief(ctx, sug.UserID, &dto.CreateBeliefRequest{
			RoleID:     sug.RoleID,
			Topic:      b.Topic,
			Stance:     b.Stance,
			Confidence: b.Confidence,
			Source:     source,
		})
		return err

	case "mood":
		var m struct {
			InstantVal        float64 `json:"instant_val"`
			InstantIntensity  float64 `json:"instant_intensity"`
			InstantEmotion    string  `json:"instant_emotion"`
			ShortVal          float64 `json:"short_val"`
			ShortIntensity    float64 `json:"short_intensity"`
			ShortEmotion      string  `json:"short_emotion"`
			BaselineVal       float64 `json:"baseline_val"`
			BaselineIntensity float64 `json:"baseline_intensity"`
			BaselineEmotion   string  `json:"baseline_emotion"`
		}
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			return fmt.Errorf("%w: mood payload 解析失败: %v", ErrInvalidSuggestionOp, err)
		}
		mood := &models.RoleMood{
			UserID:            sug.UserID,
			RoleID:            sug.RoleID,
			InstantVal:        m.InstantVal,
			InstantIntensity:  m.InstantIntensity,
			InstantEmotion:    orDefault(m.InstantEmotion, "neutral"),
			ShortVal:          m.ShortVal,
			ShortIntensity:    m.ShortIntensity,
			ShortEmotion:      orDefault(m.ShortEmotion, "neutral"),
			BaselineVal:       m.BaselineVal,
			BaselineIntensity: m.BaselineIntensity,
			BaselineEmotion:   orDefault(m.BaselineEmotion, "neutral"),
			// last_event_at 必填(MySQL strict mode 不接受 0000-00-00),suggestion 落库时给当前时间,
			// 后续累积公式走 UpdateMood handler,该字段由 UpdateMood 自己写。
			LastEventAt: time.Now(),
		}
		return s.moodRepo.UpsertWithTx(ctx, nil, mood)

	default:
		return fmt.Errorf("%w: target_type=%s", ErrInvalidSuggestionOp, sug.TargetType)
	}
}

// orDefault 空字符串走 fallback
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// ===== Feedback（M7 接口,M0 占位）=====

// SubmitFeedback 用户反馈写入
//
// M7 完整实现:根据评级触发后续动作:
//   - rating == -2(强烈负反馈)→ 创建 persona 候选 suggestion(用户可 Apply 调整人设)
//   - rating == +2(强烈正反馈)→ 创建 belief 候选 suggestion(把成功经验固化为信念)
//   - 其他评级仅写记录 + audit,不再触发
func (s *RoleCoreService) SubmitFeedback(ctx context.Context, userId uint, req *dto.SubmitFeedbackRequest) error {
	f := &models.RoleFeedback{
		UserID:    userId,
		RoleID:    req.RoleID,
		MessageID: req.MessageID,
		Rating:    req.Rating,
		Comment:   req.Comment,
	}
	if err := s.feedbackRepo.Create(ctx, f); err != nil {
		return err
	}
	_ = s.auditRepo.Create(ctx, &models.RoleCoreAuditLog{
		UserID:       userId,
		RoleID:       req.RoleID,
		Action:       models.AuditActionFeedbackApply,
		TargetType:   models.AuditTargetFeedback,
		AfterJSON:    mustMarshal(f),
		DeltaSummary: "rating=" + strconv.Itoa(req.Rating),
		TriggerEvent: models.AuditTriggerFeedback,
	})

	// 极端负反馈:候选 persona 调整(用户可 Apply 改写)
	// 极端正反馈:候选 belief 提取(把"这次表现好"固化为信念)
	switch req.Rating {
	case -2:
		suggestion := &models.RoleEvolutionSuggestion{
			UserID:         userId,
			RoleID:         req.RoleID,
			TargetType:     "persona",
			SuggestionJSON: `{"identity":"","background":"","traitsTags":[],"values":[],"speakingStyle":"","taboos":[],"exampleDialogs":[]}`,
			Source:         models.SuggestionSourceFeedback,
			Confidence:     0.5,
			Reason: fmt.Sprintf("用户对消息 %s 给了 -2 强负反馈(comment=%s),请考虑调整人设",
				req.MessageID, req.Comment),
		}
		if err := s.suggRepo.Create(ctx, suggestion); err != nil {
			utils.LogWithCtx(ctx, "RoleCoreService.SubmitFeedback",
				"创建负反馈 suggestion 失败 | err=%v", err)
		} else {
			utils.LogWithCtx(ctx, "RoleCoreService.SubmitFeedback",
				"反馈触发 persona suggestion | userId=%d roleId=%d sugId=%d", userId, req.RoleID, suggestion.ID)
		}
	case 2:
		suggestion := &models.RoleEvolutionSuggestion{
			UserID:         userId,
			RoleID:         req.RoleID,
			TargetType:     "belief",
			SuggestionJSON: `{"topic":"","stance":"","confidence":0.7,"source":"feedback"}`,
			Source:         models.SuggestionSourceFeedback,
			Confidence:     0.7,
			Reason: fmt.Sprintf("用户对消息 %s 给了 +2 强正反馈(comment=%s),可考虑固化为信念",
				req.MessageID, req.Comment),
		}
		if err := s.suggRepo.Create(ctx, suggestion); err != nil {
			utils.LogWithCtx(ctx, "RoleCoreService.SubmitFeedback",
				"创建正反馈 suggestion 失败 | err=%v", err)
		} else {
			utils.LogWithCtx(ctx, "RoleCoreService.SubmitFeedback",
				"反馈触发 belief suggestion | userId=%d roleId=%d sugId=%d", userId, req.RoleID, suggestion.ID)
		}
	}

	return nil
}

// ===== Core Snapshot(调试面板用) =====

// GetCoreSnapshot 取完整 role core 快照(给调试面板,容错性高：缺啥返啥)
func (s *RoleCoreService) GetCoreSnapshot(ctx context.Context, userId, roleId uint) (*dto.CoreSnapshotResponse, error) {
	snap := &dto.CoreSnapshotResponse{}

	// 1. persona
	if p, err := s.personaRepo.GetByUserRole(ctx, userId, roleId); err == nil && p != nil {
		snap.Persona = personaToResponse(p)
	}

	// 2. traits —— 多预设结构下取 active preset(用户最近一次保存的)。
	//
	// 历史演进:曾硬编码优先取 companion_default,不能反映用户实际选择。
	// 修复:用 resolveActivePresetType(向后兼容:无 active 记录时回退 companion_default)
	if traitsList, err := s.traitsRepo.ListByUserRole(ctx, userId, roleId); err == nil && len(traitsList) > 0 {
		activePreset, _ := s.resolveActivePresetType(ctx, userId, roleId, traitsList)
		var t *models.RoleTraits
		for i := range traitsList {
			if traitsList[i].PresetType == activePreset {
				t = &traitsList[i]
				break
			}
		}
		if t == nil {
			// 兜底:resolveActivePresetType 已尽量保证 active 在 list 中,
			// 但万一不在(数据损坏)就用第一行,不让 snapshot 为空
			t = &traitsList[0]
		}
		snap.Traits = traitsToResponse(t)
	}

	// 3. mood
	if m, err := s.GetMood(ctx, userId, roleId); err == nil && m != nil {
		snap.Mood = m
	}

	// 4. relationship
	if r, err := s.GetRelationship(ctx, userId, roleId); err == nil && r != nil {
		snap.Relationship = r
	}

	// 5. beliefs
	if b, err := s.ListBeliefs(ctx, userId, roleId); err == nil {
		snap.Beliefs = b
	}

	return snap, nil
}

// ===== 内部辅助函数 =====

// clampValue 把数值限制在 [min, max]
func clampValue(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// classifyEmotionFromValence valence → emotion 标签
func classifyEmotionFromValence(valence float64) string {
	switch {
	case valence >= 0.6:
		return "happy"
	case valence >= 0.2:
		return "smile"
	case valence >= -0.2:
		return "neutral"
	case valence >= -0.6:
		return "concerned"
	default:
		return "sad"
	}
}

// validateOceanRange 校验 OCEAN 值 ∈ [-2, 2]
//
// 说明:OCEAN 理论标准范围是 [-1, 1]。但实际应用中存在「超调」需求(用户主动把某维推到
// 极端),所以放宽到 [-2, 2] 让用户有调整空间;再宽就是噪音,不予支持。
func validateOceanRange(req *dto.SaveTraitsRequest) error {
	for _, v := range []float64{req.Openness, req.Conscientiousness, req.Extraversion, req.Agreeableness, req.Neuroticism} {
		if v < -2.0-1e-6 || v > 2.0+1e-6 {
			return fmt.Errorf("%w: value=%.3f", ErrInvalidOceanValue, v)
		}
	}
	return nil
}

// personaToResponse model → dto
func personaToResponse(p *models.RolePersona) *dto.PersonaResponse {
	if p == nil {
		return nil
	}
	out := &dto.PersonaResponse{
		UserID:        p.UserID,
		RoleID:        p.RoleID,
		Identity:      p.Identity,
		Background:    p.Background,
		SpeakingStyle: p.SpeakingStyle,
		Version:       p.Version,
		CreatedAt:     p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     p.UpdatedAt.Format(time.RFC3339),
	}
	_ = json.Unmarshal([]byte(orEmpty(p.TraitsTags)), &out.TraitsTags)
	_ = json.Unmarshal([]byte(orEmpty(p.Values)), &out.Values)
	_ = json.Unmarshal([]byte(orEmpty(p.Taboos)), &out.Taboos)
	_ = json.Unmarshal([]byte(orEmpty(p.ExampleDialogs)), &out.ExampleDialogs)
	if out.TraitsTags == nil {
		out.TraitsTags = []string{}
	}
	if out.Values == nil {
		out.Values = []string{}
	}
	if out.Taboos == nil {
		out.Taboos = []string{}
	}
	if out.ExampleDialogs == nil {
		out.ExampleDialogs = []dto.ExampleDialogItem{}
	}
	return out
}

// traitsToMap OCEAN → map (用于审计 JSON)
func traitsToMap(t *models.RoleTraits) map[string]interface{} {
	return map[string]interface{}{
		"openness":          t.Openness,
		"conscientiousness": t.Conscientiousness,
		"extraversion":      t.Extraversion,
		"agreeableness":     t.Agreeableness,
		"neuroticism":       t.Neuroticism,
		"presetType":        t.PresetType,
	}
}

// traitsToResponse model → dto(单一预设)
func traitsToResponse(t *models.RoleTraits) *dto.TraitsResponse {
	return &dto.TraitsResponse{
		UserID:            t.UserID,
		RoleID:            t.RoleID,
		PresetType:        t.PresetType,
		Openness:          t.Openness,
		Conscientiousness: t.Conscientiousness,
		Extraversion:      t.Extraversion,
		Agreeableness:     t.Agreeableness,
		Neuroticism:       t.Neuroticism,
		UpdatedAt:         t.UpdatedAt.Format(time.RFC3339),
	}
}

// suggestionToItem model → dto
func suggestionToItem(s *models.RoleEvolutionSuggestion) dto.SuggestionItem {
	item := dto.SuggestionItem{
		ID:             s.ID,
		UserID:         s.UserID,
		RoleID:         s.RoleID,
		TargetType:     s.TargetType,
		SuggestionJSON: s.SuggestionJSON,
		Status:         s.Status,
		Source:         s.Source,
		Confidence:     s.Confidence,
		Reason:         s.Reason,
		CreatedAt:      s.CreatedAt.Format(time.RFC3339),
	}
	if s.ResolvedAt != nil {
		resolved := s.ResolvedAt.Format(time.RFC3339)
		item.ResolvedAt = &resolved
	}
	return item
}

// defaultMoodSnapshot 冷启动默认中性心情
func defaultMoodSnapshot() *dto.MoodSnapshot {
	return &dto.MoodSnapshot{
		InstantVal:        0,
		InstantIntensity:  0.2,
		InstantEmotion:    "neutral",
		ShortVal:          0,
		ShortIntensity:    0.3,
		ShortEmotion:      "neutral",
		BaselineVal:       0,
		BaselineIntensity: 0.4,
		BaselineEmotion:   "neutral",
	}
}

// orEmpty 把空字符串转为 "[]" / "{}" 以便 json.Unmarshal 不报错
func orEmpty(s string) string {
	if s == "" {
		return "[]"
	}
	return s
}

// mustMarshal 安全 JSON 序列化(失败返回空串,不抛错)
func mustMarshal(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// computePersonaDelta 计算人格差异摘要(简单长度对比,不展开细节)
func computePersonaDelta(before, after string) string {
	if before == "" {
		return "create"
	}
	return "update"
}
