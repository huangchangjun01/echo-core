package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// DefaultTools 返回默认工具集
func DefaultTools() []Tool {
	return []Tool{
		{
			Name:        "get_weather",
			Description: "获取指定城市的天气信息",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"city": map[string]interface{}{
						"type":        "string",
						"description": "城市名称",
					},
				},
				"required": []interface{}{"city"},
			},
			Handler: func(params map[string]interface{}) (string, error) {
				city, ok := params["city"].(string)
				if !ok {
					return "", fmt.Errorf("city parameter is required")
				}
				return fmt.Sprintf("当前%s的天气是晴天，温度25°C", city), nil
			},
		},
		{
			Name:        "calculate",
			Description: "执行数学计算",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"expression": map[string]interface{}{
						"type":        "string",
						"description": "数学表达式，如 2+3*4",
					},
				},
				"required": []interface{}{"expression"},
			},
			Handler: func(params map[string]interface{}) (string, error) {
				expr, ok := params["expression"].(string)
				if !ok {
					return "", fmt.Errorf("expression parameter is required")
				}
				result := evalSimpleExpression(expr)
				return fmt.Sprintf("计算结果: %s = %v", expr, result), nil
			},
		},
		{
			Name:        "get_time",
			Description: "获取当前时间",
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			Handler: func(params map[string]interface{}) (string, error) {
				return fmt.Sprintf("当前时间是 2024-01-01 12:00:00"), nil
			},
		},
	}
}

// evalSimpleExpression 简单表达式求值
func evalSimpleExpression(expr string) interface{} {
	return expr
}

// RAGConfig RAG搜索配置
type RAGConfig struct {
	BaseURL string
	APIKey  string
	Domain  string
}

// RAGClient RAG知识库搜索客户端
type RAGClient struct {
	baseURL string
	apiKey  string
	domain  string
	client  *http.Client
}

// NewRAGClient 创建RAG客户端
func NewRAGClient(baseURL, apiKey string) *RAGClient {
	return &RAGClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		domain:  "",
		client:  &http.Client{Timeout: 30 * 1e9},
	}
}

// NewRAGClientWithDomain 创建RAG客户端（带域名）
func NewRAGClientWithDomain(baseURL, apiKey, domain string) *RAGClient {
	return &RAGClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		domain:  domain,
		client:  &http.Client{Timeout: 30 * 1e9},
	}
}

// ChatRequest Python服务聊天请求
type ChatRequest struct {
	Messages []map[string]interface{} `json:"messages"`
	Query    string                   `json:"query"`
	Model    string                   `json:"model,omitempty"`
}

// ChatResponse Python服务聊天响应
type ChatResponse struct {
	Query      string `json:"query"`
	Candidates []struct {
		ID       string `json:"id"`
		Document string `json:"document"`
		Metadata struct {
			FileID    string `json:"fileId"`
			FileName  string `json:"fileName"`
			UserID    string `json:"userId"`
			SourceURL string `json:"source_url"`
		} `json:"metadata"`
	} `json:"candidates"`
}

// RAGSearchResponse RAG搜索结果响应（兼容旧格式）
type RAGSearchResponse struct {
	Answer  string `json:"answer"`
	Related []struct {
		Content string  `json:"content"`
		Source  string  `json:"source"`
		Score   float64 `json:"score"`
	} `json:"related"`
}

// SearchKnowledge 搜索RAG知识库
func (c *RAGClient) SearchKnowledge(query string) (string, error) {
	log.Printf("[RAGClient] 开始搜索知识库 | query: %s", query)

	reqBody := ChatRequest{
		Messages: []map[string]interface{}{
			{"role": "user", "content": query},
		},
		Query: query,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("请求序列化失败: %w", err)
	}

	log.Printf("[RAGClient] 请求数据: %s", string(jsonData))

	req, err := http.NewRequest("POST", c.baseURL+"/chat", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	log.Printf("[RAGClient] 响应状态: %d | body: %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("RAG服务返回错误状态 %d: %s", resp.StatusCode, string(body))
	}

	// 尝试解析新格式（RAG搜索结果）
	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err == nil {
		log.Printf("[RAGClient] 解析响应成功 | query: %s | candidates_count: %d", chatResp.Query, len(chatResp.Candidates))
		// 有candidates时构建下载链接
		if len(chatResp.Candidates) > 0 {
			var results []string
			for i := range chatResp.Candidates {
				candidate := &chatResp.Candidates[i]
				// 构建完整可下载URL
				fullURL := c.buildFullURL(candidate.Metadata.SourceURL)
				results = append(results, fmt.Sprintf("文件: %s, 下载链接: %s", candidate.Metadata.FileName, fullURL))
			}
			result := strings.Join(results, "\n")
			log.Printf("[RAGClient] 搜索完成 | result_len: %d", len(result))
			return result, nil
		}
	}

	// 尝试解析旧格式（ChatResponse）
	var oldResp RAGSearchResponse
	if err := json.Unmarshal(body, &oldResp); err == nil {
		log.Printf("[RAGClient] 旧格式解析成功 | answer_len: %d | related_count: %d", len(oldResp.Answer), len(oldResp.Related))
		if oldResp.Answer != "" {
			return oldResp.Answer, nil
		}
	}

	// 尝试解析简单响应
	var simpleResp map[string]interface{}
	if err2 := json.Unmarshal(body, &simpleResp); err2 == nil {
		if answer, ok := simpleResp["answer"].(string); ok {
			return answer, nil
		}
		if content, ok := simpleResp["content"].(string); ok {
			return content, nil
		}
	}

	return "", fmt.Errorf("解析响应失败: unknown format, body: %s", string(body))
}

// SearchTools 返回搜索相关工具集（用于信息检索场景）
func SearchTools(ragClient *RAGClient) []Tool {
	return []Tool{
		{
			Name: "search_knowledge",
			Description: "在用户的 RAG 知识库 / 私有文档库 中检索任意类型的资源，" +
				"包括 文本、文档、图片、图像、PDF、文件、视频 等。返回命中的文件名与可下载 URL。" +
				"只要用户的问题涉及'我的知识库/RAG/文档/图片/文件/资料'，必须优先调用本工具，不要跳过。" +
				"未命中时返回提示文本，此时可考虑回退到 web_search。",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type": "string",
						"description": "检索关键词，提炼用户想找的核心目标，例如：" +
							"'黄色小狗图片'、'2024 财报 PDF'、'产品介绍文档'。" +
							"不要带上'在我的库中'、'帮我找一下'这类无关修饰词。",
					},
				},
				"required": []interface{}{"query"},
			},
			Handler: func(params map[string]interface{}) (string, error) {
				query, ok := params["query"].(string)
				if !ok || query == "" {
					return "", fmt.Errorf("query parameter is required")
				}

				log.Printf("[SearchKnowledge] 开始RAG知识库搜索 | query: %s", query)

				if ragClient == nil {
					log.Printf("[SearchKnowledge] RAG客户端未初始化，使用模拟返回")
					return "知识库搜索功能暂不可用，请稍后重试或直接回答。", nil
				}

				result, err := ragClient.SearchKnowledge(query)
				if err != nil {
					log.Printf("[SearchKnowledge] 搜索失败 | error: %v", err)
					return fmt.Sprintf("知识库搜索失败: %v", err), err
				}

				// 检查返回内容是否表示未找到
				if isEmptyResult(result) {
					log.Printf("[SearchKnowledge] 知识库未找到相关内容")
					return "知识库中没有找到相关信息，AI将尝试其他方式获取答案。", nil
				}

				log.Printf("[SearchKnowledge] 搜索完成 | result_len: %d", len(result))
				return fmt.Sprintf("【知识库搜索结果】\n%s", result), nil
			},
		},
		{
			Name:        "web_search",
			Description: "当RAG知识库无法提供答案时，使用此工具进行网络搜索获取最新信息",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "搜索查询词",
					},
				},
				"required": []interface{}{"query"},
			},
			Handler: func(params map[string]interface{}) (string, error) {
				query, ok := params["query"].(string)
				if !ok || query == "" {
					return "", fmt.Errorf("query parameter is required")
				}
				log.Printf("[WebSearch] 网络搜索功能需要AI直接回答 | query: %s", query)
				return "网络搜索功能已禁用，请根据已有知识直接回答用户问题。", nil
			},
		},
	}
}

// buildFullURL 构建完整的可访问URL
func (c *RAGClient) buildFullURL(sourceURL string) string {
	if sourceURL == "" {
		return ""
	}
	// 如果sourceURL已经是完整URL，直接返回
	if strings.HasPrefix(sourceURL, "http://") || strings.HasPrefix(sourceURL, "https://") {
		return sourceURL
	}
	// 如果sourceURL已经包含了域名（包含hn-bkt.clouddn.com），直接加上https://
	if strings.Contains(sourceURL, "hn-bkt.clouddn.com") {
		return "http://" + sourceURL
	}
	// 否则拼上domain
	domain := c.domain
	if domain == "" {
		domain = "tfpdkiq9g.hn-bkt.clouddn.com"
	}
	return "http://" + domain + "/" + sourceURL
}

// isEmptyResult 判断返回内容是否表示未找到相关结果
func isEmptyResult(content string) bool {
	content = strings.ToLower(content)
	emptyIndicators := []string{"未找到", "没有找到", "没有相关信息", "不在知识库中", "no relevant", "not found", "没有结果", "我不知道", "不清楚"}
	for _, indicator := range emptyIndicators {
		if strings.Contains(content, indicator) {
			return true
		}
	}
	// 内容太短且没有明确答案指向
	if len(content) < 30 {
		return true
	}
	return false
}
