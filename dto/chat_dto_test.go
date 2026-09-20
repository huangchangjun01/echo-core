package dto

import (
	"encoding/json"
	"testing"
)

// TestChatEvent_StageEvent_Marshaling 验证 ChatEvent 三个新字段（stage / emotion / relation）
// 在 JSON 序列化后能被前端按 spec §3.2 解析。
//
// 注意：ChatEvent 同一 JSON 层级只能有一个字段打 "stage" tag，因此：
//   - 原 thinking 事件的 stage 字符串已迁移到 ThinkStage 字段（tag "think_stage"）
//   - 新的 stage 事件用 Stage *StageEvent 字段（tag "stage"）
//
// 这是 Phase 2 streaming UX 重构的一部分（设计文档 §3.2 / §5.3）。
func TestChatEvent_StageEvent_Marshaling(t *testing.T) {
	cases := []struct {
		name    string
		ev      ChatEvent
		wantKey string
		wantVal any
	}{
		{
			name: "stage event",
			ev: ChatEvent{
				Type:  "stage",
				Stage: &StageEvent{Name: "intent", State: "start", TsMs: 1737038400123},
			},
			wantKey: "name",
			wantVal: "intent",
		},
		{
			name: "emotion_change event",
			ev: ChatEvent{
				Type:    "emotion_change",
				Emotion: &EmotionChange{From: "neutral", To: "joy", Intensity: 0.62},
			},
			wantKey: "to",
			wantVal: "joy",
		},
		{
			name: "relation_change event",
			ev: ChatEvent{
				Type:     "relation_change",
				Relation: &RelationChange{IntimacyDelta: 0.02, TrustDelta: 0.0, SatisfactionDelta: 0.01, Source: "stage:answer"},
			},
			wantKey: "source",
			wantVal: "stage:answer",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.ev)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			// Locate the payload field by event type
			var payload map[string]any
			switch tc.ev.Type {
			case "stage":
				payload, _ = got["stage"].(map[string]any)
			case "emotion_change":
				payload, _ = got["emotion"].(map[string]any)
			case "relation_change":
				payload, _ = got["relation"].(map[string]any)
			}
			if payload == nil {
				t.Fatalf("payload missing in %s", string(data))
			}
			if payload[tc.wantKey] != tc.wantVal {
				t.Errorf("%s.%s = %v, want %v", tc.ev.Type, tc.wantKey, payload[tc.wantKey], tc.wantVal)
			}
		})
	}
}

// TestStageEvent_RoundTrip 验证 StageEvent 三个字段（name / state / ts_ms）序列化后回环一致。
func TestStageEvent_RoundTrip(t *testing.T) {
	in := StageEvent{Name: "answer", State: "end", TsMs: 1737038400999}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["name"] != "answer" {
		t.Errorf("name = %v, want answer", got["name"])
	}
	if got["state"] != "end" {
		t.Errorf("state = %v, want end", got["state"])
	}
	// ts_ms 序列化为 JSON 数字 → float64
	if v, ok := got["ts_ms"].(float64); !ok || int64(v) != 1737038400999 {
		t.Errorf("ts_ms = %v, want 1737038400999", got["ts_ms"])
	}
}

// TestEmotionChange_OmitEmptyReason 验证 Reason 字段 omitempty 在空值时不污染 JSON。
func TestEmotionChange_OmitEmptyReason(t *testing.T) {
	in := EmotionChange{From: "neutral", To: "joy", Intensity: 0.62}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := got["reason"]; present {
		t.Errorf("reason should be omitted when empty, got %s", string(data))
	}
}

// TestChatEvent_OmitEmptyNewFields 验证 ChatEvent 中未填充的新指针字段不出现在 JSON 输出里，
// 这样旧版 echo-ai 客户端（不发送 stage/emotion/relation）不会产生空 payload。
func TestChatEvent_OmitEmptyNewFields(t *testing.T) {
	ev := ChatEvent{Type: "delta", Text: "hi"}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"stage", "emotion", "relation"} {
		if _, present := got[k]; present {
			t.Errorf("expected %q to be omitted, got %s", k, string(data))
		}
	}
}

func TestChatEvent_ContextRoundTripPreservesL2(t *testing.T) {
	// context 帧 l2_count / l2_items（echo-ai 新下发）必须经 ChatEvent 原样透传，
	// 否则 echo-core 会把这两个键丢掉，前端意图胶囊收不到 L2 会话摘要。
	l2c := 7
	ev := ChatEvent{
		Type:       "context",
		PersonaLen: intPtr(106),
		L0Count:    intPtr(3),
		L1Count:    intPtr(12),
		L2Count:    &l2c,
		Persona:    "人格原文",
		L0Items:    []string{"L0-1"},
		L1Items:    []string{"[L1] a"},
		L2Items:    []string{"[L2] 父摘要A", "[L2] 父摘要B"},
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, ok := got["l2_count"].(float64); !ok || int(v) != 7 {
		t.Errorf("l2_count = %v, want 7", got["l2_count"])
	}
	items, ok := got["l2_items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("l2_items = %v, want 2 items", got["l2_items"])
	}
	if items[0] != "[L2] 父摘要A" {
		t.Errorf("l2_items[0] = %v", items[0])
	}
	// 反向：模拟 Python context 帧 JSON 反序列化，确认字段不丢
	py := []byte(`{"type":"context","l2_count":2,"l2_items":["[L2] X"],"l1_count":1,"l1_items":["[L1] Y"]}`)
	var back ChatEvent
	if err := json.Unmarshal(py, &back); err != nil {
		t.Fatalf("unmarshal py: %v", err)
	}
	if back.L2Count == nil || *back.L2Count != 2 {
		t.Errorf("back.L2Count = %v, want 2", back.L2Count)
	}
	if len(back.L2Items) != 1 || back.L2Items[0] != "[L2] X" {
		t.Errorf("back.L2Items = %v", back.L2Items)
	}
}

func intPtr(v int) *int { return &v }

func TestChatEvent_ThinkingUsesThinkStage(t *testing.T) {
	ev := ChatEvent{
		Type:       "thinking",
		ThinkStage: "intent",
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["think_stage"] != "intent" {
		t.Errorf("think_stage = %v, want \"intent\"", got["think_stage"])
	}
	if _, ok := got["stage"]; ok {
		t.Errorf("legacy top-level stage key should not appear: %s", string(data))
	}
}
