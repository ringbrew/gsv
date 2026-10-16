# 日志脱敏与调用方字段使用手册

本手册覆盖两件事：

- **脱敏**：`extra` 里的敏感值在输出前按 key 名自动掩码，业务代码不用改。
- **调用方字段**：服务端被调日志新增 `caller_service` / `peer` / `name`，HTTP 访问日志新增 `client_ip` / `query` / `caller_service`。

字段格式与掩码形态的规范见 [JSON_FORMAT.md](JSON_FORMAT.md)。

---

## 1. 升级

依赖链是 `gsv` ← `gsv-contrib` ← 业务服务，按这个顺序发版：

1. `gsv` 打新 tag。
2. `gsv-contrib` 把 `go.mod` 里的 `github.com/ringbrew/gsv` 升到上一步的 tag，再打新 tag。
3. 业务服务同时升级两者：

```bash
go get github.com/ringbrew/gsv@<新版本> github.com/ringbrew/gsv-contrib@<新版本>
```

**只升级 `gsv` 也有保护**：走 `logger.Info(...)` 这类包级函数的日志会在调用 Logger 实现之前就脱敏好。但直接拿 Logger 实例打日志的服务（如 `zl := zaplogger.New(); zl.Info(entry)`）必须同时升级 `gsv-contrib` 才有保护。

---

## 2. 升级后自动生效、无需改代码的部分

- 所有 `extra` 按默认敏感词表脱敏，包括嵌套 map、数组、proto message / struct，以及 `body` / `req` / `resp` / `response` / `payload` 里的 JSON 文本。
- HTTP 访问日志：`extra.header` 去掉 `Cookie`，`Authorization` 等头被掩码；`extra.query` 与表单 body 里的敏感参数被掩码。
- gRPC 服务端被调日志新增 `peer`、`name`（服务设置了 `Option.Name` 时）。
- gRPC stream handler 里打的日志现在带 `trace_id` / `span_id`（之前缺失）。

### 2.1 需要知晓的行为变化

| 变化 | 影响 | 如需恢复旧行为 |
|---|---|---|
| `extra` 一级字符串超过 4096 字节截断 | 大 body 尾部丢失 | `logger.SetRedactor(logger.NewRedactor(logger.WithMaxStringLen(0)))` |
| gRPC 服务端成功调用耗时超过 500ms 时 level 由 `info` 变为 `warn`，`msg` 仍是 `success` | 按 `level:info` 统计成功数的看板会少算慢调用 | `server.LogUnaryInterceptor(server.WithLogSlowThreshold(0))` |
| `cli` 客户端错误日志：`msg` 由 `call service[..]-method[..] req[..], error[..]` 改为 `err.Error()`，`service` / `method` / `duration` / `req` 移入 `extra` | 按旧 `msg` 文本检索的查询要改成按 `extra.method` 等字段检索 | 无。旧格式把 req 拼进 msg，无法脱敏 |
| HTTP 访问日志 `extra.header` 不再包含 `Cookie` | 无法从日志看 Cookie | 无 |

---

## 3. 调整脱敏规则

在 `main` 里、打第一条日志之前设置一次：

```go
import "github.com/ringbrew/gsv/logger"

logger.SetRedactor(logger.NewRedactor(
	logger.WithSensitiveKeys("wechat_openid", "bank_account"), // 增加敏感词
	logger.WithoutSensitiveKeys("email"),                      // 移除误伤的内置词
	logger.WithJSONKeys("raw_payload"),                        // 这些 key 的字符串值按 JSON 解析后脱敏
	logger.WithMaxStringLen(8192),                             // 一级字符串截断阈值，<=0 关闭
))
```

**必须用 `SetRedactor` 包住**。只调用 `logger.NewRedactor(...)` 只是创建了一个对象，不会生效，日志仍按默认词表脱敏。

匹配规则是"完全相等"或"以敏感词结尾"，不做前缀匹配：`phone` 能命中 `userPhone`，但命中不了 `phoneList`，这类字段要单独加。

| 选项 | 作用 |
|---|---|
| `WithSensitiveKeys(keys...)` | 增加敏感词。长度 ≥ 5 的词同时按后缀匹配，复数 `s` 结尾也会命中 |
| `WithoutSensitiveKeys(keys...)` | 移除敏感词，内置词也可移除 |
| `WithJSONKeys(keys...)` | 增加"值是 JSON 文本"的 key，任意层级都生效，包括 proto 字段（如 `jsonCred`） |
| `WithMaxStringLen(n)` | `extra` 一级字符串超过 n 字节截断，默认 4096，`<=0` 关闭 |
| `WithMaxDepth(n)` | 嵌套扫描深度，默认 8。更深的值原样输出 |
| `WithMask(fn)` | 自定义掩码。**必须满足 `fn(fn(s)) == fn(s)`**，因为同一条日志可能被脱敏两次 |
| `WithDeepMarshalerScan()` | 深入扫描实现了 `json.Marshaler` / `encoding.TextMarshaler` 的非 proto 类型（默认把它们当作不透明值）。proto message 不受此选项影响，总是会扫描 |

### 3.0 proto message

`protoc-gen-go-gsv` 生成的 message 自带 `MarshalJSON`（protojson，int64 输出为字符串、字段名 camelCase、空字段也输出）。脱敏时会复制一份 message、在副本上改值，输出仍走它自己的 `MarshalJSON`，所以除了被掩码的值，日志格式和原来完全一致。字段同时按 proto 名（`json_cred`）和 JSON 名（`jsonCred`）匹配。

proto 里敏感字段的掩码：字符串和 `repeated string` 按掩码规则处理；其他类型清空成零值，protojson 会输出 `"0"`、`0`、`false`、`""`、`[]`、`{}` 或 `null`，JSON 类型不变。oneof 里的非字符串敏感字段清空后不再输出。

完全关闭：`logger.SetRedactor(nil)`。

### 3.1 自己写的 Logger 实现

如果服务实现了自己的 `logger.Logger`，输出 extra 时用 `entry.RedactedExtra()` 而不是 `entry.Extra`：

```go
func (l *myLogger) Info(entry *logger.LogEntry) {
	l.write("info", entry.Message, entry.RedactedExtra())
}
```

`RedactedExtra()` 不修改原 map；没有需要掩码的值时直接返回原 map，不复制。

### 3.2 在日志之外复用

```go
safe := logger.RedactValue(req)          // 任意值，返回脱敏后的副本；无需改动时返回原值
q := logger.RedactQuery("a=1&token=xx")  // URL query 或表单 body
```

---

## 4. 调用方服务名

服务端从 gRPC metadata `x-gsv-caller`（HTTP 头 `X-Gsv-Caller`）读调用方名字，写进 `extra.caller_service`。**这需要调用方的客户端带上自己的名字**，三种方式任选其一，推荐第一种：

```go
import "github.com/ringbrew/gsv/cli"

// 1. 全局设置一次，所有 cli.Classic() 客户端生效（推荐）
cli.SetDefaultName("iamsvc")

// 2. 构造时指定
c, err := cli.NewClient(target, cli.Classic("iamsvc"))

// 3. 自己拼 Option 时指定，优先级最高
opt := cli.Option{Name: "iamsvc", UnaryInterceptors: []grpc.UnaryClientInterceptor{cli.TraceUnaryInterceptor()}}
c, err := cli.NewClient(target, opt)
```

不走 `cli.NewClient`、直接 `grpc.Dial` 的客户端，把 `cli.CallerUnaryInterceptor("iamsvc")` / `cli.CallerStreamInterceptor("iamsvc")` 加进拦截器链即可。

服务端的 `extra.name` 来自 `server.Option.Name`，设置了就会输出：

```go
opt := server.Classic()
opt.Name = "configsvc"
srv := server.NewServer(server.GRPC, &opt)
```

说明：

- 调用方名字是**覆盖写入**的。服务把收到的 metadata 整包转发给下游时，下游看到的仍是当前服务，而不是上游。
- 没带名字的老客户端不受影响，服务端只是不输出 `caller_service`；`peer` 照常输出，可以按 IP 反查。
- 这个字段只用于排查，不能当作鉴权依据：任何客户端都能填任意值。
- HTTP 调用方需要自己在请求里带 `X-Gsv-Caller: <服务名>` 头，`gsv` 不会替 HTTP 客户端自动加。

---

## 5. 性能

Apple M1 Pro 上的 benchmark（`go test ./logger/ -bench . -benchmem`）：

| 场景 | 耗时 | 分配 |
|---|---|---|
| 被级别过滤掉的日志 | 0 | 0 |
| 7 个标量 extra（典型的服务端被调日志） | 约 130ns | 0 |
| proto message，类型里没有敏感字段 | 约 55ns | 0 |
| 普通 struct，有敏感字段且有值 | 约 1.9µs（同一 struct 直接 `json.Marshal` 约 0.5µs） | 21 次 |
| protojson message，20 条 Account，无命中 | 约 8µs（protojson 编码本身约 86µs） | 1 次 |
| protojson message，20 条 Account，有命中 | 约 66µs（protojson 编码本身约 170µs） | 约 1000 次 |
| HTTP JSON body，不含敏感 key | 约 1ns/字节 | 0 |
| HTTP JSON body，含敏感 key | 约 22µs/KB | 约 350 次/KB |

对比：zap 编码并写出一条日志本身是微秒级。只有"body 确实含敏感 key"这一种情况代价较高，因为必须完整解析 JSON。

---

## 6. 限制

- **`msg` 不脱敏**，只处理 `extra`。不要把敏感值拼进 `msg`。另外，HTTP 访问日志在状态码 ≥ 400 时 `msg` 是完整的响应体。
- **struct / proto message 内部的长字符串不截断**；截断只作用于 `extra` 的一级字符串值。
- 超过最大深度（默认 8 层）的值原样输出。
- 实现了 `json.Marshaler` / `encoding.TextMarshaler` 的非 proto 类型（如 `time.Time`）默认不深入扫描，只按它自己的 key 名判断。
- proto message 里的 `google.protobuf.Any` 内容不扫描。
- 通过**未导出**的匿名嵌入 struct 提升上来的字段，在发生掩码的那一层会被省略。
- 掩码保持 JSON 类型：数字型敏感字段输出 `0`、布尔输出 `false`、对象输出 `{}`。看到 `"phone":0` 表示被掩码了，不是真实值。这样做是为了不和 Elasticsearch 里已有的字段映射冲突（冲突时整条日志会被拒）。
