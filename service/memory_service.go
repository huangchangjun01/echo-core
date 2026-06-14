package service

import (
	"echo-core/models"
	"echo-core/remote"
	"echo-core/repository"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// MemoryService 用户长期记忆服务
// 负责：加载记忆 → 构造注入到 prompt 的记忆片段 → 自动从对话中提取记忆 → 合并去重 → 持久化
//
// 设计要点：
//  1. 跨会话生效：注入位置为 system message，紧随 Agent 自身 system prompt 之后。
//  2. 类型化存储：preference(偏好) / info(事实) / knowledge(知识) / summary(概要)。
//  3. 合并去重：提取出的新条目若与历史内容语义重复，则覆盖合并，不留垃圾。
//  4. 异步写入：自动提取放在 goroutine，不阻塞主链路响应。
type MemoryService struct {
	memRepo  *repository.MemoryRepository
	aiClient *remote.AIClient
}

// NewMemoryService 构造 MemoryService
func NewMemoryService(aiClient *remote.AIClient, memRepo *repository.MemoryRepository) *MemoryService {
	if memRepo == nil {
		memRepo = repository.NewMemoryRepository()
	}
	return &MemoryService{
		memRepo:  memRepo,
		aiClient: aiClient,
	}
}

// MemoryItem 从对话中抽取的单条记忆
type MemoryItem struct {
	Type    string `json:"type"`    // preference / info / knowledge / summary
	Content string `json:"content"` // 一句话描述
}

// LoadUserMemories 加载某用户全部长期记忆
func (s *MemoryService) LoadUserMemories(userID string) ([]models.UserMemory, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID is required")
	}
	return s.memRepo.ListUserMemories(userID)
}

// BuildMemoryContext 把用户记忆格式化为可注入到 system prompt 的文本片段
// 若用户无任何记忆，返回空字符串（调用方据此判断是否注入）
func (s *MemoryService) BuildMemoryContext(userID string) (string, error) {
	memories, err := s.LoadUserMemories(userID)
	if err != nil {
		log.Printf("[MemoryService] 加载用户记忆失败 | userID: %s | error: %v", userID, err)
		return "", err
	}
	if len(memories) == 0 {
		log.Printf("[MemoryService] 用户暂无长期记忆 | userID: %s", userID)
		return "", nil
	}

	// 按类型分组输出，便于 LLM 理解
	grouped := make(map[string][]string)
	for _, m := range memories {
		t := strings.TrimSpace(m.MemoryType)
		if t == "" {
			t = "info"
		}
		grouped[t] = append(grouped[t], m.Content)
	}

	var b strings.Builder
	b.WriteString("【用户长期记忆（跨会话生效，请结合以下用户偏好/事实/知识回答问题）】\n")
	typeOrder := []string{"preference", "info", "knowledge", "summary"}
	written := map[string]bool{}
	for _, t := range typeOrder {
		if items, ok := grouped[t]; ok && len(items) > 0 {
			fmt.Fprintf(&b, "- [%s] %s\n", t, strings.Join(items, "；"))
			written[t] = true
		}
	}
	// 兜底：未在预定义顺序里的类型也输出
	for t, items := range grouped {
		if written[t] {
			continue
		}
		fmt.Fprintf(&b, "- [%s] %s\n", t, strings.Join(items, "；"))
	}
	b.WriteString("（以上记忆是系统在历史会话中为该用户沉淀的偏好/事实/知识，请主动结合它们给出更贴切的回答。）")

	out := b.String()
	log.Printf("[MemoryService] 构造用户记忆上下文 | userID: %s | memories_count: %d | context_len: %d", userID, len(memories), len(out))
	return out, nil
}

// ExtractAsync 异步从最新一轮对话中抽取长期记忆
// 抽取失败/为空不影响主流程；常用于聊天响应返回之后的「事后」落库。
func (s *MemoryService) ExtractAsync(userID, userMessage, assistantReply string) {
	if userID == "" || strings.TrimSpace(userMessage) == "" || strings.TrimSpace(assistantReply) == "" {
		return
	}
	go func() {
		if err := s.ExtractAndSave(userID, userMessage, assistantReply); err != nil {
			log.Printf("[MemoryService] 异步抽取记忆失败 | userID: %s | error: %v", userID, err)
		}
	}()
}

// ExtractAndSave 同步从对话中抽取记忆并合并保存
func (s *MemoryService) ExtractAndSave(userID, userMessage, assistantReply string) error {
	log.Printf("[MemoryService] 开始抽取用户记忆 | userID: %s | user_msg_len: %d | asst_msg_len: %d", userID, len(userMessage), len(assistantReply))

	// 1) 调 LLM 抽取候选条目
	items, err := s.extractWithLLM(userMessage, assistantReply)
	if err != nil {
		return fmt.Errorf("extract memory failed: %w", err)
	}
	if len(items) == 0 {
		log.Printf("[MemoryService] 抽取结果为空，跳过 | userID: %s", userID)
		return nil
	}
	log.Printf("[MemoryService] 抽取候选记忆 | userID: %s | candidates: %d", userID, len(items))

	// 2) 与历史记忆合并去重
	existing, err := s.memRepo.ListUserMemories(userID)
	if err != nil {
		log.Printf("[MemoryService] 加载历史记忆失败，跳过合并 | userID: %s | error: %v", userID, err)
		existing = nil
	}

	merged := mergeMemories(existing, items)

	// 3) 落库（按 type upsert）
	for t, contents := range merged {
		if len(contents) == 0 {
			continue
		}
		m := &models.UserMemory{
			UserID:     userID,
			MemoryType: t,
			Content:    strings.Join(contents, "；"),
			UpdatedAt:  time.Now(),
		}
		if err := s.memRepo.SaveUserMemory(m); err != nil {
			log.Printf("[MemoryService] 保存记忆失败 | userID: %s | type: %s | error: %v", userID, t, err)
			// 不中断其它 type 的写入
		}
	}
	log.Printf("[MemoryService] 用户记忆合并保存完成 | userID: %s | types_count: %d", userID, len(merged))
	return nil
}

// extractWithLLM 调用 LLM 抽取长期记忆
// 严格约束 LLM 以 JSON 数组返回 [{type, content}]，便于程序解析。
func (s *MemoryService) extractWithLLM(userMessage, assistantReply string) ([]MemoryItem, error) {
	if s.aiClient == nil {
		return nil, fmt.Errorf("aiClient not initialized")
	}

	systemPrompt := `你是"用户长期记忆抽取器"，负责从单轮对话中提炼关于该用户的、可在未来会话中复用的信息。

【抽取规则】
1. 只抽取"关于用户本人"的事实/偏好/知识/经历；不要抽取助手给出的通用知识或临时性内容。
2. 一条记忆用一句话表达，避免冗余。
3. 抽取粒度：能体现用户身份、习惯、偏好、职业、家庭、健康、重要事件、专业领域、长期目标等。
4. 若该轮对话无任何可沉淀信息，返回空数组 []。
5. 严格按以下 JSON 格式返回（不要任何多余文字、代码块标记或解释）：
   [{"type":"preference","content":"..."},{"type":"info","content":"..."}]
6. type 仅允许以下枚举：preference（偏好）/ info（事实）/ knowledge（用户掌握的知识）/ summary（重要事件概要）。
7. 最多返回 5 条，按重要性排序。`

	userContent := fmt.Sprintf("【用户发言】\n%s\n\n【助手回复】\n%s", userMessage, assistantReply)

	resp, err := s.aiClient.Chat([]remote.AIChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userContent},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("LLM returned no choices")
	}

	raw, ok := resp.Choices[0].Message.Content.(string)
	if !ok {
		return nil, fmt.Errorf("invalid content type from LLM")
	}

	// 兼容模型偶尔在 JSON 外层加 ```json ... ``` 或夹杂少量文字
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```JSON")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	// 仅取首个 '[' 到最后一个 ']' 之间的内容，剔除多余说明
	if l := strings.Index(raw, "["); l >= 0 {
		if r := strings.LastIndex(raw, "]"); r > l {
			raw = raw[l : r+1]
		}
	}

	var items []MemoryItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		log.Printf("[MemoryService] 解析LLM抽取结果失败 | raw: %s | error: %v", raw, err)
		return nil, fmt.Errorf("parse extracted memory: %w", err)
	}

	// 过滤 & 规范化
	allowed := map[string]bool{"preference": true, "info": true, "knowledge": true, "summary": true}
	out := make([]MemoryItem, 0, len(items))
	for _, it := range items {
		t := strings.ToLower(strings.TrimSpace(it.Type))
		c := strings.TrimSpace(it.Content)
		if c == "" {
			continue
		}
		if !allowed[t] {
			// 未知类型归到 info
			t = "info"
		}
		out = append(out, MemoryItem{Type: t, Content: c})
	}
	return out, nil
}

// mergeMemories 把新抽取的 items 与历史记忆合并去重
// 规则：按 type 分组；同 type 内若新条目与历史内容高度相似（包含关系）则丢弃历史冗余；
// 同 type 合并后去重。
func mergeMemories(existing []models.UserMemory, items []MemoryItem) map[string][]string {
	out := make(map[string][]string)

	// 1) 先把历史记忆按 type 落入桶
	for _, m := range existing {
		t := strings.ToLower(strings.TrimSpace(m.MemoryType))
		if t == "" {
			t = "info"
		}
		if m.Content == "" {
			continue
		}
		// 历史里可能是多条以 "；" 拼接的，需要拆分
		parts := splitContents(m.Content)
		for _, p := range parts {
			if p == "" {
				continue
			}
			if !containsExact(out[t], p) {
				out[t] = append(out[t], p)
			}
		}
	}

	// 2) 再把新抽取的 items 合并入桶
	for _, it := range items {
		t := it.Type
		c := it.Content
		// 相似度判定：若新条目是历史的子串，或历史包含新条目核心词，则视为重复
		if isDuplicate(out[t], c) {
			continue
		}
		out[t] = append(out[t], c)
	}

	// 3) 控制每类最大条数（避免上下文爆炸），优先保留新加入的
	for t := range out {
		if len(out[t]) > 20 {
			out[t] = out[t][len(out[t])-20:]
		}
	}
	return out
}

// splitContents 把以"；"拼接的内容拆回多条
func splitContents(s string) []string {
	parts := strings.Split(s, "；")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// containsExact 判断 slice 中是否已存在完全一致的项
func containsExact(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// isDuplicate 简易去重：新内容与历史任一条存在显著包含/重叠则视为重复
func isDuplicate(existing []string, candidate string) bool {
	c := strings.TrimSpace(candidate)
	if c == "" {
		return true
	}
	for _, e := range existing {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if e == c {
			return true
		}
		// 包含关系：互相是对方子串视为重复
		if strings.Contains(e, c) || strings.Contains(c, e) {
			return true
		}
	}
	return false
}
