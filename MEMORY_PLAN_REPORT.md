# Echo Core · 分层记忆系统评估报告

> **版本**: v2.0  
> **日期**: 2026-06-23  
> **目标**: 为虚拟陪伴平台 Echo Core 设计并实现生产级分层记忆系统  
> **分支**: `plan`  
> **更新**: 数据库改为 MySQL + Qdrant，实现语言改为 Python（记忆系统全部在 Python AI 服务中）

---

## 目录

1. [开源记忆系统调研](#1-开源记忆系统调研)
2. [方案决策：开源接入 vs 自研](#2-方案决策开源接入-vs-自研)
3. [自研分层记忆系统详细设计方案](#3-自研分层记忆系统详细设计方案)
4. [Python 服务中实现的关键步骤](#4-python-服务中实现的关键步骤)
5. [总结与风险提示](#5-总结与风险提示)

---

## 1. 开源记忆系统调研

### 1.1 调研范围

对以下 6 个当前业界最主流的开源/商业记忆系统进行了深度调研：

| 项目 | 语言 | 协议 | 核心架构 | 定位 |
|------|------|------|----------|------|
| **Mem0** | Python | Apache 2.0 | 向量检索 + 图记忆(Mem0g) | 通用 Agent 记忆层 |
| **SuperMemory** | TypeScript/Python | 商业(可自托管) | 原子事实 + 知识图谱 + 时序 | 个人/团队记忆云 |
| **Letta (MemGPT)** | Python | Apache 2.0 | 内存层级(核心/召回/归档) | LLM OS 记忆管理 |
| **Zep** | Python | 商业(社区版免费) | 时序知识图谱(Graphiti) | 企业级 Agent 记忆 |
| **M3-Agent** | Python | Apache 2.0 | 双线程(情景+语义) + 多模态 | 多模态 Agent 记忆 |
| **OpenViking** | Python+Rust | AGPL-3.0 | 文件系统范式 + L0/L1/L2 | Agent 上下文数据库 |

### 1.2 各系统核心能力对比

| 能力维度 | Mem0 | SuperMemory | Letta | Zep | M3-Agent | OpenViking |
|----------|------|-------------|-------|-----|----------|------------|
| **向量检索** | ✅ 核心 | ✅ 混合 | ✅ 归档层 | ✅ | ✅ | ✅ |
| **知识图谱** | ✅ Mem0g | ✅ 核心 | ❌ | ✅ 核心 | ✅ 实体中心 | ❌ |
| **时序推理** | ❌ | ✅ 双层时间戳 | ❌ | ✅ 双时序 | ✅ | ❌ |
| **原子事实抽取** | ✅ | ✅ 核心 | ❌ | ✅ | ✅ | ❌ |
| **去重/合并** | ✅ LLM | ✅ 关系版本化 | ❌ | ✅ 图更新 | ✅ | ❌ |
| **记忆压缩** | ✅ | ✅ | ✅ 递归摘要 | ✅ | ✅ | ✅ L0/L1/L2 |
| **多模态** | ❌ | ✅ | ❌ | ❌ | ✅ 核心 | ❌ |
| **本地部署** | ✅ | ✅ SQLite | ✅ | ✅ | ✅ | ✅ |
| **Go SDK** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **评分(LOCOMO)** | 26%↑ vs OpenAI | 95% Recall@15 | 论文基准 | 63.8% vs Mem0 49% | 94.2% M3-Bench | N/A |

### 1.3 各系统与本项目的适配分析

#### Mem0
- **优势**: 生态最成熟(LangChain/CrewAI/AWS Strands)，Mem0g 图记忆增强，向量+图双检索，火山引擎有托管版
- **劣势**: 纯 Python，无 Go SDK，需要 HTTP 桥接，增加一跳延迟(~50ms)
- **适配度**: ★★★☆☆（架构好但语言不匹配）

#### SuperMemory
- **优势**: 原子事实+关系版本化，LongMemEval 第一名，时序推理能力强，支持时间旅行查询
- **劣势**: 核心为商业云服务，自托管复杂度高，TypeScript/Python 技术栈，无 Go SDK
- **适配度**: ★★☆☆☆（能力强但接入成本高，商业依赖风险）

#### Letta (MemGPT)
- **优势**: 学术先驱，内存层级设计经典，自编辑记忆理念先进，支持心跳式多步推理
- **劣势**: 已转型为商业平台 Letta Cloud，开源版维护放缓，无 Go SDK
- **适配度**: ★★☆☆☆（理念可借鉴，但不适合直接接入）

#### Zep
- **优势**: 时序知识图谱最强，双时序建模(valid_at + invalid_at)，企业级治理，S&P 认可
- **劣势**: 商业产品为主，社区版功能受限，Python 技术栈，无 Go SDK
- **适配度**: ★★☆☆☆（企业级但过于重量级，不适合初创陪伴平台）

#### M3-Agent (字节跳动)
- **优势**: 双线程架构(记忆/控制分离)，情景+语义双重记忆，实体中心设计，多模态支持
- **劣势**: 强依赖多模态(视频/音频)，架构偏重，与纯文本对话场景不完全匹配，Python 技术栈
- **适配度**: ★★☆☆☆（架构思想可借鉴，但不适合直接使用）

#### OpenViking (字节跳动/火山引擎)
- **优势**: 文件系统范式直观，L0/L1/L2 三级加载节省 Token 60-80%，目录递归检索，可视化轨迹
- **劣势**: AGPL-3.0 协议限制商用，Python+Rust 技术栈，与 Go 不兼容，项目较新
- **适配度**: ★★☆☆☆（协议风险，不适合直接集成）

---

## 2. 方案决策：开源接入 vs 自研

### 2.1 核心结论：**推荐自研，借鉴开源最佳实践**

### 2.2 决策理由

#### 理由一：语言栈不匹配（决定性因素）

Echo Core 是纯 Go 项目。所有主流开源记忆系统(Mem0/SuperMemory/Letta/Zep/M3-Agent/OpenViking)均为 Python 或 TypeScript 实现，**没有任何一个提供原生 Go SDK**。

如果接入开源方案，必须：
- 部署独立的 Python/Node.js 服务作为记忆 sidecar
- 通过 HTTP/gRPC 进行跨进程调用
- 每次记忆操作增加 1 跳网络延迟(~10-50ms) + 序列化开销
- 增加运维复杂度（两套部署、两套监控、两套日志）

对于虚拟陪伴这种对**首字时延**敏感的场景（当前 P0 优化目标就是 -30% 首字时延），额外引入网络跳是不可接受的。

#### 理由二：领域需求高度定制化

虚拟陪伴平台对记忆系统有特殊需求，开源方案无法直接满足：

| 需求 | 开源方案现状 | 自研优势 |
|------|-------------|----------|
| **情感记忆** | 均不支持情感维度标注 | 可设计情感衰减曲线、情感强度权重 |
| **关系演化** | 仅 Zep/M3-Agent 有时序 | 可原生建模"用户-陪伴者"双向关系 |
| **遗忘曲线** | 仅 SuperMemory 有基础支持 | 可实现 Ebbinghaus 遗忘曲线 + 情感加权 |
| **记忆优先级** | 无内置支持 | 可基于情感强度+访问频率+时效性三维排序 |
| **隐私边界** | 仅 Zep 有企业级治理 | 可设计用户可控的记忆可见性层级 |
| **与情感微模型联动** | 无 | 记忆系统可与情感模型共享 embedding、协同推理 |

#### 理由三：现有系统已有良好基础

Echo Core 当前已实现：
- `MemoryService` — 记忆抽取/合并/去重/注入
- `MemoryRepository` — GORM 持久化（MySQL）
- `Summarizer` — 增量摘要 + 滑动窗口
- `AIClient.GetTextEmbedding` — 向量化能力
- `PromptCache` — 前缀缓存

这些都是可直接升级的基础设施，不是从零开始。

#### 理由四：成本可控

| 方案 | 初期成本 | 运维成本 | Token 成本 | 总拥有成本 |
|------|---------|---------|-----------|-----------|
| 接入 Mem0 云服务 | 低 | 中(按月付费) | 中 | 中高 |
| 自部署开源方案 | 中(需要 Python 运维) | 高(两套系统) | 中 | 高 |
| **自研 Go 原生** | 中(开发 2-3 周) | 低(单一 Go 二进制) | 低(可精细优化) | **低** |

#### 理由五：可借鉴的最佳实践

虽然选择自研，但不是闭门造车。我们将从调研的开源方案中借鉴以下核心设计：

| 借鉴来源 | 借鉴的设计 | 应用方式 |
|----------|-----------|----------|
| **Mem0** | 向量+图双检索 | 短期：向量检索；中期：引入图关系 |
| **SuperMemory** | 原子事实 + 关系版本化 | 记忆粒度从"按 type 合并"升级为"原子事实+关系链" |
| **Letta** | 内存层级(核心/召回/归档) | 三层记忆架构(L0/L1/L2，借鉴 OpenViking) |
| **Zep** | 时序知识图谱 + 双时序 | 记忆带上 `valid_at`/`invalid_at` 时间戳 |
| **M3-Agent** | 记忆/控制双线程 | 记忆抽取异步化(已实现) + 增强后台记忆整理 |
| **OpenViking** | L0/L1/L2 三级加载 | 记忆注入按重要性分层，控制 Token 消耗 |

---

## 3. 自研分层记忆系统详细设计方案

### 3.1 总体架构

```
┌──────────────────────────────────────────────────────────────────────┐
│                     Echo Core · 分层记忆系统                           │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                    L0 · 核心记忆层 (Core Memory)              │   │
│  │  · 始终注入 Prompt (≤500 tokens)                              │   │
│  │  · 内容: 用户身份、核心偏好、当前情感状态、最近关键事件        │   │
│  │  · 存储: MySQL user_memory (memory_tier=0)                    │   │
│  │  · 更新: 实时(对话结束后异步抽取)                              │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                              │                                       │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                    L1 · 工作记忆层 (Working Memory)            │   │
│  │  · 按需检索注入 (≤1000 tokens)                                │   │
│  │  · 内容: 用户习惯、近期兴趣、关系状态、情境信息                │   │
│  │  · 存储: MySQL user_memory (memory_tier=1) + 向量索引          │   │
│  │  · 检索: 语义相似度 + 时间衰减 + 情感加权                      │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                              │                                       │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                    L2 · 归档记忆层 (Archival Memory)           │   │
│  │  · 精确查询时加载 (无上限)                                     │   │
│  │  · 内容: 完整对话历史、历史事件、已过时偏好                    │   │
│  │  · 存储: MySQL user_memory (memory_tier=2) + 全文索引          │   │
│  │  · 检索: 关键词/时间范围/实体查询                              │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                    记忆处理流水线 (Memory Pipeline)            │   │
│  │                                                                │   │
│  │  对话消息 ──▶ 1.原子事实抽取 ──▶ 2.向量化 ──▶ 3.去重/合并     │   │
│  │                                    │                           │   │
│  │                                    ▼                           │   │
│  │  记忆入库 ◀── 6.分层归档 ◀── 5.情感标注 ◀── 4.关系识别       │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

### 3.2 数据模型设计

#### 3.2.1 升级后的 UserMemory 模型

```go
// UserMemory 用户记忆 - 升级版分层记忆模型
type UserMemory struct {
    ID            uint      `json:"id" gorm:"primaryKey"`
    UserID        string    `json:"user_id" gorm:"index;size:64;not null"`
    MemoryType    string    `json:"memory_type" gorm:"size:50;not null"`
    // preference / info / knowledge / summary / event / emotion / relationship
    Content       string    `json:"content" gorm:"type:text;not null"`
    
    // === 分层记忆新增字段 ===
    MemoryTier    int       `json:"memory_tier" gorm:"default:1;index"`
    // 0=核心(始终注入), 1=工作(按需检索), 2=归档(精确查询)
    
    Importance    float64   `json:"importance" gorm:"default:0.5"`
    // 重要性评分 0.0~1.0，由 LLM 评估 + 访问频率 + 情感强度综合计算
    
    EmotionTag    string    `json:"emotion_tag" gorm:"size:50"`
    // 情感标签: positive/negative/neutral/nostalgic/anxious/excited 等
    
    EmotionIntensity float64 `json:"emotion_intensity" gorm:"default:0.0"`
    // 情感强度 0.0~1.0
    
    AccessCount   int       `json:"access_count" gorm:"default:0"`
    // 访问次数，用于重要性衰减/提升
    
    LastAccessAt  *time.Time `json:"last_access_at"`
    // 最后访问时间，用于遗忘曲线计算
    
    ValidFrom     time.Time `json:"valid_from" gorm:"not null"`
    // 记忆生效时间（借鉴 Zep 双时序设计）
    
    ValidUntil    *time.Time `json:"valid_until"`
    // 记忆失效时间，nil 表示持续有效
    
    // === 向量字段 ===
    EmbeddingJSON string    `json:"embedding_json" gorm:"type:mediumtext"`
    // JSON 格式的向量数据，用于语义检索
    
    // === 关系字段 ===
    ParentID      *uint     `json:"parent_id" gorm:"index"`
    // 父记忆 ID，用于构建记忆演化链（借鉴 SuperMemory 关系版本化）
    
    RelationType  string    `json:"relation_type" gorm:"size:50"`
    // 关系类型: update(更新) / contradict(矛盾) / extend(扩展) / derive(派生)
    
    // === 来源追溯 ===
    SourceSessionID string  `json:"source_session_id" gorm:"size:64;index"`
    // 来源会话 ID，用于追溯
    
    SourceMessageID uint    `json:"source_message_id" gorm:"default:0"`
    // 来源消息 ID
    
    CreatedAt     time.Time `json:"created_at" gorm:"autoCreateTime"`
    UpdatedAt     time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}
```

#### 3.2.2 新增 MemoryRelation 模型（关系链追踪）

```go
// MemoryRelation 记忆关系 - 用于追踪记忆演化（借鉴 SuperMemory 关系版本化）
type MemoryRelation struct {
    ID           uint      `json:"id" gorm:"primaryKey"`
    UserID       string    `json:"user_id" gorm:"index;size:64;not null"`
    FromMemoryID uint      `json:"from_memory_id" gorm:"index;not null"`
    ToMemoryID   uint      `json:"to_memory_id" gorm:"index;not null"`
    RelationType string    `json:"relation_type" gorm:"size:50;not null"`
    // update / contradict / extend / derive
    Description  string    `json:"description" gorm:"type:text"`
    // LLM 对关系变化的描述
    CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
}
```

#### 3.2.3 新增 EmotionMemoryIndex（情感记忆索引）

```go
// EmotionMemoryIndex 情感记忆索引 - 用于情感维度快速检索
type EmotionMemoryIndex struct {
    ID         uint      `json:"id" gorm:"primaryKey"`
    UserID     string    `json:"user_id" gorm:"index;size:64;not null"`
    MemoryID   uint      `json:"memory_id" gorm:"index;not null"`
    EmotionTag string    `json:"emotion_tag" gorm:"size:50;index;not null"`
    Intensity  float64   `json:"intensity" gorm:"default:0.0"`
    CreatedAt  time.Time `json:"created_at" gorm:"autoCreateTime"`
}
```

### 3.3 记忆抽取流水线

#### 阶段一：原子事实抽取

借鉴 SuperMemory 的"原子事实"理念，将现有 `extractWithLLM` 的抽取粒度从"按 type 合并"升级为"原子事实级"：

```
当前做法:
  用户说"我喜欢咖啡，每天早上喝，不加糖"
  → 抽取为: {type: "preference", content: "用户喜欢咖啡，每天早上喝，不加糖"}

升级后:
  → 抽取为 3 条原子事实:
    1. {type: "preference", content: "用户喜欢咖啡"}
    2. {type: "habit", content: "用户每天早上喝咖啡"}
    3. {type: "preference", content: "用户喝咖啡不加糖"}
```

每条原子事实独立存储，独立管理生命周期，独立参与检索。

#### 阶段二：向量化

```
1. 对每条原子事实调用 AIClient.GetTextEmbedding()
2. 向量存入 UserMemory.EmbeddingJSON
3. 可选：引入本地 embedding 模型（如 Ollama bge-small）降低延迟和成本
```

#### 阶段三：语义去重与关系识别

借鉴 SuperMemory 的关系版本化 + Mem0 的 LLM 判重：

```
1. 向量检索 Top-K 候选（K=5）
2. LLM 判断新记忆与候选的关系：
   - duplicate: 内容完全相同 → 跳过
   - update: 同一事实的更新版本 → 旧记忆标记 expired，建立 update 关系
   - contradict: 与旧记忆矛盾 → 旧记忆标记 expired，记录矛盾关系
   - extend: 补充旧记忆 → 建立 extend 关系，两条都保留
   - new: 全新事实 → 直接入库
3. 写入 MemoryRelation 表记录关系链
```

#### 阶段四：情感标注

```
1. 对每条记忆调用 LLM 进行情感分析
2. 输出: {emotion_tag, emotion_intensity, importance}
3. 写入 UserMemory.EmotionTag / EmotionIntensity / Importance
4. 写入 EmotionMemoryIndex 辅助索引
```

#### 阶段五：分层归档

```
L0 判定条件（满足任一）:
  - Importance >= 0.8
  - 记忆类型为 preference 且 EmotionIntensity >= 0.7
  - 用户身份信息（name, age, occupation 等）

L1 判定条件（默认，不满足 L0/L2 的归入此层）:
  - 0.3 <= Importance < 0.8
  - 近期（30天内）产生或更新的记忆

L2 判定条件（满足任一）:
  - Importance < 0.3
  - ValidUntil 已过期
  - 超过 90 天未访问且 Importance < 0.5
```

### 3.4 记忆检索与注入

#### 检索策略

```
对话开始时:
  1. L0 核心记忆: 全量加载（≤500 tokens）
  2. L1 工作记忆: 语义检索 Top-N（N=5, ≤1000 tokens）
     检索得分 = 语义相似度(0.5) + 时间衰减(0.2) + 情感强度(0.2) + 重要性(0.1)
  3. L2 归档记忆: 不主动加载，LLM 可通过 search_memory 工具按需查询
```

#### 时间衰减函数

```
decay = e^(-λ * days_since_last_access)

其中 λ 按记忆类型调整:
  - preference: λ=0.01 (偏好衰减慢)
  - event:      λ=0.05 (事件衰减快)
  - knowledge:  λ=0.005 (知识衰减最慢)
  - emotion:    λ=0.03 (情感记忆衰减中等)
```

#### 遗忘曲线

借鉴 Ebbinghaus 遗忘曲线，结合情感强度调整：

```
retention_rate = e^(-t/S)

其中 S = base_strength * (1 + emotion_intensity * 2)
  - base_strength: 基础记忆强度（默认 1.0）
  - emotion_intensity: 情感强度加成（高情感记忆遗忘更慢）
  - t: 天数
```

### 3.5 记忆后台整理（Memory Consolidation）

借鉴 M3-Agent 的"记忆流程"后台线程 + Letta 的"sleep-time agent"：

```go
// MemoryConsolidator 记忆后台整理器
type MemoryConsolidator struct {
    memService *MemoryService
    aiClient   *remote.AIClient
    ticker     *time.Ticker
}

// 定时任务（每小时执行一次）:
// 1. 扫描 L1 中超过 30 天未访问的记忆 → 降级到 L2
// 2. 扫描同一用户同类型记忆 → 合并相似项
// 3. 检查矛盾记忆 → 标记旧版本 expired
// 4. 更新 Importance 评分（基于 AccessCount + 情感强度衰减）
// 5. 清理 ValidUntil 已过期的记忆 → 标记为 L2 历史
```

---

## 4. Python 服务中实现的关键步骤

> **注意**: 记忆系统全部在 Python AI 服务中实现。Go 侧只保留会话管理，不再包含记忆处理逻辑。

### 4.1 实施路线图

```
Phase 1 (基础设施)          Phase 2 (记忆引擎)          Phase 3 (智能进阶)
┌─────────────────┐       ┌─────────────────┐       ┌─────────────────┐
│ 1. MySQL 表结构   │  ──▶  │ 5. 向量检索器     │  ──▶  │ 9. 记忆关系图谱   │
│ 2. Qdrant 部署    │       │ 6. Embedding 优化 │       │ 10. 记忆后台整理  │
│ 3. 原子事实抽取   │       │ 7. 语义去重升级   │       │ 11. 遗忘曲线      │
│ 4. 分层归档逻辑   │       │ 8. 情感标注系统   │       │ 12. 记忆压缩      │
└─────────────────┘       └─────────────────┘       └─────────────────┘
    第 1-2 周                   第 3-4 周                   第 5-6 周
```

### 4.2 Phase 1: 基础设施（第 1-2 周）

#### 步骤 1: MySQL 表结构创建

**文件**: `migrations/001_create_memory_tables.sql`

- [ ] 创建 `identity_memory` 表（身份记忆，1 条/用户，永久 L0）
- [ ] 创建 `episodic_memory` 表（情景记忆，核心表，L0/L1/L2 分层）
- [ ] 创建 `perceptual_memory` 表（感知记忆，穿戴设备帧，30 天归档）
- [ ] 创建 `summary_memory` 表（摘要记忆，LLM 定期压缩生成）
- [ ] 创建 `persona_memory` 表（人格记忆，虚拟人物设定）
- [ ] 创建 `memory_relations` 表（跨表关系：因果/更新/矛盾）
- [ ] 添加索引：user_id + tier / category / emotion / captured_at

#### 步骤 2: Qdrant 部署与集合创建

**文件**: `scripts/setup_qdrant.py`

- [ ] 部署 Qdrant 服务（Docker 或云服务）
- [ ] 创建文本向量集合 `episodic_memory`（768 维，BGE-M3）
- [ ] 创建图片向量集合 `multimodal_image`（512 维，CLIP）
- [ ] 创建音频向量集合 `multimodal_audio`（256 维，Whisper）
- [ ] 创建视频向量集合 `multimodal_video`（512 维，VideoMAE）
- [ ] 配置 HNSW 索引参数

#### 步骤 3: 原子事实抽取

**文件**: `services/memory/extractor.py`

- [ ] 实现 `extract_atom_facts(user_msg, asst_reply) → List[AtomicFact]`
- [ ] LLM prompt 要求以"原子事实"粒度输出
- [ ] 每条原子事实独立存储（不再按 type 合并）
- [ ] 同时抽取因果关系：causes/leads_to/related_to/contradicts
- [ ] 输出格式校验 + 容错处理

#### 步骤 4: 分层归档逻辑

**文件**: `services/memory/tier.py`

- [ ] 实现 `classify_tier(fact, emotion_tag, intensity) → int`（L0/L1/L2 判定）
- [ ] 实现 `calculate_importance(fact) → float`
- [ ] L0 判定规则：importance >= 0.8，或身份信息，或情感强度 >= 0.7
- [ ] L2 判定规则：importance < 0.3，或 ValidUntil 已过期

### 4.3 Phase 2: 记忆引擎（第 3-4 周）

#### 步骤 5: 向量检索器

**文件**: `services/memory/retriever.py`

- [ ] 实现 `semantic_search(user_id, query, tier, top_k) → List[Memory]`
- [ ] Qdrant 文本向量检索
- [ ] 检索得分公式：语义相似度(0.5) + 时间衰减(0.2) + 情感强度(0.2) + 重要性(0.1)
- [ ] 实现时间衰减函数：`decay = e^(-λ * days_since_last_access)`
- [ ] 多模态检索：跨模态向量查询

#### 步骤 6: Embedding 优化

**文件**: `services/embedding/service.py`

- [ ] 实现 embedding 结果缓存（Redis，TTL 24h）
- [ ] 批量 embedding 支持（减少 API 调用次数，攒批 10 条）
- [ ] 本地 embedding 模型评估（Ollama + bge-small）
- [ ] 多模态 embedding：CLIP（图片）、Whisper（音频）、VideoMAE（视频）

#### 步骤 7: 语义去重升级

**文件**: `services/memory/dedup.py`

- [ ] 实现 `resolve_relations(user_id, embedding, content) → RelationType`
- [ ] Qdrant 向量召回候选（Top-3）→ LLM 判重
- [ ] 五种关系处理：duplicate（跳过）/ update（标记过期）/ contradict（标记过期）/ extend（保留）/ new（新建）
- [ ] 写入 `memory_relations` 表

#### 步骤 8: 情感标注系统

**文件**: `services/memory/emotion.py`

- [ ] 实现 `annotate_emotion(content) → (tag, intensity)`
- [ ] LLM 情感分析 prompt
- [ ] 写入 `episodic_memory.emotion_tag` / `emotion_intensity`
- [ ] 情感维度检索 API

### 4.4 Phase 3: 智能进阶（第 5-6 周）

#### 步骤 9: 记忆关系图谱

**文件**: `services/memory/graph.py`

- [ ] 实现因果链查询（MySQL 递归 CTE）
- [ ] 实现记忆演化链：`get_memory_chain(memory_id)`
- [ ] 实现用户记忆关系图：`get_user_memory_graph(user_id)`
- [ ] 为 LLM 格式化因果链文本

#### 步骤 10: 记忆后台整理

**文件**: `services/memory/consolidator.py`

- [ ] 实现 `MemoryConsolidator`（APScheduler 定时任务）
- [ ] 每天凌晨：L1 超过 30 天未访问 → 降级 L2
- [ ] 每天凌晨：LLM 生成"今日日记" → summary_memory
- [ ] 每周：相似记忆合并
- [ ] 每周：矛盾记忆清理，旧版本标记过期
- [ ] 每月：perceptual_memory 压缩归档

#### 步骤 11: 遗忘曲线

**文件**: `services/memory/decay.py`

- [ ] 实现 `calculate_retention_rate(memory) → float` — Ebbinghaus 遗忘曲线
- [ ] 实现 `calculate_decay_score(memory) → float` — 时间衰减得分
- [ ] 在检索排序中集成遗忘曲线得分
- [ ] 自动降级：retention_rate < 0.3 → L2

#### 步骤 12: 记忆压缩

**文件**: `services/memory/compressor.py`

- [ ] 实现 `compress_daily(user_id, date) → SummaryMemory`
- [ ] 读取当天 episodic + perceptual 记忆
- [ ] LLM 生成"今日日记"摘要
- [ ] 选择代表性关键帧
- [ ] 生成多模态嵌入
- [ ] 原始数据标记为可归档

### 4.5 跨切关注点

#### 性能优化

| 优化项 | 方案 | 预期收益 |
|--------|------|----------|
| Embedding 缓存 | Redis 缓存 | -50% embedding API 调用 |
| 批量 embedding | 攒批 10 条一起发送 | -80% 网络往返 |
| L0 记忆预加载 | 用户登录时预热（Redis） | 首字时延 -100ms |
| 向量检索 | Qdrant HNSW 索引 | 检索 <10ms |
| 异步抽取 | 对话后异步处理 | 0ms 主链路影响 |

#### 监控指标

| 指标 | 类型 | 含义 |
|------|------|------|
| `memory_extract_total` | counter | 记忆抽取次数 |
| `memory_extract_duration_ms` | histogram | 抽取耗时 |
| `memory_tier_distribution` | gauge | 各层记忆数量分布 |
| `memory_duplicate_rate` | gauge | 去重前后记忆比例 |
| `memory_retrieve_duration_ms` | histogram | 检索耗时 |
| `memory_l0_token_count` | gauge | L0 注入 token 数 |
| `memory_l1_token_count` | gauge | L1 注入 token 数 |
| `memory_consolidation_duration_ms` | histogram | 后台整理耗时 |

#### 测试策略

| 测试类型 | 覆盖内容 |
|----------|----------|
| 单元测试 | 分层判定、重要性计算、去重逻辑、遗忘曲线 |
| 集成测试 | 记忆抽取→归档→检索→注入 全链路 |
| 基准测试 | 不同规模下的检索性能(100/1000/10000 条记忆) |
| 对比测试 | 新系统 vs 旧系统的 Token 节省率、检索准确率 |

---

## 5. 总结与风险提示

### 5.1 决策总结

| 决策项 | 结论 | 核心理由 |
|--------|------|----------|
| 开源 vs 自研 | **自研** | Python 生态统一管理、领域需求定制化、Go 侧只做会话管理 |
| 实现语言 | **Python** | LLM 编排、向量计算、记忆抽取在 Python 进程内完成，零网络开销 |
| 架构设计 | **三层记忆(L0/L1/L2)** | 借鉴 OpenViking 分层 + Letta 内存层级 |
| 数据库 | **MySQL + Qdrant** | MySQL 存关系型数据，Qdrant 做全模态向量检索 |
| 记忆粒度 | **原子事实** | 借鉴 SuperMemory，提升检索精度 + 去重效果 |
| 记忆表设计 | **5 实体表 + 1 关系表** | 按生命周期和读写模式分表，各司其职 |
| 去重策略 | **向量召回 + LLM 判重** | 借鉴 Mem0，解决当前 Contains 粗粒度问题 |
| 时序管理 | **双时序(valid_from/valid_until)** | 借鉴 Zep，支持记忆时效性推理 |
| 关系追踪 | **关系版本化 + 因果链** | 借鉴 SuperMemory，支持 update/contradict/causes |
| 情感集成 | **情感标签+强度** | 为本项目"情感微模型"预留接口 |
| 后台整理 | **定时 Consolidator** | 借鉴 M3-Agent 记忆线程 + Letta sleep-time agent |

### 5.2 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| LLM 抽取质量不稳定 | 脏记忆膨胀 | 严格的 prompt 约束 + 输出格式校验 + 人工抽查 |
| 向量检索性能瓶颈 | 检索延迟增加 | 分阶段：先 Qdrant HNSW 索引（<10ms），多模态集合独立调优 |
| 记忆爆炸（用户量增长） | 存储和检索成本上升 | L2 归档策略 + 遗忘曲线自动清理 + 每用户记忆上限 + LLM 定期压缩 |
| 情感标注偏差 | 检索结果偏离用户真实状态 | 允许用户手动修正情感标签 + 多轮对话交叉验证 |
| 与情感微模型集成延迟 | 两系统信息不同步 | 预留共享 embedding 接口 + 统一事件总线 |

### 5.3 下一步行动

1. **立即开始**: Phase 1 数据模型升级（预计 2-3 天）
2. **并行推进**: 情感微模型方案调研，与记忆系统设计对齐接口
3. **验证节点**: Phase 1 完成后，用 10 个测试用户验证分层注入效果
4. **里程碑**: 6 周内完成全部 3 个 Phase，交付完整分层记忆系统

---

## 附录

### A. 开源项目参考链接

- **Mem0**: https://github.com/mem0ai/mem0 | 论文: arXiv:2504.19413
- **SuperMemory**: https://supermemory.ai/research | https://github.com/supermemoryai
- **Letta (MemGPT)**: https://docs.letta.com | 论文: arXiv:2310.08560
- **Zep**: https://github.com/getzep/zep | https://www.getzep.com
- **M3-Agent**: https://github.com/ByteDance-Seed/M3-Agent-Memorization
- **OpenViking**: https://github.com/volcengine/OpenViking

### B. 关键术语对照

| 术语 | 英文 | 说明 |
|------|------|------|
| 原子事实 | Atomic Fact | 不可再分的单一事实单元 |
| 关系版本化 | Relational Versioning | 追踪记忆间的 update/contradict/extend 关系 |
| 双时序 | Bi-temporal | valid_at(事实真实时间) + transaction_at(记录时间) |
| 遗忘曲线 | Forgetting Curve | Ebbinghaus 提出的记忆衰减数学模型 |
| 记忆后台整理 | Memory Consolidation | 后台定期整理、合并、降级记忆的过程 |
| 分层记忆 | Tiered Memory | L0(核心)/L1(工作)/L2(归档) 三级分层 |