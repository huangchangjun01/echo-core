# M2 / M3 / M6 端到端测试用例

> 配套实施 PR,落地后逐条跑通打勾。账号:`echo_test_001`(已存在)。

---

## M2: Mood prompt 注入

| # | 用例 | 期望 | 通过 |
|---|---|---|---|
| M2-1 | 启动 echo-ai 服务,新建一个空 mood 行(0/0/neutral) | DB 有 `role_mood` 行,build_segments 的 `mood` 段 = `""`(空串,中性 fallback) | ☐ |
| M2-2 | 触发一次 negative event(impact=-0.5 intensity=0.5),三桶公式更新 instant | GET /role-core/mood instant_val < 0 | ☐ |
| M2-3 | 调 `/api/chat` 一次,Python 侧日志确认 `build_segments.ok` 含 `mood` key 且非空 | 日志 non_empty 含 `["mood"]` | ☐ |
| M2-4 | 给一个 negative 心情,调一次 chat,确认 reply 语气偏向"低能量/安慰"风格(LLM 风格化,接受肉眼判定) | reply 含安抚性词 | ☐ |

---

## M3: Relationship 累积

| # | 用例 | 期望 | 通过 |
|---|---|---|---|
| M3-1 | GET /role-core/relationship 新角色 | 200 + intimacy=0 trust=0 satisfaction=0 coDays=0 | ☐ |
| M3-2 | POST /role-core/relationship/event `eventType=self_disclosure` | 200 + intimacy=0.05(信任/满意度不变) | ☐ |
| M3-3 | 再来一次 self_disclosure(同一天) | intimacy=0.10(累积) | ☐ |
| M3-4 | `eventType=positive_feedback` | trust=0.02, satisfaction=0.02 | ☐ |
| M3-5 | `eventType=negative_feedback` 累积 30 次 | trust 不会 ≤ 0(下限 clamp),trust ≈ 0.6 (1 - 30×0.02 = 0.4,但 clamp 在 [0,1],由于已有 trust=0.04 应 ≈ 0)→ 验证 clamp 行为 | ☐ |
| M3-6 | `eventType=co_days_7` | intimacy+0.03, trust+0.02, satisfaction+0.02 | ☐ |
| M3-7 | `eventType=invalid` | 400 拒绝(DTO oneof) | ☐ |
| M3-8 | 跨用户越权:用户 A 触发 event 想影响 B | 404 / 不影响 B 的记录 | ☐ |
| M3-9 | audit_log 写入 `action=update target_type=relationship delta_summary 含 event=...` | DB 验证 | ☐ |

---

## M3: Relationship 段 prompt 注入

| # | 用例 | 期望 | 通过 |
|---|---|---|---|
| M3-10 | 累积后调 chat,Python 日志确认 build_segments.mood+relationship 都有非空 | 日志 non_empty 包含 "mood","relationship" | ☐ |
| M3-11 | prompt 中含 "【关系】亲密 X.XX · 信任 X.XX · ..." | 内容正确(从 audit 日志看 system prompt) | ☐ |

---

## M3: Belief summary 注入

| # | 用例 | 期望 | 通过 |
|---|---|---|---|
| M3-12 | 用户手动新增 3 条信念(高/中/低 confidence 各一) | DB 有 3 条 user_confirmed=1 | ☐ |
| M3-13 | 调 chat,Python 日志确认 build_segments.belief_summary 非空 | 日志 non_empty 包含 belief_summary | ☐ |
| M3-14 | 5 分钟内重复调 chat,只查一次 DB(走缓存) | DB 查询日志只 1 条 | ☐ |
| M3-15 | 超过 5 分钟再调,缓存失效重查 | DB 查询日志增加 | ☐ |
| M3-16 | 调一次 API 删一条信念 → 缓存未刷新但下一轮 chat 不报错(用空串 fallback) | chat 200 OK | ☐ |

---

## M6: Suggestion apply 主表落地

| # | 用例 | 期望 | 通过 |
|---|---|---|---|
| M6-1 | 创建一条 `target_type=persona` 的 suggestion,suggestion_json 含 identity/background | suggestion 创建成功,status=pending | ☐ |
| M6-2 | resolve suggestion `action=apply` | status=applied + role_persona 行更新(identity/background 写入) | ☐ |
| M6-3 | 创建 `target_type=traits` suggestion,JSON 含 OCEAN 5 维 | apply 后 role_traits 更新 | ☐ |
| M6-4 | 创建 `target_type=belief` suggestion,JSON 含新 topic/stance | apply 后 role_belief 新增一行 | ☐ |
| M6-5 | 创建 `target_type=mood` suggestion,JSON 含三桶值 | apply 后 role_mood 更新(若无则 INSERT) | ☐ |
| M6-6 | resolve suggestion `action=skip` | status=skipped,主表不动 | ☐ |
| M6-7 | resolve suggestion `action=edit` 带 editedJson | suggestion_json 更新,status=edited | ☐ |
| M6-8 | apply 后 audit_log 写入含 target_type 与 summary | DB 验证 | ☐ |
| M6-9 | 跨用户越权:用户 B 尝试 apply 用户 A 的 suggestion | 404 | ☐ |
| M6-10 | apply 不存在的 suggestion id | 404 | ☐ |
| M6-11 | apply 时 suggestion_json 格式错误(target_type 与 JSON 不匹配) | 400 友好提示,不崩库 | ☐ |

---

## 自动化

`docs/M2-M3-M6-test-plan.md` 是手工 case,后续可转 vitest(前端) + go test(后端) 自动化。

跑通后,服务清理(避免 8080/5173 端口长期占用)。