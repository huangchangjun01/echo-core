# Echo Core · 虚拟陪伴平台综合设计文档

> **版本**: v2.0  
> **日期**: 2026-06-22  
> **目标**: 整合记忆系统与情感微模型的完整技术方案  
> **分支**: `memory-plan`

---

## 目录

1. [项目概述与核心定位](#1-项目概述与核心定位)
2. [系统总体架构](#2-系统总体架构)
3. [记忆分层系统详细设计](#3-记忆分层系统详细设计)
4. [情感微模型训练方案](#4-情感微模型训练方案)
5. [多模态数据与穿戴设备集成](#5-多模态数据与穿戴设备集成)
6. [存储架构与数据库选型](#6-存储架构与数据库选型)
7. [技术选型决策记录](#7-技术选型决策记录)
8. [实施路线图](#8-实施路线图)

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
| 后端服务 | Go (Echo Core) |
| AI 服务 | Python (记忆系统 + 情感模型训练/推理) |
| 数据库 | PostgreSQL + pgvector + Qdrant + MinIO |
| 缓存 | Redis |
| 消息队列 | Kafka |
| 基座模型 | Qwen2.5-7B/14B-Instruct |
| 向量模型 | BGE / CLIP |

---

## 2. 系统总体架构

### 2.1 服务分层架构

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
│                         Go 后端服务层（Echo Core）                       │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  HTTP API 层（Gin）                                              │    │
│  │  - 用户认证、会话管理、消息路由                                    │    │
│  │  - WebSocket 实时通信                                            │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              │                                           │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  业务编排层                                                      │    │
│  │                                                                  │    │
│  │  MemoryService（精简版）                                         │    │
│  │  ├── 调用 Python 记忆服务 API（主路径）                          │    │
│  │  └── 降级：本地 MySQL 缓存（Python 不可用时）                     │    │
│  │                                                                  │    │
│  │  ChatService                                                     │    │
│  │  ├── 流式级联生成：小模型前缀 + 大模型续写                        │    │
│  │  ├── 记忆上下文注入                                              │    │
│  │  └── 情感风格控制                                                │    │
│  │                                                                  │    │
│  │  SessionService                                                  │    │
│  │  └── 会话消息管理（短期记忆）                                     │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              │                                           │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  数据访问层                                                      │    │
│  │  - MySQL（会话消息、业务数据、降级缓存）                          │    │
│  │  - Redis（活跃会话缓存）                                         │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼ (HTTP/gRPC)
┌─────────────────────────────────────────────────────────────────────────┐
│                      Python AI 服务层                                    │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  记忆系统服务（Memory System）                                    │    │
│  │                                                                  │    │
│  │  记忆检索 API                                                    │    │
│  │  ├── get_memory_context(user_id, query) → 分层记忆上下文         │    │
│  │  ├── search_memories(user_id, query, filters) → 语义检索结果     │    │
│  │  └── get_memory_chain(memory_id) → 因果链                       │    │
│  │                                                                  │    │
│  │  记忆抽取 API                                                    │    │
│  │  ├── extract_and_store(user_id, user_msg, asst_reply)            │    │
│  │  │   ├── 原子事实抽取                                            │    │
│  │  │   ├── 向量化（文本/图片/音频/视频）                            │    │
│  │  │   ├── 语义去重 + 关系识别                                     │    │
│  │  │   ├── 情感标注                                                │    │
│  │  │   └── 分层归档（L0/L1/L2）                                    │    │
│  │  └── consolidate_memories(user_id) → 后台整理                    │    │
│  │                                                                  │    │
│  │  多模态处理管道                                                  │    │
│  │  ├── 帧事件接收（来自 Kafka）                                    │    │
│  │  ├── 多模态嵌入生成（CLIP/BGE/Whisper）                          │    │
│  │  ├── 事件关联分析（地点/人物/情感聚类）                           │    │
│  │  ├── 每日 LLM 压缩摘要                                          │    │
│  │  └── 定期归档清理                                               │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              │                                           │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  情感微模型服务（Emotion Model）                                  │    │
│  │                                                                  │    │
│  │  模型推理                                                        │    │
│  │  ├── 小模型快速前缀生成（首字 < 200ms）                          │    │
│  │  └── 大模型深度续写（记忆整合 + 因果推理）                        │    │
│  │                                                                  │    │
│  │  模型训练（离线）                                                │    │
│  │  ├── SFT 监督微调                                                │    │
│  │  ├── DPO 偏好对齐                                                │    │
│  │  └── RLVER 情感强化（可选进阶）                                   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         数据存储层                                       │
│                                                                          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────────────┐  │
│  │ PostgreSQL  │  │  Qdrant     │  │  MinIO / 阿里云OSS              │  │
│  │ (元数据+关系)│  │ (向量索引)  │  │  (多媒体文件)                   │  │
│  │             │  │             │  │                                 │  │
│  │ • 记忆本体  │  │ • 文本嵌入  │  │ • 虚拟人物素材                  │  │
│  │ • 关系图谱  │  │ • 图片嵌入  │  │ • 关键帧图片                    │  │
│  │ • 用户信息  │  │ • 音频嵌入  │  │ • 重要视频片段                  │  │
│  │ • 时序索引  │  │ • 视频嵌入  │  │ • 压缩归档                      │  │
│  └─────────────┘  └─────────────┘  └─────────────────────────────────┘  │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 3. 记忆分层系统详细设计

### 3.1 三层记忆架构

| 层级 | 名称 | 加载策略 | 内容 | 存储 |
|------|------|---------|------|------|
| **L0** | 核心记忆 | 始终注入 prompt | 用户身份、核心偏好、当前情感状态、最近关键事件 | PostgreSQL |
| **L1** | 工作记忆 | 按需向量检索 | 用户习惯、近期兴趣、关系状态、情境信息 | PostgreSQL + Qdrant |
| **L2** | 归档记忆 | 精确查询时加载 | 完整对话历史、历史事件、已过时偏好 | PostgreSQL + 冷存储 |

### 3.2 记忆数据模型

```sql
-- 记忆本体表
CREATE TABLE user_memories (
    id              SERIAL PRIMARY KEY,
    user_id         VARCHAR(64) NOT NULL,
    memory_type     VARCHAR(50) NOT NULL,  -- preference/info/knowledge/event/emotion
    content         TEXT NOT NULL,
    
    memory_tier     INT DEFAULT 1,         -- 0=核心 1=工作 2=归档
    importance      FLOAT DEFAULT 0.5,
    
    emotion_tag     VARCHAR(50),
    emotion_intensity FLOAT DEFAULT 0.0,
    
    access_count    INT DEFAULT 0,
    last_access_at  TIMESTAMP,
    
    valid_from      TIMESTAMP NOT NULL DEFAULT NOW(),
    valid_until     TIMESTAMP,
    
    embedding       vector(768),           -- pgvector 向量类型
    
    parent_id       INT REFERENCES user_memories(id),
    relation_type   VARCHAR(50),           -- update/contradict/extend/derive
    
    source_session_id VARCHAR(64),
    source_message_id INT,
    
    created_at      TIMESTAMP DEFAULT NOW(),
    updated_at      TIMESTAMP DEFAULT NOW()
);

-- 记忆关系表
CREATE TABLE memory_relations (
    id              SERIAL PRIMARY KEY,
    user_id         VARCHAR(64) NOT NULL,
    from_memory_id  INT NOT NULL REFERENCES user_memories(id),
    to_memory_id    INT NOT NULL REFERENCES user_memories(id),
    relation_type   VARCHAR(50) NOT NULL,  -- causes/leads_to/related_to/contradicts
    description     TEXT,
    confidence      FLOAT DEFAULT 0.5,
    created_at      TIMESTAMP DEFAULT NOW()
);

-- 多模态记忆扩展表
CREATE TABLE multimodal_memories (
    id              SERIAL PRIMARY KEY,
    user_id         VARCHAR(64) NOT NULL,
    memory_id       INT REFERENCES user_memories(id),
    
    modality_type   VARCHAR(20) NOT NULL,  -- image/audio/video
    media_url       TEXT NOT NULL,         -- MinIO/OSS 地址
    
    -- 多模态嵌入（分别存储，Qdrant 中聚合）
    text_embedding  vector(768),
    image_embedding vector(512),
    audio_embedding vector(256),
    video_embedding vector(512),
    
    scene_description TEXT,                -- 端侧/云端生成的场景描述
    detected_objects  JSONB,               -- 检测到的物体列表
    detected_faces    INT DEFAULT 0,
    emotion_hint      VARCHAR(50),
    
    timestamp       TIMESTAMP NOT NULL,
    location        VARCHAR(200),
    
    created_at      TIMESTAMP DEFAULT NOW()
);

-- 索引
CREATE INDEX idx_memories_user_tier ON user_memories(user_id, memory_tier);
CREATE INDEX idx_memories_user_type ON user_memories(user_id, memory_type);
CREATE INDEX idx_memories_emotion ON user_memories(user_id, emotion_tag);
CREATE INDEX idx_memories_valid ON user_memories(user_id, valid_until);
CREATE INDEX idx_relations_from ON memory_relations(from_memory_id);
CREATE INDEX idx_relations_to ON memory_relations(to_memory_id);
CREATE INDEX idx_multimodal_user_time ON multimodal_memories(user_id, timestamp);

-- 向量索引
CREATE INDEX idx_memories_embedding ON user_memories 
    USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 200);
```

### 3.3 记忆处理流水线

```
对话消息 ──▶ 1.原子事实抽取 ──▶ 2.多模态向量化 ──▶ 3.语义去重/关系识别
                                    │
                                    ▼
  记忆入库 ◀── 6.分层归档 ◀── 5.情感标注 ◀── 4.因果推理
```

#### 阶段 1：原子事实 + 因果关系抽取

```python
EXTRACT_PROMPT = """
从以下对话中抽取用户的原子事实和事实间的因果关系。

【抽取规则】
1. 抽取关于用户的原子事实（每条一句话）
2. 如果两个事实之间存在因果关系，标注出来
3. 因果关系类型：causes（导致）、leads_to（引发）、related_to（相关）、contradicts（矛盾）

【输出格式】
{
  "facts": [
    {"id": "f1", "content": "用户3月失恋了", "type": "event", "emotion": "sad", "intensity": 0.9},
    {"id": "f2", "content": "用户从3月开始失眠", "type": "habit", "emotion": "anxious", "intensity": 0.7}
  ],
  "relations": [
    {"from": "f1", "to": "f2", "type": "causes", "confidence": 0.8, "description": "失恋导致情绪波动，进而引发失眠"}
  ]
}
"""
```

#### 阶段 2：多模态向量化

| 模态 | 模型 | 维度 | 用途 |
|------|------|------|------|
| 文本 | BGE-M3 / text-embedding-3 | 768 | 语义检索 |
| 图片 | CLIP | 512 | 视觉场景检索 |
| 音频 | Whisper + 音频编码器 | 256 | 声纹/语音内容检索 |
| 视频 | VideoMAE / 关键帧 CLIP | 512 | 视频片段检索 |

#### 阶段 3-6：去重、情感标注、因果推理、分层归档

详见第 2 份报告 `MEMORY_PLAN_REPORT.md` 中的详细设计。

### 3.4 记忆检索策略

```python
def get_memory_context(user_id: str, query: str) -> str:
    """构建注入 prompt 的记忆上下文"""
    
    # L0: 核心记忆（全量加载）
    l0_memories = db.query(UserMemory).filter(
        user_id == user_id,
        memory_tier == 0,
        or_(valid_until == None, valid_until > now())
    ).all()
    
    # L1: 工作记忆（向量检索 Top-10）
    query_embedding = embed_text(query)
    l1_memories = db.query(UserMemory).filter(
        user_id == user_id,
        memory_tier == 1,
        or_(valid_until == None, valid_until > now())
    ).order_by(
        UserMemory.embedding.cosine_distance(query_embedding)
    ).limit(10).all()
    
    # 多模态检索（如果查询涉及视觉/听觉）
    multimodal_results = qdrant.search(
        collection_name="multimodal_memories",
        query_vector={"text": query_embedding},
        filter={"user_id": user_id},
        limit=5
    )
    
    # 因果链检索（如果相关记忆有因果关系）
    causal_chains = []
    for m in l1_memories:
        chain = get_causal_chain(m.id, max_depth=3)
        if len(chain) > 1:
            causal_chains.append(chain)
    
    # 格式化为 prompt
    return format_memory_context(l0_memories, l1_memories, multimodal_results, causal_chains)
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
用户请求
    │
    ▼
Go 后端
    │
    ├──► Python 记忆服务：get_memory_context(user_id, query)
    │         │
    │         └──► 返回：分层记忆上下文（L0 + L1 + 因果链）
    │
    └──► Python 情感模型服务：generate_stream()
              │
              ├──► System Prompt：人格定义 + 记忆上下文
              ├──► 小模型：快速生成前缀（首字 < 200ms）
              └──► 大模型：续写深度回复（记忆整合 + 因果推理）
                    │
                    └──► 流式返回用户
```

---

## 5. 多模态数据与穿戴设备集成

### 5.1 自适应帧采样策略

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

### 5.2 边缘端处理流程

```python
class AdaptiveFrameSampler:
    def __init__(self):
        self.base_interval = 30          # 基础采样：30秒
        self.scene_change_threshold = 0.3  # 场景变化阈值
        
        # 端侧轻量 ONNX 模型
        self.scene_classifier = load_onnx("scene_classifier.onnx")
        self.object_detector = load_onnx("yolov8n.onnx")
        self.face_detector = load_onnx("retinaface.onnx")
        self.emotion_classifier = load_onnx("emotion_mobilenet.onnx")
    
    def process_frame(self, frame, audio_buffer):
        should_sample = False
        trigger_reason = "scheduled"
        
        # 1. 基础定时采样
        if time_since_last_sample >= 30:
            should_sample = True
        
        # 2. 场景变化检测
        if scene_similarity < 0.3:
            should_sample = True
            trigger_reason = "scene_change"
        
        # 3. 人脸检测
        faces = self.face_detector.detect(frame)
        if len(faces) > 0:
            should_sample = True
            trigger_reason = "face_detected"
        
        # 4. 语音活动检测
        if vad.detect(audio_buffer):
            if time_since_last_sample >= 2:
                should_sample = True
                trigger_reason = "speech"
        
        # 5. 情绪变化
        emotion = self.emotion_classifier.classify(faces[0]) if faces else None
        if emotion != last_emotion:
            should_sample = True
            trigger_reason = "emotion_change"
        
        if should_sample:
            return FrameEvent(
                frame=compress_frame(frame, 640x480, quality=80),
                scene_description=self.scene_classifier.describe(frame),
                detected_objects=self.object_detector.detect(frame),
                detected_faces=len(faces),
                emotion_hint=emotion,
                trigger_reason=trigger_reason
            )
        return None
```

### 5.3 成本对比

| 方案 | 月数据量/用户 | 月成本/万用户 | 可回溯精度 |
|------|-------------|-------------|-----------|
| 连续 1080p 视频 | 2.4TB | 240万元 | 100% |
| 每5秒1帧 | 120GB | 12万元 | 80% |
| **自适应采样（推荐）** | **2GB** | **2000元** | **90%** |

### 5.4 虚拟人物素材存储

用户导入的虚拟人物素材（文本/图片/音频/视频）：

| 类型 | 存储位置 | 处理方式 |
|------|---------|---------|
| 文本介绍 | PostgreSQL | 直接存储，用于构建 System Prompt |
| 图片 | MinIO/OSS | CLIP 嵌入 → Qdrant，原图 CDN 分发 |
| 音频 | MinIO/OSS | Whisper 转录 + 声纹嵌入 → Qdrant |
| 视频 | MinIO/OSS | 关键帧提取 + VideoMAE 嵌入 → Qdrant |

---

## 6. 存储架构与数据库选型

### 6.1 最终选型

| 组件 | 选型 | 用途 | 理由 |
|------|------|------|------|
| **关系型数据库** | **PostgreSQL + pgvector** | 记忆本体、关系图谱、元数据、时序 | 支持向量检索 + 关系查询 + 全文搜索，一个库解决 |
| **向量数据库** | **Qdrant** | 多模态向量索引（文本/图片/音频/视频） | 开源、多向量集合、REST API 友好 |
| **对象存储** | **MinIO** (自建) 或 **阿里云OSS** | 多媒体文件（图片/音频/视频/帧） | 大文件存储、CDN 分发、成本低 |
| **缓存** | **Redis** | 活跃会话、实时感知摘要、热点记忆 | 高频读写、低延迟 |
| **消息队列** | **Kafka** | 穿戴设备事件流、异步处理 | 高吞吐、可回溯 |

### 6.2 为什么不用图数据库（Neo4j）

| 考量 | 结论 |
|------|------|
| 关系复杂度 | 陪伴场景关系简单（1-3 跳），PostgreSQL 递归 CTE 够用 |
| 规模 | 单用户活跃记忆 < 1000 条，PostgreSQL 毫秒级响应 |
| 运维成本 | 引入 Neo4j = 新增一套系统，运维翻倍 |
| 未来扩展 | 如真需要复杂图遍历，可加 Neo4j 作为只读副本，不改主架构 |

### 6.3 数据流架构

```
穿戴设备帧事件
    │
    ▼
Kafka 消息队列
    │
    ├──► 实时处理消费者
    │     ├── 多模态嵌入生成（CLIP/BGE/Whisper）
    │     ├── 存入 Qdrant（向量索引）
    │     └── 元数据存入 PostgreSQL
    │
    └──► 定时压缩任务（每天凌晨）
          ├── 读取当天事件
          ├── LLM 生成"今日日记"
          ├── 选择代表性关键帧
          ├── 生成多模态嵌入
          ├── 存入"压缩记忆"集合
          └── 原始帧标记为"可归档"
```

---

## 7. 技术选型决策记录

### 7.1 记忆系统：自研 vs 开源

| 维度 | 开源（Mem0/Zep） | 自研（推荐） |
|------|-----------------|-------------|
| 语言栈 | Python，无 Go SDK | Python + Go，原生集成 |
| 领域适配 | 通用 Agent，不贴合陪伴场景 | 专为虚拟陪伴设计 |
| 延迟 | 需 HTTP 桥接，+50ms | Go 直接调用，零额外延迟 |
| 运维 | 两套系统 | 统一架构 |
| 成本 | 云服务按月付费 | 可控，一次性开发 |

**决策：自研**，借鉴开源最佳实践（Mem0 的向量+图双检索、SuperMemory 的原子事实+关系版本化、Letta 的内存层级、Zep 的双时序、M3-Agent 的双线程、OpenViking 的 L0/L1/L2）。

### 7.2 数据库：PostgreSQL vs 图数据库

| 维度 | PostgreSQL + pgvector | Neo4j |
|------|----------------------|-------|
| 记忆 CRUD | 天然擅长 | 可以但啰嗦 |
| 向量检索 | pgvector 支持 | 需额外插件 |
| 关系查询 | 递归 CTE 支持 3 跳 | 原生支持多跳 |
| 运维复杂度 | 低 | 高 |
| Go 生态 | GORM 成熟 | 驱动较弱 |

**决策：PostgreSQL + pgvector**，关系表模拟图谱，未来如需复杂图遍历再加 Neo4j 只读副本。

### 7.3 模型训练：SFT + DPO 路线

| 阶段 | 方法 | 目的 |
|------|------|------|
| SFT | LoRA 微调 | 教模型"如何陪伴" |
| DPO | 偏好对齐 | 让回复更温暖、更共情 |
| RLVER（可选） | 情感强化 | 用情感奖励信号优化 |

**决策：SFT + DPO 为核心，RLVER 作为进阶选项。**

---

## 8. 实施路线图

### Phase 1：基础记忆系统（第 1-4 周）

| 周 | 任务 | 产出 |
|----|------|------|
| W1 | PostgreSQL + pgvector 环境搭建 | 数据库就绪 |
| W1 | 数据模型升级（UserMemory + MemoryRelation） | 表结构就绪 |
| W2 | 原子事实抽取 + 向量化 | 记忆抽取流水线 |
| W2 | 语义去重 + 关系识别 | 去重/关系模块 |
| W3 | 分层归档逻辑（L0/L1/L2） | 分层系统 |
| W3 | 分层注入引擎 | BuildMemoryContext 升级 |
| W4 | 向量检索器（内存索引/pgvector） | 语义检索 |
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
| W8 | Go 后端集成 | 端到端可运行 |

### Phase 3：多模态与穿戴设备（第 9-14 周）

| 周 | 任务 | 产出 |
|----|------|------|
| W9 | Qdrant 部署 + 多模态嵌入模型 | 向量数据库就绪 |
| W9 | MinIO/OSS 对象存储搭建 | 文件存储就绪 |
| W10 | 帧采样引擎（端侧 ONNX 模型） | 边缘处理模块 |
| W11 | Kafka 消息队列 + 云端消费者 | 事件流管道 |
| W12 | 多模态记忆存储 + 检索 | 图片/音频/视频记忆 |
| W13 | 每日 LLM 压缩摘要 | 记忆压缩系统 |
| W14 | 穿戴设备联调 + 端到端测试 | 完整多模态体验 |

### Phase 4：优化与扩展（第 15-20 周）

| 周 | 任务 | 产出 |
|----|------|------|
| W15 | 记忆后台整理（Consolidator） | 自动归档/合并 |
| W16 | 遗忘曲线 + 情感衰减 | 智能记忆管理 |
| W17 | 因果推理增强 | 深度关系推导 |
| W18 | A/B 测试 + 用户反馈收集 | 数据驱动优化 |
| W19 | 性能优化（缓存/批量/索引） | 生产级性能 |
| W20 | 文档完善 + 监控告警 | 可运维系统 |

---

## 附录

### A. 相关文档

| 文档 | 内容 | 文件 |
|------|------|------|
| 分层记忆系统评估报告 | 开源调研 + 自研方案 + Go 实现步骤 | `MEMORY_PLAN_REPORT.md` |
| 情感微模型训练指南 | 训练路线 + 数据集 + 代码示例 | `EMOTION_MODEL_TRAINING_GUIDE.md` |
| 综合设计文档 | 本文档，整合以上所有内容 | `COMPREHENSIVE_DESIGN_DOCUMENT.md` |

### B. 关键术语

| 术语 | 说明 |
|------|------|
| L0/L1/L2 | 核心记忆/工作记忆/归档记忆三层架构 |
| MAU | 多模态原子单元（Multimodal Atomic Unit） |
| RLVER | 可验证情感奖励的强化学习（腾讯混元） |
| LoRA | 低秩适配，轻量级微调技术 |
| DPO | 直接偏好优化，简化版 RLHF |
| pgvector | PostgreSQL 的向量扩展 |
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
| MemOS | github.com/MemTensor/MemOS | MemCube 统一记忆抽象 |
| OmniMem | arxiv.org/abs/2507.14215 | 多模态终身记忆 |
| ScrapMem | arxiv.org/abs/2605.03804 | 光学遗忘+仿生记忆 |
| Clipto.AI | clipto.ai | 端侧多模态记忆 |
