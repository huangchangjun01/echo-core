# Echo Core · 虚拟情感陪伴微模型训练指南

> **版本**: v2.0  
> **日期**: 2026-06-23  
> **目标**: 为虚拟陪伴平台 Echo Core 设计情感微模型的训练方案  
> **受众**: 无模型训练基础的开发者  
> **分支**: `plan`  
> **更新**: 对齐最新架构：对话逻辑 + ReAct 在 Python，Go 只做会话管理

---

## 目录

1. [核心概念速览](#1-核心概念速览)
2. [训练路线总览](#2-训练路线总览)
3. [第一步：选择基座模型](#3-第一步选择基座模型)
4. [第二步：准备数据集](#4-第二步准备数据集)
5. [第三步：环境搭建](#5-第三步环境搭建)
6. [第四步：SFT 监督微调](#6-第四步sft-监督微调)
7. [第五步：DPO 偏好对齐](#7-第五步dpo-偏好对齐)
8. [第六步：情感强化（可选进阶）](#8-第六步情感强化可选进阶)
9. [第七步：模型评估](#9-第七步模型评估)
10. [第八步：部署与集成](#10-第八步部署与集成)
11. [数据集来源汇总](#11-数据集来源汇总)
12. [成本与硬件建议](#12-成本与硬件建议)
13. [常见问题 FAQ](#13-常见问题-faq)

---

## 1. 核心概念速览

在动手之前，先理解几个关键概念。不需要懂数学，只需要知道它们各自是做什么的。

| 概念 | 一句话解释 | 类比 |
|------|-----------|------|
| **预训练 (Pre-training)** | 让模型学会"说人话"，阅读海量文本学习语言规律 | 婴儿听大人说话，学会语言的基本规律 |
| **SFT (监督微调)** | 用高质量"问答对"教模型"按指令回答" | 老师示范"如何回答这个问题" |
| **DPO (直接偏好优化)** | 给模型看"好回答 vs 差回答"，让它学会偏好更好的 | 老师打分：这个回答比那个好 |
| **RLHF (人类反馈强化学习)** | 用奖励模型打分，通过强化学习让模型持续优化 | 训练狗狗：做对给奖励，做错不给 |
| **LoRA** | 只训练模型的一小部分参数，大幅降低显存需求 | 只给房子换壁纸，不动承重墙 |
| **基座模型 (Base Model)** | 已经预训练好的通用大模型，作为你的"原材料" | 买一块已经发酵好的面团 |
| **Tokenizer** | 把文字切成模型能理解的"小块"(token) | 把句子切成一个个字或词 |
| **Checkpoint** | 训练过程中保存的模型快照，可以随时恢复 | 游戏存档 |

---

## 2. 训练路线总览

对于虚拟情感陪伴场景，**不需要从零预训练**。业界标准做法是：

```
选择一个开源基座模型
        │
        ▼
  ┌─────────────────┐
  │  阶段一：SFT    │  ← 用情感对话数据教模型"如何陪伴"
  │  监督微调       │     耗时：1-3 天
  └────────┬────────┘
           │
           ▼
  ┌─────────────────┐
  │  阶段二：DPO    │  ← 用"好/差回答对比"让回复更温暖
  │  偏好对齐       │     耗时：半天-1 天
  └────────┬────────┘
           │
           ▼
  ┌─────────────────┐
  │  阶段三(可选)   │  ← 用情感奖励信号强化共情能力
  │  情感强化(RLVER)│     耗时：1-2 天
  └─────────────────┘
```

**为什么不需要预训练？**

预训练需要数百万美元和数千张 GPU，目的是让模型"学会语言"。开源社区已经有大量优秀的预训练模型（Qwen、LLaMA、DeepSeek 等），它们已经"会说话了"。你只需要教它们"如何做一个好的陪伴者"，这就是微调的使命。

---

## 3. 第一步：选择基座模型

### 3.1 推荐方案

| 模型 | 参数 | 显存需求(推理) | 显存需求(训练 LoRA) | 中文能力 | 推荐理由 |
|------|------|---------------|---------------------|---------|---------|
| **Qwen2.5-7B-Instruct** | 7B | ~16GB | ~24GB | 极强 | 阿里开源，中文最强基座，生态完善 |
| **Qwen2.5-14B-Instruct** | 14B | ~32GB | ~48GB | 极强 | 预算充足时首选，陪伴质量显著提升 |
| **LLaMA-3.2-3B-Instruct** | 3B | ~8GB | ~16GB | 中等 | 显存受限时选择，英文原生 |
| **DeepSeek-R1-Distill-Qwen-7B** | 7B | ~16GB | ~24GB | 强 | 带推理能力，适合深度情感对话 |

### 3.2 选择建议

- **预算有限 / 初次尝试**: Qwen2.5-7B-Instruct（单卡 24GB 即可训练）
- **追求陪伴质量**: Qwen2.5-14B-Instruct（需要 48GB 显存或双卡）
- **移动端部署**: LLaMA-3.2-3B-Instruct（量化后可在手机运行）

### 3.3 下载模型

```bash
# 安装 huggingface-cli
pip install huggingface-hub

# 下载 Qwen2.5-7B-Instruct（约 15GB）
huggingface-cli download Qwen/Qwen2.5-7B-Instruct --local-dir ./models/Qwen2.5-7B-Instruct

# 或者使用 modelscope（国内镜像，速度更快）
pip install modelscope
python -c "from modelscope import snapshot_download; snapshot_download('Qwen/Qwen2.5-7B-Instruct', cache_dir='./models')"
```

---

## 4. 第二步：准备数据集

### 4.1 数据集是训练的灵魂

数据集的质量直接决定了你的陪伴模型是"温暖贴心"还是"机械冰冷"。

### 4.2 数据集格式

训练数据需要整理成如下 JSON 格式：

```json
{
  "messages": [
    {"role": "system", "content": "你是一个温柔体贴的虚拟伴侣，善于倾听和共情。"},
    {"role": "user", "content": "今天工作压力好大，感觉快要撑不住了..."},
    {"role": "assistant", "content": "听起来你今天真的很不容易。工作上的压力累积到一定程度，确实会让人感到喘不过气。你愿意跟我多说说具体发生了什么吗？我在这里陪着你。"},
    {"role": "user", "content": "老板总是临时加任务，我已经加班两周了。"},
    {"role": "assistant", "content": "连续加班两周，还要面对临时加派的任务，换做是谁都会感到疲惫和委屈。你的感受是完全合理的。有没有想过找个机会和老板沟通一下你的工作量？或者，至少今晚给自己一个小小的喘息空间？"}
  ]
}
```

**关键字段说明**：
- `system`: 定义模型的"人格"，决定陪伴风格
- `user`: 用户的输入
- `assistant`: 期望模型的回复（这是模型要学习的"正确答案"）

### 4.3 数据来源（免费开源）

#### 来源一：efaqa-corpus-zh（心理咨询对话）

| 属性 | 详情 |
|------|------|
| 规模 | 20,000 条心理咨询对话 |
| 语言 | 中文 |
| 质量 | 专业心理咨询师标注，三级分类体系 |
| 获取 | https://gitcode.com/gh_mirrors/ef/efaqa-corpus-zh |
| 许可证 | 需申请使用证书 |
| 适用 | 情感支持、心理疏导场景 |

**数据示例**：
```json
{
  "title": "工作压力太大怎么办",
  "description": "最近工作压力很大...",
  "chats": [
    {"sender": "client", "value": "我最近工作压力特别大..."},
    {"sender": "counselor", "value": "听起来你最近承受了很大的压力..."}
  ],
  "label": {"level": "S1", "category": "工作压力"}
}
```

**转换脚本思路**：
```python
# 将 efaqa 格式转为训练格式
# 1. 读取每条记录的 chats 数组
# 2. 将 sender=client 转为 role=user
# 3. 将 sender=counselor 转为 role=assistant
# 4. 添加 system prompt 定义陪伴人格
# 5. 输出为 JSONL 格式（每行一个 JSON 对象）
```

#### 来源二：ESConv（情感支持对话）

| 属性 | 详情 |
|------|------|
| 规模 | 约 1,000 段多轮对话 |
| 语言 | 英文（需翻译或作为参考） |
| 质量 | 学术研究级，含策略标注 |
| 获取 | https://github.com/thu-coai/Emotional-Support-Conversation |
| 许可证 | 开源 |
| 适用 | 学习情感支持策略 |

#### 来源三：TIDE（PTSD 共情对话）

| 属性 | 详情 |
|------|------|
| 规模 | 10,000 段两轮对话，500 个 PTSD 人格 |
| 语言 | 英文 |
| 质量 | 临床级，专业心理学背景 |
| 获取 | https://huggingface.co/datasets/yenopoya/TIDE |
| 许可证 | 开源 |
| 适用 | 深度共情训练 |

#### 来源四：自建数据集（推荐，最关键）

**为什么必须自建？**

开源数据集是"通用心理咨询"风格，而你的陪伴平台需要独特的"人格"。比如：
- 活泼可爱型 vs 成熟稳重型
- 古风诗意型 vs 现代直率型
- 二次元角色型 vs 现实伴侣型

**自建方法**：

```
方法 A：人工编写（质量最高，量最少）
  - 找 3-5 个写手，按你定义的人格编写 500-1000 段对话
  - 每段 3-10 轮，覆盖常见情感场景

方法 B：LLM 辅助生成（质量中高，量可扩展）
  - 用 GPT-4o / Qwen-Max 生成对话草稿
  - 人工审核、修改、润色
  - 可快速扩展到 5000-10000 条

方法 C：用户数据回流（长期积累）
  - 上线后收集真实用户对话
  - 人工筛选高质量对话作为训练数据
  - 注意隐私合规（脱敏、用户授权）
```

**推荐组合策略**：

| 阶段 | 数据来源 | 数量 | 用途 |
|------|---------|------|------|
| 冷启动 | efaqa-corpus-zh + 自建 500 条 | ~3000 条 | SFT 初版 |
| 迭代优化 | 自建 3000 条 + 用户回流 | ~5000 条 | SFT 优化 + DPO |
| 持续进化 | 用户回流 + 人工审核 | 持续增长 | 定期重新训练 |

### 4.4 数据清洗清单

```
□ 去除含个人隐私的信息（手机号、地址、真实姓名）
□ 去除极端有害内容（自残、暴力详细描述）
□ 去除过短对话（少于 2 轮）
□ 去除质量差的回复（机械、重复、答非所问）
□ 统一编码格式（UTF-8）
□ 检查 JSON 格式合法性
□ 确保每段对话以 assistant 结尾
```

---

## 5. 第三步：环境搭建

### 5.1 硬件要求

| 配置 | 最低要求 | 推荐配置 |
|------|---------|---------|
| GPU | NVIDIA RTX 3090 (24GB) | NVIDIA RTX 4090 (24GB) 或 A100 (40/80GB) |
| 显存 | 24GB | 48GB+ |
| 内存 | 32GB | 64GB |
| 硬盘 | 100GB SSD | 500GB NVMe SSD |
| 系统 | Ubuntu 20.04+ | Ubuntu 22.04 LTS |

**没有 GPU 怎么办？**

- 使用云服务：AutoDL、阿里云 PAI、火山引擎（按小时计费，约 2-5 元/小时）
- 使用 Colab Pro：约 10 美元/月，提供 T4/V100 GPU

### 5.2 软件环境

```bash
# 1. 安装 Python 3.10+
python --version  # 确认 >= 3.10

# 2. 创建虚拟环境
python -m venv venv
source venv/bin/activate  # Linux/Mac
# venv\Scripts\activate  # Windows

# 3. 安装核心依赖
pip install torch torchvision torchaudio --index-url https://download.pytorch.org/whl/cu121
pip install transformers datasets accelerate peft trl bitsandbytes
pip install wandb  # 可选，用于训练可视化

# 4. 验证 GPU 可用
python -c "import torch; print(f'GPU: {torch.cuda.get_device_name(0)}, 显存: {torch.cuda.get_device_properties(0).total_memory / 1e9:.1f} GB')"
```

### 5.3 项目目录结构

```
emotion-model-training/
├── models/                    # 存放下载的基座模型
│   └── Qwen2.5-7B-Instruct/
├── data/                      # 数据集
│   ├── raw/                   # 原始数据
│   ├── processed/             # 清洗后的数据
│   └── sft_train.jsonl        # SFT 训练数据
│   └── dpo_train.jsonl        # DPO 训练数据
├── scripts/                   # 训练脚本
│   ├── sft_train.py           # SFT 训练脚本
│   ├── dpo_train.py           # DPO 训练脚本
│   └── data_prepare.py        # 数据预处理脚本
├── outputs/                   # 训练输出
│   ├── sft/                   # SFT 模型输出
│   └── dpo/                   # DPO 模型输出
└── configs/                   # 配置文件
    ├── sft_config.yaml
    └── dpo_config.yaml
```

---

## 6. 第四步：SFT 监督微调

### 6.1 什么是 SFT？

SFT 是训练的第一步，也是最核心的一步。你把准备好的"问答对"给模型看，让它学习"当用户这样说时，我应该怎样回应"。

### 6.2 使用 LoRA 降低显存需求

LoRA 是一种"轻量级微调"技术，只训练模型中不到 1% 的参数，但效果接近全参数微调。

```python
# scripts/sft_train.py
import torch
from transformers import (
    AutoModelForCausalLM,
    AutoTokenizer,
    TrainingArguments,
    DataCollatorForSeq2Seq
)
from peft import LoraConfig, get_peft_model, TaskType
from trl import SFTTrainer
from datasets import load_dataset

# ========== 配置 ==========
MODEL_NAME = "./models/Qwen2.5-7B-Instruct"
DATA_PATH = "./data/processed/sft_train.jsonl"
OUTPUT_DIR = "./outputs/sft"

# LoRA 配置（这些参数初学者不用改）
lora_config = LoraConfig(
    r=16,                    # LoRA 秩，越大效果越好但显存越高
    lora_alpha=32,           # 缩放因子
    target_modules=[         # 要训练的模块
        "q_proj", "k_proj", "v_proj", "o_proj",
        "gate_proj", "up_proj", "down_proj"
    ],
    lora_dropout=0.05,       # 防止过拟合
    bias="none",
    task_type=TaskType.CAUSAL_LM,
)

# ========== 加载模型和分词器 ==========
print("正在加载模型...")
model = AutoModelForCausalLM.from_pretrained(
    MODEL_NAME,
    torch_dtype=torch.bfloat16,    # 使用 bfloat16 节省显存
    device_map="auto",              # 自动分配层到 GPU
)
tokenizer = AutoTokenizer.from_pretrained(MODEL_NAME)
tokenizer.pad_token = tokenizer.eos_token

# 应用 LoRA
model = get_peft_model(model, lora_config)
model.print_trainable_parameters()  # 查看可训练参数占比

# ========== 加载数据 ==========
print("正在加载数据...")
dataset = load_dataset("json", data_files=DATA_PATH, split="train")

# ========== 训练参数 ==========
training_args = TrainingArguments(
    output_dir=OUTPUT_DIR,
    num_train_epochs=3,             # 训练轮数
    per_device_train_batch_size=1,  # 每设备批次大小（显存小设为1）
    gradient_accumulation_steps=8,  # 梯度累积步数（有效 batch = 1*8 = 8）
    learning_rate=2e-4,             # 学习率
    max_grad_norm=0.3,              # 梯度裁剪
    warmup_ratio=0.03,              # 预热比例
    lr_scheduler_type="cosine",     # 学习率衰减策略
    logging_steps=10,               # 每10步打印日志
    save_steps=500,                 # 每500步保存一次
    save_total_limit=3,             # 最多保留3个checkpoint
    bf16=True,                      # 使用 bfloat16
    report_to="none",               # 不使用 wandb（初学者）
    remove_unused_columns=False,
)

# ========== 开始训练 ==========
print("开始 SFT 训练...")
trainer = SFTTrainer(
    model=model,
    tokenizer=tokenizer,
    train_dataset=dataset,
    args=training_args,
    max_seq_length=2048,            # 最大序列长度
    dataset_text_field="text",      # 数据集中的文本字段
    data_collator=DataCollatorForSeq2Seq(tokenizer, padding=True),
)

trainer.train()

# 保存最终模型
model.save_pretrained(f"{OUTPUT_DIR}/final")
tokenizer.save_pretrained(f"{OUTPUT_DIR}/final")
print(f"训练完成！模型已保存到 {OUTPUT_DIR}/final")
```

### 6.3 数据预处理脚本

```python
# scripts/data_prepare.py
import json

def prepare_sft_data(input_file, output_file, system_prompt):
    """将原始对话数据转换为 SFT 训练格式"""
    with open(input_file, 'r', encoding='utf-8') as f:
        raw_data = [json.loads(line) for line in f]
    
    processed = []
    for item in raw_data:
        messages = [{"role": "system", "content": system_prompt}]
        
        # 假设原始数据格式: [{"sender": "user", "text": "..."}, ...]
        for chat in item.get("chats", []):
            role = "user" if chat["sender"] == "client" else "assistant"
            messages.append({"role": role, "content": chat["value"]})
        
        # 转换为模型训练需要的文本格式
        text = tokenizer.apply_chat_template(messages, tokenize=False)
        processed.append({"text": text})
    
    with open(output_file, 'w', encoding='utf-8') as f:
        for item in processed:
            f.write(json.dumps(item, ensure_ascii=False) + '\n')
    
    print(f"处理完成：{len(processed)} 条数据 -> {output_file}")

# 使用示例
SYSTEM_PROMPT = """你是一个温柔体贴的虚拟伴侣，名叫"小暖"。你善于倾听、共情和安慰。
你的特点是：
- 说话温暖亲切，像知心朋友
- 善于察觉用户的情绪变化
- 不会说教，而是陪伴和理解
- 适度幽默，让对话轻松愉快
- 记住用户的喜好和经历，让对话有连续性"""

prepare_sft_data("./data/raw/efaqa.jsonl", "./data/processed/sft_train.jsonl", SYSTEM_PROMPT)
```

### 6.4 SFT 训练监控

训练过程中你会看到类似这样的输出：

```
{'loss': 2.3456, 'learning_rate': 1.89e-4, 'epoch': 0.5}
{'loss': 1.8765, 'learning_rate': 1.56e-4, 'epoch': 1.0}
{'loss': 1.2345, 'learning_rate': 1.02e-4, 'epoch': 2.0}
{'loss': 0.9876, 'learning_rate': 0.45e-4, 'epoch': 3.0}
```

**关键指标**：
- `loss` 持续下降 = 训练正常
- `loss` 降到 1.0 以下 = 模型已经学得很好了
- `loss` 突然飙升 = 学习率太大或数据有问题

### 6.5 SFT 训练后测试

```python
# 测试训练后的模型
from transformers import AutoModelForCausalLM, AutoTokenizer
from peft import PeftModel

base_model = AutoModelForCausalLM.from_pretrained(
    "./models/Qwen2.5-7B-Instruct",
    torch_dtype=torch.bfloat16,
    device_map="auto"
)
model = PeftModel.from_pretrained(base_model, "./outputs/sft/final")
tokenizer = AutoTokenizer.from_pretrained("./models/Qwen2.5-7B-Instruct")

messages = [
    {"role": "system", "content": SYSTEM_PROMPT},
    {"role": "user", "content": "我今天失恋了，好难过..."}
]

inputs = tokenizer.apply_chat_template(messages, return_tensors="pt").to("cuda")
outputs = model.generate(inputs, max_new_tokens=200, temperature=0.7)
response = tokenizer.decode(outputs[0], skip_special_tokens=True)
print(response)
```

---

## 7. 第五步：DPO 偏好对齐

### 7.1 什么是 DPO？

SFT 让模型"学会回答"，但不同回答有优劣之分。DPO 给模型看"好回答 vs 差回答"的对比，让它学会"哪种回答更好"。

对于情感陪伴场景，DPO 可以让模型：
- 选择更温暖、更共情的回答（而非机械回答）
- 避免说教式回应
- 学会在合适的时候提问、什么时候倾听

### 7.2 DPO 数据格式

```json
{
  "prompt": "我今天工作压力好大，感觉快要撑不住了...",
  "chosen": "听起来你今天真的很不容易。工作上的压力累积到一定程度，确实会让人感到喘不过气。你愿意跟我多说说具体发生了什么吗？我在这里陪着你。",
  "rejected": "工作压力是每个人都会遇到的，你可以试试时间管理方法，比如番茄工作法。另外多运动也能缓解压力。"
}
```

- `chosen`: 更好的回答（温暖、共情、陪伴）
- `rejected`: 较差的回答（机械、说教、冷漠）

### 7.3 构建 DPO 数据的方法

```
方法 1：人工标注（质量最高）
  - 对同一问题写 2-3 个不同回答
  - 人工选出最好的和最差的
  - 适合 500-2000 条规模

方法 2：LLM 生成 + 人工筛选（效率最高）
  - 用 GPT-4o 生成多个回答变体
  - 人工按"温暖度"排序
  - 取 top 作为 chosen，bottom 作为 rejected
  - 适合 2000-10000 条规模

方法 3：策略对比（推荐）
  - 定义几种回应策略：共情型、建议型、提问型、陪伴型
  - 对每种策略生成回答
  - 根据场景选择最优策略作为 chosen
```

### 7.4 DPO 训练脚本

```python
# scripts/dpo_train.py
import torch
from transformers import AutoModelForCausalLM, AutoTokenizer
from peft import LoraConfig, get_peft_model, TaskType, PeftModel
from trl import DPOTrainer, DPOConfig
from datasets import load_dataset

MODEL_NAME = "./outputs/sft/final"  # 基于 SFT 后的模型继续训练
DATA_PATH = "./data/processed/dpo_train.jsonl"
OUTPUT_DIR = "./outputs/dpo"

# 加载 SFT 后的模型
model = AutoModelForCausalLM.from_pretrained(
    "./models/Qwen2.5-7B-Instruct",
    torch_dtype=torch.bfloat16,
    device_map="auto",
)
model = PeftModel.from_pretrained(model, "./outputs/sft/final")
tokenizer = AutoTokenizer.from_pretrained("./models/Qwen2.5-7B-Instruct")

# DPO 配置
dpo_config = DPOConfig(
    output_dir=OUTPUT_DIR,
    num_train_epochs=1,                    # DPO 通常 1 轮即可
    per_device_train_batch_size=1,
    gradient_accumulation_steps=8,
    learning_rate=5e-5,                    # DPO 学习率比 SFT 小
    logging_steps=10,
    save_steps=200,
    bf16=True,
    beta=0.1,                              # DPO 温度参数，控制偏好强度
    max_length=2048,
    max_prompt_length=512,
)

# 加载 DPO 数据
dataset = load_dataset("json", data_files=DATA_PATH, split="train")

# 开始 DPO 训练
trainer = DPOTrainer(
    model=model,
    ref_model=None,  # 使用隐式参考模型（节省显存）
    args=dpo_config,
    train_dataset=dataset,
    tokenizer=tokenizer,
)

trainer.train()
model.save_pretrained(f"{OUTPUT_DIR}/final")
tokenizer.save_pretrained(f"{OUTPUT_DIR}/final")
print(f"DPO 训练完成！模型已保存到 {OUTPUT_DIR}/final")
```

---

## 8. 第六步：情感强化（可选进阶）

### 8.1 RLVER 框架简介

腾讯混元团队 2025 年提出的 RLVER 框架，通过"可验证的情感奖励"来强化模型的共情能力。在 Sentient-Benchmark 上，Qwen2.5-7B 经 RLVER 训练后得分从 13.3 飙升至 79.2，接近 GPT-4o 水平。[2]

### 8.2 核心思想

```
1. 构建一个"情感用户模拟器"（有特定人格、情绪状态）
2. 模型与模拟器进行多轮对话
3. 每轮对话后，模拟器给出情绪分数（0-100）
4. 模型通过强化学习（PPO/GRPO）优化，目标是提升情绪分数
5. 最终模型学会"让用户感觉更好"
```

### 8.3 实施建议

**对于初学者**：
- 先跳过此步骤，SFT + DPO 已经足够产出可用的陪伴模型
- 等模型上线后，收集用户满意度数据，再考虑引入 RLVER

**对于进阶团队**：
- 参考 RLVER 论文实现情感奖励函数
- 使用 TRL 库的 PPOTrainer 或 GRPOTrainer
- 需要更多算力和调参经验

---

## 9. 第七步：模型评估

### 9.1 自动评估指标

| 指标 | 工具 | 说明 |
|------|------|------|
| **Perplexity** | transformers | 困惑度，越低越好（语言流畅度） |
| **BLEU/ROUGE** | nltk/rouge | 与参考答案的文本相似度 |
| **BERTScore** | bert-score | 语义相似度（比 BLEU 更合理） |

### 9.2 情感专项评估

```python
# 评估脚本示例
test_cases = [
    {
        "input": "我今天失恋了，好难过...",
        "dimensions": ["共情度", "温暖度", "非说教", "适当提问"]
    },
    {
        "input": "最近总是失眠，很焦虑",
        "dimensions": ["共情度", "安全感", "非医疗建议", "陪伴感"]
    },
    # ... 更多测试用例
]

# 用 GPT-4o 作为裁判打分
for case in test_cases:
    response = model.generate(case["input"])
    score = gpt4_judge(response, case["dimensions"])
    print(f"输入: {case['input'][:30]}... 得分: {score}")
```

### 9.3 人工评估

找 10-20 个真实用户进行盲测：

```
测试设计：
1. 准备 10 个情感场景问题
2. 让模型和 GPT-4o 分别回答（随机顺序，不告诉用户哪个是哪个）
3. 用户从以下维度打分（1-5分）：
   - 温暖度
   - 理解力
   - 陪伴感
   - 自然度
   - 愿意继续聊
4. 统计哪个模型得分更高
```

### 9.4 A/B 测试（上线后）

```
1. 将 50% 用户流量分配给新模型，50% 给旧模型/基座模型
2. 追踪指标：
   - 对话轮数（越长说明越愿意聊）
   - 用户满意度评分
   - 次日留存率
   - 分享/推荐率
3. 运行 2 周后对比数据
```

---

## 10. 第八步：部署与集成

### 10.1 模型合并与导出

```python
# 合并 LoRA 权重到基座模型（推理时不需要 PEFT）
from peft import AutoPeftModelForCausalLM

model = AutoPeftModelForCausalLM.from_pretrained("./outputs/dpo/final")
merged_model = model.merge_and_unload()  # 合并权重
merged_model.save_pretrained("./outputs/emotion-companion-v1")
tokenizer.save_pretrained("./outputs/emotion-companion-v1")
```

### 10.2 量化部署（降低显存）

```bash
# 使用 llama.cpp 或 vLLM 进行量化部署
# 4-bit 量化后，7B 模型仅需 8GB 显存

# 方法 1：vLLM（推荐，推理速度快）
pip install vllm
python -m vllm.entrypoints.openai.api_server \
  --model ./outputs/emotion-companion-v1 \
  --quantization awq \
  --port 8000

# 方法 2：llama.cpp（CPU 也可运行）
# 转换为 GGUF 格式，支持 4-bit/5-bit/8-bit 量化
```

### 10.3 与 Echo Core 集成

```
架构概览：

Go 后端（Echo Core）                          Python AI 服务
┌──────────────────────┐        一次HTTP      ┌──────────────────────────┐
│  ChatService         │ ──────────────────►  │ 对话服务 (POST /chat/stream)│
│  ├── 加载历史消息     │                      │                          │
│  ├── 调 Python 对话   │                      │  ├── 记忆检索（进程内）    │
│  ├── 流式透传        │                      │  ├── 轻量 ReAct（2步）     │
│  └── 保存消息        │                      │  │   ├── 小模型前缀生成    │
│                      │                      │  │   └── 大模型续写        │
│                      │ ◄── SSE 流式 ──────── │  └── 流式返回            │
└──────────────────────┘                      └──────────────────────────┘
```

Python 对话服务配置：
```python
# .env
EMOTION_MODEL_BASE_URL=http://localhost:8000/v1    # vLLM 推理地址
EMOTION_MODEL_NAME=emotion-companion-v1            # 训练后的模型名
LARGE_MODEL_BASE_URL=https://api.openai.com/v1     # 大模型（续写用）
LARGE_MODEL_NAME=gpt-4o                            # 或 Qwen-Max
```

### 10.4 灰度发布策略

```
Week 1: 5% 用户使用情感微模型，监控异常
Week 2: 20% 用户，收集反馈
Week 3: 50% 用户，对比留存数据
Week 4: 100% 用户，全量上线
```

---

## 11. 数据集来源汇总

### 11.1 开源数据集一览

| 数据集 | 语言 | 规模 | 类型 | 获取地址 | 许可证 |
|--------|------|------|------|---------|--------|
| **efaqa-corpus-zh** | 中文 | 20,000 条 | 心理咨询对话 | gitcode.com/gh_mirrors/ef/efaqa-corpus-zh | 需证书 |
| **ESConv** | 英文 | ~1,000 段 | 情感支持对话 | github.com/thu-coai/ESConv | 开源 |
| **TIDE** | 英文 | 10,000 段 | PTSD 共情对话 | huggingface.co/datasets/yenopoya/TIDE | 开源 |
| **EmpatheticDialogues** | 英文 | 25,000 段 | 共情对话 | huggingface.co/datasets/empathetic_dialogues | 开源 |
| **DailyDialog** | 英文 | 13,000 段 | 日常对话 | huggingface.co/datasets/daily_dialog | 开源 |
| **CPsyCoun** | 中文 | ~3,000 段 | 中文心理对话 | huggingface.co/datasets | 开源 |
| **PsyQA** | 中文 | ~15,000 条 | 心理问答 | github.com/thu-coai/PsyQA | 开源 |
| **AI-HUB 情感对话** | 韩文 | 大规模 | 多语言情感 | aihub.or.kr | 需申请 |

### 11.2 数据集构建服务

| 服务 | 功能 | 成本 |
|------|------|------|
| **GPT-4o API** | 生成对话数据 | ~0.15 元/千 token |
| **Qwen-Max API** | 生成对话数据 | ~0.12 元/千 token |
| **Amazon Mechanical Turk** | 人工标注 | ~$0.1-0.5 / 条 |
| **Scale AI** | 专业数据标注 | 定制报价 |

---

## 12. 成本与硬件建议

### 12.1 训练成本估算（Qwen2.5-7B）

| 阶段 | 数据量 | 耗时 | 云 GPU 成本(AutoDL) |
|------|--------|------|-------------------|
| SFT (LoRA) | 3000 条 | 4-8 小时 | ~30-60 元 |
| DPO (LoRA) | 2000 条 | 2-4 小时 | ~15-30 元 |
| 总计 | - | 6-12 小时 | **~50-100 元** |

### 12.2 硬件采购建议

| 场景 | 推荐配置 | 价格 |
|------|---------|------|
| 个人学习 | RTX 3090 (24GB) | ~5000 元（二手） |
| 小团队 | RTX 4090 (24GB) | ~15000 元 |
| 专业训练 | A100 40GB x 2 | ~20 万元 |
| 云服务 | AutoDL RTX 4090 | ~2.5 元/小时 |

### 12.3 推理部署成本

| 部署方式 | 显存需求 | 并发能力 | 成本 |
|----------|---------|---------|------|
| 4-bit 量化 + vLLM | 8GB | 10 QPS | ~5000 元服务器 |
| 8-bit 量化 | 16GB | 5 QPS | ~10000 元服务器 |
| FP16 全精度 | 16GB | 3 QPS | ~10000 元服务器 |
| 云服务 API | 无 | 弹性 | ~0.003 元/千 token |

---

## 13. 常见问题 FAQ

### Q1: 我完全不懂深度学习，能跟着这份指南做吗？

**可以。** 这份指南假设你没有模型训练基础。你只需要：
- 会基本的 Python 编程
- 有一台带 NVIDIA GPU 的电脑（或云服务器预算）
- 按照步骤复制粘贴代码即可

建议先跑通一遍 SFT 训练，理解流程后再深入调优。

### Q2: 训练出来的模型和 GPT-4o 比怎么样？

7B 模型经过 SFT + DPO 后，在情感陪伴场景可以达到 GPT-4o 的 **60-70%** 水平。差距主要在：
- 知识广度（GPT-4o 知道更多世界知识）
- 复杂推理（多轮深度对话）
- 多语言能力

但在"温暖度"和"陪伴感"上，通过精心设计的训练数据，可以非常接近甚至超越 GPT-4o。

### Q3: 需要多少数据才能训练出好效果？

| 数据量 | 效果 |
|--------|------|
| 500 条 | 能对话，但回复较机械 |
| 2000 条 | 基本可用，有陪伴感 |
| 5000 条 | 较好的陪伴体验 |
| 10000 条+ | 接近商业产品水平 |

**质量比数量更重要。** 1000 条精心编写的对话 > 10000 条低质量对话。

### Q4: 训练过程中 loss 不下降怎么办？

检查清单：
1. 学习率是否太大？（尝试降到 1e-4 或 5e-5）
2. 数据格式是否正确？（确保是模型期望的格式）
3. 数据量是否太少？（至少 500 条）
4. 是否过拟合？（loss 很低但测试效果差，减少 epoch 或增加 dropout）

### Q5: 模型回复太啰嗦/太短怎么办？

- 在生成时调整 `max_new_tokens` 参数
- 在训练数据中控制回复长度
- 在 system prompt 中明确"回复长度要求"

### Q6: 如何让模型记住用户的喜好？

这就是 Echo Core 记忆系统的工作。训练好的模型本身不会"记住"跨会话信息，需要通过：
1. 记忆系统（见 MEMORY_PLAN_REPORT.md）将用户偏好注入 prompt
2. 模型 + 记忆系统协同工作

### Q7: 可以训练多个不同人格的模型吗？

可以。两种方式：
1. **分别训练**：为每个人格准备独立数据集，训练独立模型（成本高）
2. **统一训练 + 人格切换**：在 system prompt 中指定人格，一个模型服务多个人格（推荐）

```python
# 人格切换示例
persona_a = "你是一个活泼可爱的二次元少女..."
persona_b = "你是一个成熟稳重的知性伴侣..."

messages[0]["content"] = persona_a  # 切换到人格 A
# 或
messages[0]["content"] = persona_b  # 切换到人格 B
```

---

## 附录

### A. 推荐学习资源

| 资源 | 类型 | 链接 |
|------|------|------|
| Hugging Face Transformers 文档 | 官方文档 | huggingface.co/docs/transformers |
| TRL (Transformer Reinforcement Learning) | 训练库 | huggingface.co/docs/trl |
| PEFT (Parameter-Efficient Fine-Tuning) | 微调库 | huggingface.co/docs/peft |
| 大模型入门：从零到一 | 中文教程 | CSDN |
| 企业级大模型训练手册 V1.0 | 中文手册 | mininggoat.com |

### B. 关键论文

| 论文 | 作者 | 贡献 |
|------|------|------|
| RLVER: Reinforcement Learning with Verifiable Emotion Rewards | 腾讯混元 | 情感强化学习框架 [2] |
| DecoupledESC: Strategy-Response Decoupled Preference Optimization | 浙江大学 | 情感支持对话 DPO [4] |
| Distilling Empathy from Large Language Models | ACL 2025 | 小模型共情蒸馏 [1] |
| LoRA: Low-Rank Adaptation of Large Language Models | Microsoft | 轻量级微调方法 |

### C. 工具链版本参考

```
torch>=2.1.0
transformers>=4.41.0
datasets>=2.14.0
peft>=0.11.0
trl>=0.8.0
accelerate>=0.25.0
bitsandbytes>=0.43.0
```
