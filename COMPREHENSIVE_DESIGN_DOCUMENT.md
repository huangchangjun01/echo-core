# Echo Core · 虚拟陪伴平台综合设计文档

> **版本**: v3.0  
> **日期**: 2026-06-23  
> **目标**: 整合记忆系统、情感微模型、对话架构的完整技术方案  
> **分支**: `plan`

---

## 目录

1. [项目概述与核心定位](#1-项目概述与核心定位)
2. [系统总体架构](#2-系统总体架构)
3. [记忆分层系统详细设计](#3-记忆分层系统详细设计)
4. [情感微模型训练方案](#4-情感微模型训练方案)
5. [对话架构设计](#5-对话架构设计)
6. [多模态数据与穿戴设备集成](#6-多模态数据与穿戴设备集成)
7. [存储架构与数据库选型](#7-存储架构与数据库选型)
8. [技术选型决策记录](#8-技术选型决策记录)
9. [实施路线图](#9-实施路线图)

---

## 1. 项目概述与核心定位

### 1.1 产品定义

Echo Core 是一个虚拟情感陪伴平台，核心目标是让用户感受到"像真人一样被理解、被记住、被陪伴"。平台包含两大核心子系统：

- **记忆分层系统**：让虚拟人物拥有类似人类的记忆能力（短期/长期/情感记忆）
- **情感微模型**：让虚拟人物拥有温暖、共情的对话风格

### 1.2 关键产品特性

| 特性 | 描述 |
|------|------|
| 虚拟人物创建 | 用户可导入现实中的人物介绍文本、图片、音频、视频来塑造虚拟人物 |
| 多模态感知 | 通过穿戴设备（智能眼镜等），虚拟人物实时看到/听到用户所感知的内容 |
| 永久记忆 | 长期保留用户与虚拟人物的交互历史，支持跨会话记忆 |
| 因果推理 | 基于历史记忆推导逻辑关系，形成完整的因果链 |
| 情感陪伴 | 通过自研情感微模型，提供温暖、共情的对话体验 |

### 1.3 技术栈概览

| 层级 | 技术选型 |
|------|---------|
| 后端服务 | Go (Echo Core) — 会话管理、消息持久化、HTTP/WS 协议处理 |
| AI 服务 | Python — 记忆计算、对话逻辑、模型推理、嵌入生成 |
| 关系型数据库 | MySQL — 会话消息、业务数据、记忆本体（5 张表） |
| 向量数据库 | Qdrant — 文本/图片/音频/视频全模态向量索引 |
| 对象存储 | MinIO / 阿里云OSS — 多媒体文件（图片/音频/视频/帧） |
| 缓存 | Redis — 活跃会话、热点记忆 |
| 消息队列 | Kafka — 穿戴设备事件流 |
| 基座模型 | Qwen2.5-7B/14B-Instruct |
| 向量模型 | BGE / CLIP |

---

## 2. 系统总体架构

### 2.1 核心设计原则

**Go 管会话，Python 管大脑。** 一次对话只有一次 Go→Python HTTP 调用，所有记忆计算、LLM 推理、工具调用、ReAct 循环都在 Python 进程内完成。

### 2.2 服务分层架构

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           用户设备层                                     │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────────────┐  │
│  │ 手机 App    │  │ 智能眼镜    │  │ 其他穿戴设备                     │  │
│  │             │  │ (摄像头+麦克风)│  │                                 │  │
│  └──────┬──────┘  └──────┬──────┘  └────────────────┬────────────────┘  │
│         │                │                           │                   │
│         └────────────────┴───────────────────────────┘                   │
│                          │                                              │
│                   边缘计算（手机本地）                                     │
│                          │                                              │
│         ┌────────────────┼────────────────┐                            │
│         ▼                ▼                ▼                            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                    │
│  │ 实时场景分析 │  │ 语音转文字   │  │ 情绪检测     │                    │
│  │ (端侧ONNX)  │  │ (端侧模型)  │  │ (端侧模型)  │                    │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘                    │
│         │                │                │                            │
│         └────────────────┴────────────────┘                            │
│                          │                                              │
│         ┌─────────────────────────────────────┐                        │
│         │  自适应帧采样 + 结构化事件生成        │                        │
│         │  {time, scene, activity, emotion,   │                        │
│         │   people, objects, key_frame}       │                        │
│         └──────────────────┬──────────────────┘                        │
│                            │                                           │
│         ┌─────────────────────────────────────┐                        │
│         │  选择性上传：重要事件 → 云端          │                        │
│         │  本地缓存：24小时后自动清理           │                        │
│         └─────────────────────────────────────┘                        │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                     Go 后端（Echo Core）— 会话管理层                      │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  HTTP API 层（Gin）+ WebSocket                                    │   │
│  │  - 用户认证、会话路由                                              │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                              │                                           │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  ChatService（精简版）                                            │   │
│  │                                                                   │   │
│  │  每次对话仅做 4 件事：                                             │   │
│  │  1. 加载历史消息（MySQL）                                         │   │
│  │  2. 调 Python 对话服务（一次 HTTP，流式返回）                       │   │
│  │  3. 透传流式结果给用户                                             │   │
│  │  4. 保存消息 + 异步触发记忆抽取（MySQL）                           │   │
│  │                                                                   │   │
│  │  不再需要：ReActEngine、MultiAgentOrchestrator、Tools             │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                              │                                           │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  Summarizer + PromptCache                                         │   │
│  │  - 会话摘要管理（MySQL）                                          │   │
│  │  - Prompt 前缀缓存（Go 内存 LRU）                                 │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  MySQL: 会话消息、业务数据、摘要                                         │
│  Redis: 活跃会话缓存                                                     │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
                          │
                    一次 HTTP 调用
                          │
┌─────────────────────────────────────────────────────────────────────────┐
│                  Python AI 服务 — 对话大脑 + 记忆系统                      │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                    对话服务（POST /chat/stream）                   │   │
│  │                                                                   │   │
│  │  接收：{user_id, session_id, message, history, attachments}       │   │
│  │                                                                   │   │
│  │  内部流程（进程内，零网络开销）：                                   │   │
│  │                                                                   │   │
│  │  1. 预注入：L0 核心记忆 + 人格设定                                │   │
│  │  2. 轻量 ReAct 循环（max_steps=2）：                              │   │
│  │     ├── 调 LLM（小模型前缀 + 大模型续写）                          │   │
│  │     ├── 有 tool_calls？                                           │   │
│  │     │   ├── understand_image → 视觉模型（进程内）                  │   │
│  │     │   ├── understand_audio → 语音模型（进程内）                  │   │
│  │     │   ├── search_memory   → 向量检索 + 因果链（进程内）          │   │
│  │     │   ├── analyze_emotion → 情感分析（进程内）                   │   │
│  │     │   └── 注入结果，继续循环                                     │   │
│  │     └── 无 tool_calls → 流式返回最终回复                           │   │
│  │  3. 异步：抽取记忆 + 分层归档                                      │   │
│  │                                                                   │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                              │                                           │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                    记忆系统（Memory System）                       │   │
│  │                                                                   │   │
│  │  记忆抽取（extract）:                                              │   │
│  │  ├── 原子事实 + 因果关系抽取                                       │   │
│  │  ├── 向量化（BGE/CLIP/Whisper） → Qdrant                          │   │
│  │  ├── 语义去重 + 关系识别                                           │   │
│  │  ├── 情感标注（emotion_tag + intensity）                           │   │
│  │  └── 分层归档（L0/L1/L2） → MySQL                                  │   │
│  │                                                                   │   │
│  │  记忆检索（retrieve）:                                             │   │
│  │  ├── L0: MySQL 全量加载核心记忆                                    │   │
│  │  ├── L1: Qdrant 向量检索 Top-K                                    │   │
│  │  ├── 因果链: MySQL memory_relations 查询                           │   │
│  │  └── 多模态: Qdrant 跨模态检索                                     │   │
│  │                                                                   │   │
│  │  定时任务（每天凌晨）:                                              │   │
│  │  ├── 记忆压缩: LLM 生成"今日日记" → summary_memory                 │   │
│  │  ├── 归档清理: L1 → L2 降级                                        │   │
│  │  └── 情感衰减: 更新 importance                                     │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         数据存储层                                       │
│                                                                          │
│  ┌───────────────┐  ┌───────────────┐  ┌─────────────────────────────┐  │
│  │     MySQL     │  │    Qdrant     │  │    MinIO / 阿里云OSS        │  │
│  │  (关系型数据)  │  │  (向量索引)   │  │    (多媒体文件)             │  │
│  │               │  │               │  │                             │  │
│  │ • 会话消息    │  │ • 文本嵌入    │  │ • 虚拟人物素材              │  │
│  │ • 记忆5表+1表 │  │ • 图片嵌入    │  │ • 关键帧图片                │  │
│  │ • 关系图谱    │  │ • 音频嵌入    │  │ • 音频/视频片段              │  │
│  │ • 业务数据    │  │ • 视频嵌入    │  │ • 压缩归档                  │  │
│  └───────────────┘  └───────────────┘  └─────────────────────────────┘  │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 3. 记忆分层系统详细设计

### 3.1 三层记忆架构

| 层级 | 名称 | 加载策略 | 内容 | 存储 |
|------|------|---------|------|------|
| **L0** | 核心记忆 | 始终注入 prompt | 身份信息、核心偏好、LLM 认知摘要、人格设定 | MySQL |
| **L1** | 工作记忆 | 按需向量检索 | 近期习惯、情境信息、情感状态 | MySQL + Qdrant |
| **L2** | 归档记忆 | 精确查询时加载 | 完整历史细节、过时偏好 | MySQL + 冷存储 |

### 3.2 记忆表设计：5 张实体表 + 1 张关系表

不是说记忆只有一种类型，而是按**生命周期、读写模式、查询方式**分成 5 张独立的表。

```
┌─────────────────────────────────────────────────────────────────┐
│                    MySQL 记忆表结构（5+1）                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ① identity_memory    身份记忆（永久不变，L0）                    │
│  ② episodic_memory    情景记忆（对话中抽取，L0/L1/L2 分层）      │
│  ③ perceptual_memory  感知记忆（穿戴设备帧采样，L1/L2）           │
│  ④ summary_memory     摘要记忆（LLM 定期压缩生成，L0/L1）        │
│  ⑤ persona_memory     人格记忆（虚拟人物素材/设定，L0）          │
│                                                                  │
│  ⑥ memory_relations   记忆关系（跨表关联，因果/更新/矛盾）        │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

#### ① identity_memory — 身份记忆

```sql
CREATE TABLE identity_memory (
    id           INT AUTO_INCREMENT PRIMARY KEY,
    user_id      VARCHAR(64) NOT NULL,
    
    nickname     VARCHAR(100),
    gender       VARCHAR(20),
    birth_date   DATE,
    occupation   VARCHAR(200),
    location     VARCHAR(200),
    
    core_facts   JSON,            -- {"family": [...], "pets": [...], "health": [...]}
    key_people   JSON,            -- [{"name": "张三", "relation": "男朋友", "since": "2024-03"}]
    
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    UNIQUE INDEX idx_user (user_id)
);
```

#### ② episodic_memory — 情景记忆（核心记忆表）

```sql
CREATE TABLE episodic_memory (
    id              INT AUTO_INCREMENT PRIMARY KEY,
    user_id         VARCHAR(64) NOT NULL,
    
    content         TEXT NOT NULL,
    memory_tier     TINYINT DEFAULT 1,          -- 0=核心 1=工作 2=归档
    category        VARCHAR(30) NOT NULL,       -- event/habit/preference/emotion/relationship
    
    emotion_tag     VARCHAR(30),
    emotion_intensity FLOAT DEFAULT 0.0,
    
    importance      FLOAT DEFAULT 0.5,
    access_count    INT DEFAULT 0,
    last_access_at  DATETIME,
    
    valid_from      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_until     DATETIME,
    
    source_type     VARCHAR(20) DEFAULT 'chat', -- chat / wearable / manual
    source_session_id VARCHAR(64),
    source_message_id INT,
    
    parent_id       INT,
    relation_type   VARCHAR(30),                -- update/contradict/extend
    
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    INDEX idx_user_tier (user_id, memory_tier),
    INDEX idx_user_category (user_id, category),
    INDEX idx_user_emotion (user_id, emotion_tag),
    INDEX idx_user_valid (user_id, valid_until),
    INDEX idx_source_session (source_session_id)
);
```

#### ③ perceptual_memory — 感知记忆

```sql
CREATE TABLE perceptual_memory (
    id              INT AUTO_INCREMENT PRIMARY KEY,
    user_id         VARCHAR(64) NOT NULL,
    
    captured_at     DATETIME NOT NULL,
    memory_tier     TINYINT DEFAULT 1,
    
    scene_desc      VARCHAR(500),
    location        VARCHAR(200),
    detected_objects JSON,
    detected_faces   INT DEFAULT 0,
    emotion_hint     VARCHAR(30),
    trigger_reason  VARCHAR(30),
    
    related_session_id VARCHAR(64),
    frame_url       VARCHAR(500),
    audio_url       VARCHAR(500),
    
    importance      FLOAT DEFAULT 0.3,
    compressed      TINYINT DEFAULT 0,          -- 0=原始 1=已压缩 2=已归档
    
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_user_time (user_id, captured_at),
    INDEX idx_user_compressed (user_id, compressed),
    INDEX idx_related_session (related_session_id)
);
```

#### ④ summary_memory — 摘要记忆

```sql
CREATE TABLE summary_memory (
    id              INT AUTO_INCREMENT PRIMARY KEY,
    user_id         VARCHAR(64) NOT NULL,
    
    summary_type    VARCHAR(30) NOT NULL,       -- daily / weekly / monthly / theme
    title           VARCHAR(200),
    content         TEXT NOT NULL,
    key_insight     TEXT,
    
    period_start    DATETIME NOT NULL,
    period_end      DATETIME NOT NULL,
    
    source_memory_ids JSON,
    source_perceptual_ids JSON,
    
    causal_chains   JSON,
    dominant_emotion VARCHAR(30),
    emotion_trend   VARCHAR(30),
    
    memory_tier     TINYINT DEFAULT 0,
    
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_user_type_period (user_id, summary_type, period_start)
);
```

#### ⑤ persona_memory — 人格记忆

```sql
CREATE TABLE persona_memory (
    id              INT AUTO_INCREMENT PRIMARY KEY,
    user_id         VARCHAR(64) NOT NULL,
    persona_id      VARCHAR(64) NOT NULL,
    
    personality_desc TEXT,
    background_story TEXT,
    speaking_style  TEXT,
    relationship_to_user VARCHAR(100),
    
    avatar_url      VARCHAR(500),
    voice_sample_url VARCHAR(500),
    video_sample_url VARCHAR(500),
    additional_images JSON,
    
    fixed_traits    JSON,
    relationship_level INT DEFAULT 1,
    relationship_history JSON,
    
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    UNIQUE INDEX idx_user_persona (user_id, persona_id)
);
```

#### ⑥ memory_relations — 记忆关系

```sql
CREATE TABLE memory_relations (
    id              INT AUTO_INCREMENT PRIMARY KEY,
    user_id         VARCHAR(64) NOT NULL,
    
    from_table      VARCHAR(30) NOT NULL,       -- episodic / perceptual / summary
    from_id         INT NOT NULL,
    
    to_table        VARCHAR(30) NOT NULL,
    to_id           INT NOT NULL,
    
    relation_type   VARCHAR(30) NOT NULL,       -- causes/leads_to/related_to/contradicts/update
    description     TEXT,
    confidence      FLOAT DEFAULT 0.5,
    
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_user_from (user_id, from_table, from_id),
    INDEX idx_user_to (user_id, to_table, to_id)
);
```

### 3.3 各表对比总结

| 表 | 条数/用户 | 读写模式 | 生命周期 | 向量检索 | 层级 |
|----|----------|---------|---------|---------|------|
| identity_memory | 1 | 极少写 | 永久 | 不需要 | 始终 L0 |
| episodic_memory | 千~万级 | 高频写 | 长期 | Qdrant | L0/L1/L2 |
| perceptual_memory | 万~十万级 | 极高频写 | 30天归档 | Qdrant | L1/L2 |
| summary_memory | 百级 | 低频写 | 长期 | 不需要 | L0/L1 |
| persona_memory | 个位数 | 极少写 | 永久 | Qdrant(素材) | 始终 L0 |
| memory_relations | 千~万级 | 中等 | 长期 | 不需要 | 跨表关联 |

### 3.4 记忆处理流水线

```
对话消息 ──▶ 1.原子事实抽取 ──▶ 2.向量化(BGE) ──▶ 3.语义去重(Qdrant+LLM)
                                    │
                                    ▼
  记忆入库 ◀── 6.分层归档 ◀── 5.情感标注 ◀── 4.因果推理
```

全部在 Python 进程内完成，零网络开销。

### 3.5 记忆检索策略

```python
def get_memory_context(user_id: str, query: str) -> str:
    """构建注入 prompt 的记忆上下文 — 在 Python 对话服务中调用"""
    
    # L0: MySQL 全量加载核心记忆
    l0 = mysql.query("""
        SELECT * FROM identity_memory WHERE user_id = %s
        UNION ALL
        SELECT * FROM summary_memory WHERE user_id = %s AND memory_tier = 0
        ORDER BY created_at DESC LIMIT 5
    """, user_id)
    
    # L1: Qdrant 向量检索 Top-10
    embedding = embed_text(query)
    l1_ids = qdrant.search("episodic_memory", embedding, filter={"user_id": user_id}, limit=10)
    l1 = mysql.query("SELECT * FROM episodic_memory WHERE id IN (%s)", l1_ids)
    
    # 因果链: MySQL 递归查询
    chains = get_causal_chains(user_id, [m.id for m in l1])
    
    return format_context(l0, l1, chains)
```

---

## 4. 情感微模型训练方案

### 4.1 训练路线图

```
Phase 1: SFT 监督微调（1-3 天）
  └── 用情感对话数据教模型"如何陪伴"

Phase 2: DPO 偏好对齐（半天-1 天）
  └── 用"好/差回答对比"让回复更温暖

Phase 3: 情感强化 RLVER（可选，1-2 天）
  └── 用情感奖励信号强化共情能力
```

### 4.2 基座模型选择

| 模型 | 参数 | 显存需求(训练) | 中文能力 | 推荐场景 |
|------|------|---------------|---------|---------|
| **Qwen2.5-7B-Instruct** | 7B | ~24GB | 极强 | 预算有限/初次尝试 |
| **Qwen2.5-14B-Instruct** | 14B | ~48GB | 极强 | 追求陪伴质量 |
| **DeepSeek-R1-Distill-Qwen-7B** | 7B | ~24GB | 强 | 需要推理能力 |

### 4.3 数据集来源

| 数据集 | 语言 | 规模 | 类型 | 获取 |
|--------|------|------|------|------|
| efaqa-corpus-zh | 中文 | 20,000 条 | 心理咨询对话 | gitcode.com |
| ESConv | 英文 | ~1,000 段 | 情感支持对话 | GitHub |
| TIDE | 英文 | 10,000 段 | PTSD 共情对话 | HuggingFace |
| EmpatheticDialogues | 英文 | 25,000 段 | 共情对话 | HuggingFace |
| **自建数据集（最关键）** | 中文 | 500-10,000 条 | 定制化人格对话 | GPT-4o 辅助 + 人工审核 |

### 4.4 与记忆系统的集成

```
Go 端（一次 HTTP 调用）:
    POST /chat/stream {user_id, session_id, message, history, attachments}

Python 端（进程内完成）:
    1. 检索记忆: get_memory_context(user_id, message)
    2. 拼接 prompt: [人格设定 + 记忆上下文 + 历史消息 + 用户消息]
    3. 流式生成: 小模型前缀(首字<200ms) + 大模型续写
    4. 流式返回给 Go
```

---

## 5. 对话架构设计

### 5.1 核心决策：对话逻辑在 Python，不在 Go

**为什么？**

情感陪伴场景的对话模型是"理解 → 检索 → 回应"的轻量 ReAct（最多 2 步），不是"推理 → 执行 → 验证 → 再推理"的深度 Agent 循环。陪伴场景不需要查天气、算数学、搜知识库，工具调用全部围绕记忆和多模态理解。

**ReAct 在 Go → 多跳往返：**
```
Go ──HTTP──► Python 记忆服务（取记忆）
Go ──HTTP──► Python LLM（Step 1）
Go ──HTTP──► Python 记忆服务（工具调用）
Go ──HTTP──► Python LLM（Step 2）
4 次 HTTP 往返，延迟累加
```

**ReAct 在 Python → 一次往返：**
```
Go ──HTTP──► Python 对话服务
                ├── 记忆检索（进程内）
                ├── LLM 推理（进程内）
                ├── 工具调用（进程内）
                └── 流式返回
1 次 HTTP 往返，进程内零开销
```

### 5.2 陪伴场景的轻量 ReAct

陪伴需要 ReAct，但是**轻量的**——模型按需调用工具理解多模态数据、搜索记忆：

```
用户: [发了一张夕阳照片] "今天加班到很晚，出来看到这个..."

Python 轻量 ReAct（max_steps=2）:

Step 1: LLM 决策 → 需要理解图片 + 搜索记忆
  ├── tool_call: understand_image(image)
  │   └── 返回: "海边日落，一个人，画面偏冷色调，氛围孤独"
  ├── tool_call: search_memory("日落 海边")
  │   └── 返回: "去年和前任来海边看过日落" "用户喜欢拍日落"
  └── tool_call: analyze_emotion("加班到很晚...")
      └── 返回: "疲惫、孤独、略带感伤"

Step 2: LLM 综合 → 生成回复
  "加班到现在辛苦了吧。这张照片拍得真好，我记得你一直喜欢拍日落。
   去年那会儿你也拍过海边的日落...今天一个人看到的景色，和那会儿比，
   有什么不一样的感觉吗？"
```

### 5.3 陪伴场景的工具集

| 工具 | 触发场景 | 实现 |
|------|---------|------|
| `understand_image` | 用户发了图片 | 视觉模型（进程内） |
| `understand_audio` | 用户发了语音/穿戴设备音频 | 语音模型（进程内） |
| `search_memory` | 用户提到过去的事 | Qdrant 向量检索 + MySQL 因果链（进程内） |
| `analyze_emotion` | 需要精确判断用户情绪 | 情感分析（进程内） |

**全部在 Python 进程内执行，零网络开销。**

### 5.4 Go 侧 ChatService 精简后的样子

```go
// ChatService 精简版 —— 只做会话管理 + 调用 Python

func (s *ChatService) ChatStream(req ChatRequest, onChunk func(StreamChunk)) {
    // 1. 加载历史消息（MySQL）
    history := s.loadHistory(req.SessionID, req.UserID)
    
    // 2. 保存用户消息（MySQL）
    s.saveUserMessage(req)
    
    // 3. 调 Python 对话服务（一次 HTTP，流式返回）
    resp := s.pythonClient.ChatStream(PythonChatRequest{
        UserID:       req.UserID,
        SessionID:    req.SessionID,
        Message:      req.Message,
        History:      history,
        Attachments:  req.Attachments,
        SystemPrompt: systemPrompt,
    })
    
    // 4. 收集完整回复 + 流式透传给前端
    var fullReply strings.Builder
    for chunk := range resp {
        fullReply.WriteString(chunk.Delta)
        onChunk(StreamChunk{Delta: chunk.Delta, Reply: fullReply.String()})
    }
    
    // 5. 保存助手回复（MySQL）
    s.saveAssistantMessage(req, fullReply.String())
    
    // 6. 异步触发记忆抽取
    go s.pythonClient.ExtractMemory(req.UserID, req.Message, fullReply.String())
    
    onChunk(StreamChunk{Done: true, Reply: fullReply.String()})
}
```

**Go 侧删除的代码：**
- `agent/react_engine.go`（462 行 ReAct 循环）
- `agent/tools.go` 中的工具定义和 Handler（380 行）
- `service/chat_service.go` 中的 MultiAgentOrchestrator 构造逻辑

**Go 侧保留的代码：**
- 会话消息读写（MySQL）
- Prompt 缓存
- 摘要触发
- HTTP/WebSocket 处理

---

## 6. 多模态数据与穿戴设备集成

### 6.1 自适应帧采样策略

**核心思路**：不是存储连续视频，而是智能采样关键帧

```
基础采样（始终运行）
├── 每 30 秒 1 帧（默认）
│   └── 目的：构建用户日常轨迹的时间线

事件触发采样（动态增强）
├── 场景变化时 +3 帧（变化前/中/后）
├── 检测到人脸时 +5 帧
├── 检测到特定物体时 +2 帧
├── 用户说话时每 2 秒 1 帧
└── 用户情绪变化时 +3 帧

压缩策略
├── 24小时内：原始分辨率存储
├── 7天后：降采样到 480p
├── 30天后：只保留关键帧 + LLM 生成的文本描述
└── 90天后：删除图片，只保留文本摘要
```

### 6.2 成本对比

| 方案 | 月数据量/用户 | 月成本/万用户 | 可回溯精度 |
|------|-------------|-------------|-----------|
| 连续 1080p 视频 | 2.4TB | 240万元 | 100% |
| 每5秒1帧 | 120GB | 12万元 | 80% |
| **自适应采样（推荐）** | **2GB** | **2000元** | **90%** |

### 6.3 虚拟人物素材存储

| 类型 | 存储位置 | 处理方式 |
|------|---------|---------|
| 文本介绍 | MySQL (persona_memory) | 直接存储，用于构建 System Prompt |
| 图片 | MinIO/OSS | CLIP 嵌入 → Qdrant，原图 CDN 分发 |
| 音频 | MinIO/OSS | Whisper 转录 + 声纹嵌入 → Qdrant |
| 视频 | MinIO/OSS | 关键帧提取 + VideoMAE 嵌入 → Qdrant |

---

## 7. 存储架构与数据库选型

### 7.1 最终选型

| 组件 | 选型 | 用途 | 理由 |
|------|------|------|------|
| **关系型数据库** | **MySQL** | 会话消息、记忆 5+1 表、业务数据 | 现有技术栈，零迁移成本 |
| **向量数据库** | **Qdrant** | 全模态向量索引（文本/图片/音频/视频） | 开源、多向量集合、支持亿级 |
| **对象存储** | **MinIO** (自建) 或 **阿里云OSS** | 多媒体文件（图片/音频/视频/帧） | 大文件存储、CDN 分发 |
| **缓存** | **Redis** | 活跃会话、热点记忆 | 高频读写、低延迟 |
| **消息队列** | **Kafka** | 穿戴设备事件流、异步处理 | 高吞吐、可回溯 |

### 7.2 为什么用 MySQL 而不是 PostgreSQL

| 考量 | 结论 |
|------|------|
| 现有技术栈 | 已有 MySQL，零迁移成本 |
| 向量检索 | 全部交给 Qdrant，MySQL 不需要向量能力 |
| 递归 CTE | MySQL 8.0+ 支持，满足 1-3 跳因果链查询 |
| 全文搜索 | MySQL FULLTEXT 索引替代 GIN |
| GORM 兼容 | 不需要换 driver |

### 7.3 为什么不用图数据库（Neo4j）

| 考量 | 结论 |
|------|------|
| 关系复杂度 | 陪伴场景关系简单（1-3 跳），MySQL 递归 CTE 够用 |
| 规模 | 单用户活跃记忆 < 1000 条，MySQL 毫秒级响应 |
| 运维成本 | 引入 Neo4j = 新增一套系统，运维翻倍 |
| 未来扩展 | 如真需要复杂图遍历，可加 Neo4j 作为只读副本 |

### 7.4 数据流架构

```
穿戴设备帧事件
    │
    ▼
Kafka 消息队列
    │
    ├──► 实时处理消费者（Python）
    │     ├── 多模态嵌入生成（CLIP/BGE/Whisper）
    │     ├── 存入 Qdrant（向量索引）
    │     └── 元数据存入 MySQL（perceptual_memory）
    │
    └──► 定时压缩任务（每天凌晨，Python）
          ├── 读取当天事件
          ├── LLM 生成"今日日记"
          ├── 选择代表性关键帧
          ├── 生成多模态嵌入
          ├── 存入 summary_memory → Qdrant
          └── 原始帧标记为"可归档"
```

---

## 8. 技术选型决策记录

### 8.1 ReAct 位置：Python vs Go

| 维度 | ReAct 在 Go | ReAct 在 Python（选择） |
|------|------------|------------------------|
| HTTP 往返 | 每 step 一次（3-4 次/对话） | 1 次/对话 |
| 工具调用延迟 | ~10ms 网络 × N 次 | 进程内调用，~0ms |
| 首字时延 | 多跳延迟累积 | 一次请求，Python 内部优化 |
| 代码维护 | LLM prompt 编排散落 Go/Python | 全部在 Python，统一管理 |
| 记忆检索 | Go→Python 额外调用 | 进程内直接调 MemoryService |

**决策：ReAct 在 Python。** 陪伴场景的轻量 ReAct（max_steps=2）在 Python 进程内完成，工具调用零网络开销。

### 8.2 数据库：MySQL vs PostgreSQL

| 维度 | PostgreSQL + pgvector | MySQL + Qdrant（选择） |
|------|----------------------|------------------------|
| 迁移成本 | 需要从 MySQL 迁移 | 零迁移 |
| 向量检索 | pgvector 内嵌 | Qdrant 独立服务 |
| 查询方式 | 一条 SQL 同时做关系+向量 | 两次查询（MySQL + Qdrant），~1ms |
| 运维 | 1 个数据库 | 1 个数据库 + 1 个向量库（Qdrant 本来就要） |

**决策：MySQL + Qdrant。** 现有 MySQL 不动，向量检索全部交给 Qdrant。

### 8.3 记忆系统：自研 vs 开源

| 维度 | 开源（Mem0/Zep） | 自研（选择） |
|------|-----------------|-------------|
| 语言栈 | Python，无 Go SDK | Python，原生集成 |
| 领域适配 | 通用 Agent，不贴合陪伴场景 | 专为虚拟陪伴设计 |
| 成本 | 云服务按月付费 | 可控，一次性开发 |

**决策：自研**，借鉴开源最佳实践。

### 8.4 模型训练：SFT + DPO 路线

| 阶段 | 方法 | 目的 |
|------|------|------|
| SFT | LoRA 微调 | 教模型"如何陪伴" |
| DPO | 偏好对齐 | 让回复更温暖、更共情 |
| RLVER（可选） | 情感强化 | 用情感奖励信号优化 |

**决策：SFT + DPO 为核心，RLVER 作为进阶选项。**

---

## 9. 实施路线图

### Phase 1：Python 记忆系统（第 1-4 周）

| 周 | 任务 | 产出 |
|----|------|------|
| W1 | MySQL 5+1 表结构创建 | 数据库就绪 |
| W1 | Qdrant 部署 + 向量集合创建 | 向量库就绪 |
| W2 | 原子事实抽取 + 向量化 | 记忆抽取流水线 |
| W2 | 语义去重（Qdrant + LLM）+ 关系识别 | 去重/关系模块 |
| W3 | 分层归档逻辑（L0/L1/L2） | 分层系统 |
| W3 | 记忆检索 API（L0 全量 + L1 向量 Top-K + 因果链） | 检索服务 |
| W4 | 记忆压缩（LLM 每日摘要） | 压缩系统 |
| W4 | 集成测试 + 基准测试 | 可运行的记忆系统 |

### Phase 2：情感微模型（第 5-8 周）

| 周 | 任务 | 产出 |
|----|------|------|
| W5 | 基座模型下载 + 环境搭建 | 训练环境就绪 |
| W5 | 数据集准备（efaqa + 自建） | 训练数据 |
| W6 | SFT 训练（LoRA） | 初版情感模型 |
| W7 | DPO 训练 | 优化版情感模型 |
| W7 | 模型评估（自动 + 人工） | 评估报告 |
| W8 | 模型部署（vLLM 量化） | 生产就绪模型 |

### Phase 3：对话架构整合（第 9-11 周）

| 周 | 任务 | 产出 |
|----|------|------|
| W9 | Python 对话服务（POST /chat/stream） | 对话 API |
| W9 | 轻量 ReAct 循环（max_steps=2） | 工具调用框架 |
| W10 | 工具实现：understand_image / search_memory / analyze_emotion | 工具集 |
| W10 | 流式级联生成：小模型前缀 + 大模型续写 | 流式输出 |
| W11 | Go 侧 ChatService 精简 | 删除 ReAct/Agent 代码 |
| W11 | 端到端集成测试 | 完整对话链路 |

### Phase 4：多模态与穿戴设备（第 12-17 周）

| 周 | 任务 | 产出 |
|----|------|------|
| W12 | Qdrant 多模态集合 + 嵌入模型 | 多模态向量库 |
| W12 | MinIO/OSS 对象存储搭建 | 文件存储就绪 |
| W13 | 帧采样引擎（端侧 ONNX 模型） | 边缘处理模块 |
| W14 | Kafka 消息队列 + 云端消费者 | 事件流管道 |
| W15 | 多模态记忆存储 + 检索 | 图片/音频/视频记忆 |
| W16 | 每日 LLM 压缩摘要 | 记忆压缩系统 |
| W17 | 穿戴设备联调 + 端到端测试 | 完整多模态体验 |

### Phase 5：优化与扩展（第 18-22 周）

| 周 | 任务 | 产出 |
|----|------|------|
| W18 | 记忆后台整理（Consolidator） | 自动归档/合并 |
| W19 | 遗忘曲线 + 情感衰减 | 智能记忆管理 |
| W20 | 因果推理增强 | 深度关系推导 |
| W21 | A/B 测试 + 用户反馈收集 | 数据驱动优化 |
| W22 | 性能优化 + 监控告警 | 可运维系统 |

---

## 附录

### A. 相关文档

| 文档 | 内容 | 文件 |
|------|------|------|
| 分层记忆系统评估报告 | 开源调研 + 自研方案 + 实现步骤 | `MEMORY_PLAN_REPORT.md` |
| 情感微模型训练指南 | 训练路线 + 数据集 + 代码示例 | `EMOTION_MODEL_TRAINING_GUIDE.md` |
| 综合设计文档 | 本文档，整合以上所有内容 | `COMPREHENSIVE_DESIGN_DOCUMENT.md` |

### B. 关键术语

| 术语 | 说明 |
|------|------|
| L0/L1/L2 | 核心记忆/工作记忆/归档记忆三层架构 |
| ReAct | Reasoning + Acting，LLM 推理+工具调用的循环模式 |
| RLVER | 可验证情感奖励的强化学习（腾讯混元） |
| LoRA | 低秩适配，轻量级微调技术 |
| DPO | 直接偏好优化，简化版 RLHF |
| HNSW | 分层导航小世界图，向量索引算法 |

### C. 参考项目

| 项目 | 链接 | 借鉴点 |
|------|------|--------|
| Mem0 | github.com/mem0ai/mem0 | 向量+图双检索 |
| SuperMemory | supermemory.ai | 原子事实+关系版本化 |
| Letta (MemGPT) | docs.letta.com | 内存层级设计 |
| Zep | getzep.com | 时序知识图谱 |
| M3-Agent | github.com/ByteDance-Seed/M3-Agent-Memorization | 双线程架构 |
| OpenViking | github.com/volcengine/OpenViking | L0/L1/L2 分层 |