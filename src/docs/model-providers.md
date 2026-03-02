# 大模型厂商与配置说明

本文档说明 OpenOcta 支持的大模型厂商（Model Providers）、模型引用格式、以及如何通过配置文件或环境变量进行认证与自定义端点配置。格式与用法参考 [OpenClaw Model Providers](https://docs.openclaw.ai/concepts/model-providers)。

## 模型引用格式

模型使用 **`provider/modelId`** 形式引用，例如：

- `anthropic/claude-sonnet-4-5-20250929`
- `openai/gpt-4`
- `openrouter/auto`
- `moonshot/kimi-k2.5`
- `kimi-coding/k2p5`

在配置中指定默认模型时，可在 `agents.defaults.model.primary` 或单个 agent 的 `model` 中填写上述引用。

## 配置方式概览

1. **环境变量 / config.env.vars**  
   为各厂商设置对应的 API Key 环境变量（见下表），无需在 `models.providers` 中声明即可使用内置端点。

2. **config.models.providers**  
   需要自定义 Base URL、或使用非内置厂商时，在 `models.providers` 中为该 provider 配置 `baseUrl`、`apiKey`（可为字面量或 `$ENV_VAR`）、以及可选的 `api` 类型与 `models` 列表。

## 支持的内置厂商

以下厂商在**未**在 `models.providers` 中配置时，会使用下表默认 Base URL 和对应环境变量；若已在 `models.providers` 中配置，则优先使用配置中的 `baseUrl` 与 `apiKey`。

| 厂商 (provider) | 认证环境变量 | 默认模型示例 | 默认 Base URL | API 类型 |
|-----------------|-------------|--------------|---------------|----------|
| **anthropic**   | `ANTHROPIC_API_KEY` | claude-sonnet-4-5-20250929 | (官方) | Anthropic |
| **openai**      | `OPENAI_API_KEY`    | gpt-4                       | (官方) | OpenAI |
| **openrouter** | `OPENROUTER_API_KEY` | auto                        | https://openrouter.ai/api/v1 | OpenAI |
| **litellm**     | `LITELLM_API_KEY`   | (需指定)                    | http://localhost:4000 | OpenAI |
| **moonshot**    | `MOONSHOT_API_KEY`  | kimi-k2.5                   | https://api.moonshot.ai/v1 | OpenAI |
| **moonshot-cn** | `MOONSHOT_API_KEY`  | kimi-k2.5                   | https://api.moonshot.cn/v1 | OpenAI |
| **kimi-coding** | `KIMI_API_KEY`      | k2p5                        | https://api.moonshot.ai/anthropic | Anthropic |
| **opencode**    | `OPENCODE_API_KEY`  | claude-opus-4-6             | https://opencode.ai/zen/v1 | OpenAI |
| **zai**         | `ZAI_API_KEY`       | glm-5                       | https://api.z.ai/api/paas/v4 | OpenAI |
| **xai**         | `XAI_API_KEY`       | grok-3-mini                 | https://api.x.ai/v1 | OpenAI |
| **together**    | `TOGETHER_API_KEY`  | meta-llama/Llama-3.3-70B-Instruct-Turbo | https://api.together.xyz/v1 | OpenAI |
| **venice**      | `VENICE_API_KEY`    | falcon-3.1-70b              | https://api.venice.ai/api/v1 | OpenAI |
| **synthetic**   | `SYNTHETIC_API_KEY` | hf:MiniMaxAI/MiniMax-M2.1   | https://api.synthetic.new/anthropic | Anthropic |
| **qianfan**     | `QIANFAN_API_KEY`   | deepseek-v3-2-251201         | https://qianfan.baidubce.com/v2 | OpenAI |
| **huggingface** | `HUGGINGFACE_HUB_TOKEN` 或 `HF_TOKEN` | (需指定) | https://router.huggingface.co/v1 | OpenAI |
| **xiaomi**      | `XIAOMI_API_KEY`    | mimo-v2-flash               | https://api.xiaomimimo.com/anthropic | Anthropic |
| **minimax**     | `MINIMAX_API_KEY`   | MiniMax-M2.1                | https://api.minimax.io/anthropic | Anthropic |
| **mistral**     | `MISTRAL_API_KEY`   | mistral-large-latest        | https://api.mistral.ai/v1 | OpenAI |
| **groq**        | `GROQ_API_KEY`      | llama-3.3-70b-versatile     | https://api.groq.com/openai/v1 | OpenAI |
| **cerebras**    | `CEREBRAS_API_KEY`  | llama-4-scout-17b-16e-instruct | https://api.cerebras.ai/v1 | OpenAI |
| **ollama**      | `OLLAMA_API_KEY`（可选） | llama3.3                | http://127.0.0.1:11434/v1 | OpenAI |
| **vllm**        | `VLLM_API_KEY`（可选） | (需指定)                 | http://127.0.0.1:8000/v1 | OpenAI |
| **vercel-ai-gateway** | `AI_GATEWAY_API_KEY` | (需指定)           | https://api.vercel.ai/v1 | OpenAI |

说明：

- **API 类型**：`OpenAI` 表示兼容 OpenAI Chat Completions 的端点；`Anthropic` 表示兼容 Anthropic Messages API 的端点。自定义 `models.providers` 时可通过 `api: "openai-completions"` 或 `api: "anthropic-messages"` 指定。
- 环境变量可在 `config.env.vars` 中配置，也可直接使用系统环境变量；`config.env.vars` 优先级更高。
- 对于**未在 `models.providers` 中单独声明**的内置厂商，还可以通过以下环境变量覆盖默认模型和 Base URL：
  - 通用：`<PROVIDER>_MODEL`、`<PROVIDER>_BASE_URL`，例如：
    - `GROQ_MODEL`、`GROQ_BASE_URL`
    - `VENICE_MODEL`、`VENICE_BASE_URL`
  - 对于 **Kimi Coding**，为了与官方命名保持一致，前缀为 `KIMI` 而不是 `KIMI_CODING`：
    - `KIMI_MODEL`：覆盖默认模型 `k2p5`
    - `KIMI_BASE_URL`：覆盖默认 Base URL `https://api.moonshot.ai/anthropic`

## 配置示例

### 使用环境变量 + 默认端点

在 `env.vars` 或系统环境中设置 API Key，并在 agent 中指定模型引用即可：

```json
{
  "env": {
    "vars": {
      "ANTHROPIC_API_KEY": "sk-ant-...",
      "OPENROUTER_API_KEY": "sk-or-...",
      "MOONSHOT_API_KEY": "sk-..."
    }
  },
  "agents": {
    "defaults": {
      "model": { "primary": "anthropic/claude-sonnet-4-5-20250929" }
    },
    "list": [
      {
        "id": "main",
        "default": true,
        "name": "Clawd",
        "model": "moonshot/kimi-k2.5"
      }
    ]
  }
}
```

### OpenRouter

OpenRouter 统一接入多家模型，模型引用格式为 `openrouter/<可选子路径>/<模型名>`，例如 `openrouter/auto`、`openrouter/anthropic/claude-sonnet-4-5`。

```json
{
  "env": { "vars": { "OPENROUTER_API_KEY": "sk-or-..." } },
  "agents": {
    "defaults": { "model": { "primary": "openrouter/auto" } }
  }
}
```

### Moonshot（Kimi K2）

使用 Moonshot 开放平台、OpenAI 兼容端点：

```json
{
  "env": { "vars": { "MOONSHOT_API_KEY": "sk-..." } },
  "agents": {
    "defaults": { "model": { "primary": "moonshot/kimi-k2.5" } }
  }
}
```

### Kimi Coding

Kimi Coding 使用单独的端点和 API Key（与 Moonshot 开放平台密钥不通用），采用 Anthropic 兼容接口，模型引用为 `kimi-coding/k2p5`。

```json
{
  "env": {
    "vars": {
      "KIMI_API_KEY": "sk-...",
      "KIMI_MODEL": "k2p5",                               // 可选：覆盖默认模型
      "KIMI_BASE_URL": "https://api.moonshot.ai/anthropic" // 可选：覆盖默认 Base URL
    }
  },
  "agents": {
    "defaults": { "model": { "primary": "kimi-coding/k2p5" } }
  }
}
```

获取 API Key：<https://www.kimi.com/code/en>。

若不在 `models.providers` 中为 `kimi-coding` 显式配置 provider，运行时会：

- 从 `KIMI_API_KEY` 读取 API Key；
- 优先从 `KIMI_MODEL` 读取模型 ID（若未设置则回退到模型引用中的 ID 或内置默认 `k2p5`）；
- 优先从 `KIMI_BASE_URL` 读取 Base URL（若未设置则使用内置默认 `https://api.moonshot.ai/anthropic`）。

### 自定义 Base URL（models.providers）

需要自建代理、或使用与默认不同的端点时，在 `models.providers` 中为该 provider 配置 `baseUrl`、`apiKey` 和可选 `api`、`models`：

```json
{
  "env": { "vars": { "MOONSHOT_API_KEY": "sk-..." } },
  "agents": {
    "defaults": { "model": { "primary": "moonshot/kimi-k2.5" } }
  },
  "models": {
    "mode": "merge",
    "providers": {
      "moonshot": {
        "baseUrl": "https://api.moonshot.ai/v1",
        "apiKey": "$MOONSHOT_API_KEY",
        "api": "openai-completions",
        "models": [
          { "id": "kimi-k2.5", "name": "Kimi K2.5", "reasoning": false, "input": ["text"] }
        ]
      }
    }
  }
}
```

- `apiKey` 可为字面量，或以 `$` 开头表示环境变量名（如 `$MOONSHOT_API_KEY`），会从 `config.env.vars` 或系统环境中读取。
- `api`：`openai-completions` 使用 OpenAI 兼容客户端；`anthropic-messages` 使用 Anthropic 兼容客户端。

### Anthropic 兼容自定义端点（如 Synthetic、MiniMax）

```json
{
  "models": {
    "providers": {
      "synthetic": {
        "baseUrl": "https://api.synthetic.new/anthropic",
        "apiKey": "$SYNTHETIC_API_KEY",
        "api": "anthropic-messages",
        "models": [{ "id": "hf:MiniMaxAI/MiniMax-M2.1", "name": "MiniMax M2.1", "reasoning": false, "input": ["text"] }]
      }
    }
  },
  "agents": {
    "defaults": { "model": { "primary": "synthetic/hf:MiniMaxAI/MiniMax-M2.1" } }
  }
}
```

### LiteLLM / 本地代理

LiteLLM 默认连接本地 `http://localhost:4000`，模型引用需包含具体模型名（如 `litellm/claude-opus-4-6`），需在 agent 或 defaults 中明确指定。

```json
{
  "env": { "vars": { "LITELLM_API_KEY": "your-proxy-key" } },
  "agents": {
    "defaults": { "model": { "primary": "litellm/claude-opus-4-6" } }
  }
}
```

### Ollama 本地模型

Ollama 默认地址为 `http://127.0.0.1:11434/v1`，通常无需 API Key；若需占位可设置 `OLLAMA_API_KEY`。

```json
{
  "agents": {
    "defaults": { "model": { "primary": "ollama/llama3.3" } }
  }
}
```

## 配置项说明（models.providers）

| 字段 | 类型 | 说明 |
|------|------|------|
| `baseUrl` | string | 该 provider 的 API 根地址（如 `https://api.moonshot.ai/v1`）。 |
| `apiKey` | string | API 密钥；或以 `$` 开头表示环境变量名（如 `$MOONSHOT_API_KEY`）。 |
| `api` | string | 可选。`openai-completions`（默认）或 `anthropic-messages`，用于选择兼容的客户端。 |
| `models` | array | 可选。模型列表，每项包含 `id`、`name`、`reasoning`、`input` 等；用于校验与默认模型回退。 |

未在 `models.providers` 中配置的 provider 将使用上表中的内置默认 Base URL 与对应环境变量；若配置了该 provider，则完全以配置为准。

## 参考

- [OpenClaw Model Providers](https://docs.openclaw.ai/concepts/model-providers)  
- [OpenClaw Kimi Coding](https://docs.openclaw.ai/concepts/model-providers#kimi-coding)  
- [OpenOcta 配置说明](./configuration.md)
