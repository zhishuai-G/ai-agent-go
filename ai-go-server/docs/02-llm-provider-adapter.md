# 第二章：LLM 全平台接入

本章依据课程 [M02 LLM 全平台接入](https://liwenzhou.com/courses/ai-agent/02-llm-provider-adapter/) 实现。第一章解决“可靠地发一次模型请求”，第二章解决“上层 Agent 不必知道请求实际发给哪一家模型”。

这里的“全平台”并不意味着把各 Provider 的全部特性强行揉成同一套字段，而是建立一套足够小、语义明确的公共模型：普通对话、流式输出和 token 用量。Provider 的协议差异留在适配器内部，暂时没有统一表达的能力必须明确标记为不支持。

## 学习目标

1. 识别 OpenAI 兼容协议与 Anthropic Messages API 的关键差异。
2. 用 `llm.Provider` 隔离上层业务和厂商 HTTP 协议。
3. 复用第一章的 HTTP 传输层，避免每个 Provider 重复实现重试、连接池和取消逻辑。
4. 将不同 SSE 结束标记归一化为 `StreamChunk`。
5. 为结构化输出生成 JSON Schema，并对 token 和成本进行累计。
6. 用路由器实现主模型失败后的 Provider 降级。

## 统一抽象的边界

本章的核心接口位于 [`internal/llm/llm.go`](../internal/llm/llm.go)：

| 类型 | 作用 |
| --- | --- |
| `ChatRequest` | 统一模型名、消息列表、温度、最大 token 和流式开关 |
| `ChatResponse` | 统一完整文本、输入 token 和输出 token |
| `StreamChunk` | 统一流式增量或终止错误 |
| `Provider` | 统一 `Chat`、`ChatStream`、名称和能力声明 |
| `Capability` | 诚实地声明 Streaming、Thinking、Tools 是否已被当前抽象实现 |

`Temperature` 使用 `*float64`，而不是普通 `float64`。这样“调用方没有设置温度”和“调用方明确设为 0”是两个不同状态，序列化时也能正确省略未设置的字段。`NewChatRequest`、`WithTemperature` 与 `WithMaxTokens` 将这些可选配置集中在构造阶段。

当前统一请求尚未表达推理内容、工具定义和工具调用结果，所以即使个别厂商原生支持这些能力，`Capabilities().Thinking` 和 `Capabilities().Tools` 也必须保持 `false`。这能防止路由层把一个尚未定义好语义的功能错误地当成通用能力。

## 协议家族与适配方式

| 协议家族 | 本章实现 | 适配策略 |
| --- | --- | --- |
| OpenAI Chat Completions 兼容 | [`internal/provider/openai`](../internal/provider/openai) | 将统一请求基本原样编码为 `/chat/completions` 请求；通过不同 `baseURL` 复用 DeepSeek、豆包、千问、Ollama 等兼容服务 |
| Anthropic Messages | [`internal/provider/claude`](../internal/provider/claude) | 将 system 消息提到顶层 `system` 字段，设置必填 `max_tokens`、`x-api-key` 与 `anthropic-version`，并处理内容块数组 |

OpenAI 兼容并不等于所有平台完全没有差别，但它们在本章使用的聊天、Bearer 鉴权和 `[DONE]` 流式结束语义上足够接近。因此 [`openai.NewDeepSeek`](../internal/provider/openai/provider.go)、`NewDoubao`、`NewQwen` 与 `NewOllama` 只需预置不同地址即可复用同一个 Provider。

Claude 不能直接复用这层。Anthropic 要求 `max_tokens`，system prompt 不放在普通消息数组中，非流式返回的文本也位于 `content` 块数组。适配器的 `adaptRequest` 负责这种翻译，响应侧只拼接 `type == "text"` 的内容块。由于 M02 的 `Message` 还没有 `tool_call_id`，OpenAI `tool` 消息和 Claude 的其他未建模 role 都会被拒绝，而不是发送不完整的协议数据。

## 从 CLI 到 Provider

`minicall` 现在不再手工拼接 OpenAI HTTP 请求。它创建统一的 `llm.ChatRequest`，然后将请求交给 [`openai.Provider`](../internal/provider/openai/provider.go)。这让命令行入口只负责配置、提问和输出；切换为路由器或 Claude Provider 时，调用方式不需要改变。

```text
minicall
  -> llm.ChatRequest
  -> llm.Provider.Chat
  -> Provider 协议转换
  -> transport.Client.Do
  -> 标准化 ChatResponse
```

第一章的 `transport.Client` 仍是唯一出站入口，因此连接池、429/5xx 重试、`Retry-After`、请求体重放和 Context 取消在所有 Provider 上保持一致。

## 流式输出：通用解析与协议结束

[`transport.ParseSSE`](../internal/transport/sse.go) 只负责 SSE 的通用语法：忽略注释、按空行收集一个事件中的多行 `data:`，再将合并后的数据交给 Provider。它不吞掉任何厂商事件，因为 `[DONE]` 和 `message_stop` 的含义属于协议层。

- OpenAI 风格流从 `choices[0].delta.content` 提取文本，并以 `[DONE]` 结束。
- Claude 流只接受 `content_block_delta` 中 `text_delta` 的内容，并以 `message_stop` 结束。

两个适配器都将文本输出为 `llm.StreamChunk{Content: ...}`。若连接在结束标志前关闭，消费者会收到带错误的 Chunk；如果用户取消 Context，则不会再继续阻塞读取或发送。

## JSON Schema：让模型知道结构化结果形状

[`internal/schema`](../internal/schema/schema.go) 根据 Go 类型通过反射生成 JSON Schema 子集。它识别基本类型、结构体、数组、映射、指针及 `json`/`desc` 标签。

```go
type GetWeatherArgs struct {
    City string `json:"city" desc:"城市名"`
    Days int    `json:"days,omitempty" desc:"预报天数，默认 1"`
}

schema := schema.Generate(GetWeatherArgs{})
```

上例会生成一个 object Schema：`city` 是 required string，`days` 是可选 integer。它可在后续章节作为工具参数或结构化输出定义的输入。生成器刻意只覆盖此项目当前需要的稳定子集；复杂的 `oneOf`、引用定义和自定义 format 应在真实需求出现后再加入。

## Token 与成本

不同 Provider 返回用量字段的位置不同，但适配器返回的都是 `ChatResponse.InputTokens` 和 `OutputTokens`。[`internal/cost`](../internal/cost/cost.go) 以“每百万 token 的输入/输出价格”计算单次费用，并使用并发安全的 `Accumulator` 汇总一轮对话或工作流。

成本必须与实际成功 Provider 和实际模型关联。路由场景中，应使用 `Router.RouteChat` 返回的 Provider 名称选择对应价目表，而不是按默认模型估算。

## 多模型路由与降级

[`internal/router`](../internal/router/router.go) 将 Provider 和策略组合。默认 `Priority` 策略按注册顺序尝试：主 Provider 请求失败时再尝试下一个，成功后返回回答与 Provider 名称。

路由层有两个重要限制：

1. 一旦上游 Context 被取消，不再尝试备用 Provider。这是用户主动中断，不是可恢复的模型故障。
2. 流式请求在某个 Provider 已经开始发送内容后不能无缝切换到其他 Provider，否则用户会收到两段不连续的回答。因此 `ChatStream` 只在建立流之前进行降级。

[`internal/provider/registry`](../internal/provider/registry/registry.go) 展示了如何按已配置的密钥创建 Provider 集合，并始终提供本地 Ollama 作为开发选项。注册表本身不决定优先级，优先级由 Router 策略控制。

## 验证

运行完整测试和静态检查：

```bash
cd ai-go-server
make test
make vet
```

| 验证内容 | 测试位置 |
| --- | --- |
| 可选请求字段和显式零温度 | [`internal/llm/llm_test.go`](../internal/llm/llm_test.go) |
| SSE 多行聚合、EOF 与回调错误 | [`internal/transport/sse_test.go`](../internal/transport/sse_test.go) |
| OpenAI 兼容请求、Tool 拒绝与 `[DONE]` 流 | [`internal/provider/openai/provider_test.go`](../internal/provider/openai/provider_test.go) |
| Claude 请求翻译、认证头、内容块与 `message_stop` | [`internal/provider/claude/provider_test.go`](../internal/provider/claude/provider_test.go) |
| Schema 标签映射 | [`internal/schema/schema_test.go`](../internal/schema/schema_test.go) |
| 成本累计 | [`internal/cost/cost_test.go`](../internal/cost/cost_test.go) |
| Provider 失败降级和流式能力筛选 | [`internal/router/router_test.go`](../internal/router/router_test.go) |

测试使用 `httptest` 模拟模型服务，不需要真实 API Key。真实验证时仍可沿用第一章的 `.env` 配置和命令：

```bash
source .env
go run ./cmd/minicall -- "介绍 Go 中 Provider 适配器的作用"
```

## 本章结论

可替换 Provider 的关键不是写很多 SDK 包装，而是先定义稳定、有限且可验证的应用层语义。兼容协议应该复用，真正不同的协议应该翻译；能力尚未被统一时应显式拒绝。这样后续增加工具调用、多模态、推理模型或服务端 API 时，扩展的是明确的边界，而不是把厂商判断散落到 Agent 业务代码中。
