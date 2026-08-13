package utils

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// StringOrNumber JSON 字段类型：同时接受字符串和数字，统一规整为字符串。
//
// 用途：用于 ID 等"客户端可能传字符串也可能传数字"的入参。
// 典型场景：
//   - 前端 TypeScript 里 id 类型声明为 number（数组下标、DB 自增 ID），
//     axios 序列化时直接发 number；
//   - 后端若把字段声明为 string，json.Unmarshal 在类型不匹配时直接报错 → 400。
//
// 用法：
//
//	type Req struct {
//	    ResourceID utils.StringOrNumber `json:"resourceId"`
//	}
//	// service 拿到的就是 string：
//	rid := string(req.ResourceID)
//
// 接受形态：
//   - "97"      → "97"
//   - 97        → "97"
//   - 97.0      → "97"（整数部分；小数会被截断，避免误用）
//   - ""        → ""（保留为字符串零值，由业务层校验）
//   - nil       → ""（同上）
//   - 其余      → 报错
type StringOrNumber string

// UnmarshalJSON 实现 json.Unmarshaler 接口。
func (s *StringOrNumber) UnmarshalJSON(data []byte) error {
	// 先尝试 null
	if string(data) == "null" {
		*s = ""
		return nil
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	switch x := v.(type) {
	case string:
		*s = StringOrNumber(x)
		return nil
	case float64:
		// JSON 数字默认解码为 float64。整数 ID 用 int64 安全范围截断；
		// 小数视为非法输入（ID 不应是小数）。
		if x != float64(int64(x)) {
			return fmt.Errorf("StringOrNumber 不接受小数: %v", x)
		}
		*s = StringOrNumber(strconv.FormatInt(int64(x), 10))
		return nil
	case json.Number:
		// 一些场景下 json.Decoder.UseNumber() 会让数字以 json.Number 形式落地
		*s = StringOrNumber(string(x))
		return nil
	}
	return fmt.Errorf("StringOrNumber: 期望 string 或 number，实际类型 %T", v)
}
