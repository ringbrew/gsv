# GSV 日志 JSON 格式规范（Filebeat 采集对齐）

本文是 **Go 服务现网日志格式的唯一约定**。Filebeat 按该 JSON 抽字段写入 Elasticsearch（索引 `yyyy.MM.dd_backend_log`）。  
Python / 其它语言服务只要 stdout 打出 **同结构的一行一条 JSON**，现有 Filebeat 配置即可采集，无需改采集端。

实现来源：

- 接口定义：`github.com/ringbrew/gsv/logger`
- 现网编码器：`github.com/ringbrew/gsv-contrib/logger/zaplogger`（`zap.NewProductionConfig()` + 自定义 `time`）
- 服务入口统一：`logger.SetLogger(zaplogger.New())`，日志打到 **stdout**

---

## 1. 传输约定（必须遵守）

| 项 | 要求 |
|---|---|
| 输出目标 | **stdout**（Docker `json-file` 驱动采集；容器需打 `filebeat: enable` label） |
| 记录形态 | **NDJSON**：一行一条，一条一个 JSON object |
| 编码 | UTF-8 |
| 换行 | 每条以 `\n` 结束 |
| 禁止 | JSON 前/后再加 logging 前缀、时间戳、级别文字；禁止 pretty-print（多行 JSON） |
| 空字段 | `trace_id` / `span_id` / `parent_id` / `extra` **没有值就省略，不要写 `null` 或 `""`** |

错误示例（Filebeat 解不出业务字段）：

```text
2026-09-04 16:46:37 INFO {"msg":"success"}
INFO:root:{"level":"info","msg":"success"}
```

---

## 2. 字段规范

字段顺序建议与 zap 一致：`level` → `time` → `caller` → `msg` → 可选链路字段 → `extra`。  
顺序不影响解析，**字段名、类型、取值必须一致**。

### 2.1 必填字段

| 字段 | 类型 | 取值 | 说明 |
|---|---|---|---|
| `level` | string | `debug` / `info` / `warn` / `error` / `panic` | **小写**。对应 zap `LowercaseLevelEncoder`。不要用 `INFO`、`WARNING`、`err` |
| `time` | string | RFC3339 / RFC3339Nano，**必须带时区** | 对应 Go `time.RFC3339Nano`。例：`2026-09-04T16:46:37.04313+08:00`。纳秒末尾 0 可省略。不要用 unix 秒、`2006-01-02 15:04:05` |
| `caller` | string | `file:line` 短路径 | 对应 zap `ShortCallerEncoder`。例：`cmd/configsvc.go:73`、`app/handler.py:128` |
| `msg` | string | 日志正文 | 键名是 **`msg`，不是 `message`** |

### 2.2 可选字段（有值才输出）

| 字段 | 类型 | 说明 |
|---|---|---|
| `trace_id` | string | 链路 trace id |
| `span_id` | string | 当前 span id |
| `parent_id` | string | 父 span id |
| `extra` | object | 业务扩展字段，**必须是嵌套对象**，不要摊平到根上 |

`extra` 常见键（Go 中间件已在用，Python 按需对齐即可）：

| extra 键 | 典型场景 | 类型 |
|---|---|---|
| `method` | gRPC full method 或 HTTP method | string |
| `duration` | 耗时，Go 用 `time.Duration.String()` | string，如 `"12.345ms"` |
| `code` | gRPC status code | number |
| `codeString` | gRPC status 文本 | string，如 `"OK"` |
| `path` / `host` / `status` / `size` | HTTP 访问日志 | string / number |
| `start_time` | HTTP 开始时间 | RFC3339Nano string |
| `peer` | gRPC 被调日志：对端地址 | string，如 `"10.0.3.17:52144"` |
| `caller_service` | gRPC / HTTP 被调日志：调用方服务名，未知时不输出 | string |
| `name` | 本服务名（gRPC 被调日志与 HTTP 访问日志） | string |
| `client_ip` | HTTP 访问日志：客户端 IP，取 `X-Forwarded-For` 首段 → `X-Real-IP` → 连接地址 | string |
| `query` | HTTP 访问日志：原始 query，已脱敏，为空时不输出 | string |

不要把 `level` / `time` / `caller` / `msg` / `trace_id` / `span_id` / `parent_id` / `extra` 再塞进 `extra`。

**调用方字段叫 `caller_service`，不是 `caller`**：根上的 `caller` 已经是 `file:line`。

### 2.3 敏感信息脱敏

Go 侧在输出前按 **key 名** 对 `extra` 做脱敏（含嵌套对象、数组、proto message，以及 `body` / `req` / `resp` / `response` / `payload` 里的 JSON 文本）。Kibana 里看到下面这些形态是正常的，不是数据损坏：

| 原值 | 输出 | 规则 |
|---|---|---|
| `""` | `""` | 空串不变 |
| 1–6 个字符，如 `123456` | `***` | 整体隐藏 |
| 7–10 个字符，如 `hunter22` | `h*****2` | 首 1 + `*****` + 尾 1 |
| 11 个字符以上，如 `13812345678` | `138******78` | 首 3 + `******` + 尾 2 |
| 数字，如 `13812345678`（number） | `0` | **保持 JSON 类型不变** |
| 布尔 | `false` | 同上 |
| 对象 / 数组 | `{}` / `[]` | 同上 |
| 字符串数组，如 `["Bearer abcdefghijklmnop"]` | `["Bea******op"]` | 逐个元素掩码 |
| 二进制（Go `[]byte`，编码为 base64 字符串） | `***` | |
| `null` | `null` | 不变 |
| 无法解析且含敏感 key 的 JSON 文本 | `***(unparsable, redacted)` | 整段隐藏 |
| `extra` 一级字符串超过 4096 字节 | 截断并以 `...(truncated)` 结尾 | 总长不超过 4096 字节 |

key 匹配规则：先转小写并去掉 `_` `-`（`id_card` / `idCard` / `ID-Card` 视为同一个），再与敏感词表 **完全相等**；长度 ≥ 5 的敏感词还按 **后缀** 匹配（`password` 命中 `user_password`）；末尾的复数 `s` 会去掉再匹配一次（`tokens` 命中 `token`）。默认敏感词表：

```text
password passwd pwd oldpassword newpassword token accesstoken refreshtoken idtoken
authorization cookie setcookie secret appsecret clientsecret apikey accesskey secretkey privatekey
signature sign idcard idcardno idno phone mobile telephone email bankcard cardno
credential session sessionid otp smscode verifycode
```

**Python / 其它语言服务必须对齐同一份敏感词表和掩码规则**，否则同一个字段会出现一边明文一边掩码的情况。

**脱敏不改变任何字段的 JSON 类型**：数字掩成 `0`、布尔掩成 `false`、对象掩成 `{}`，这样已经被 Elasticsearch 映射成数字或对象的字段不会因为突然收到字符串而导致整条日志被拒。非敏感字段也保持原样，例如 proto enum 仍是数字。注意数字型敏感字段输出的 `0` 是掩码结果，不是真实值。

### 2.4 level 映射

| 语义 | JSON `level` | 备注 |
|---|---|---|
| Debug | `debug` | |
| Info | `info` | 默认级别 |
| Warn | `warn` | 不要写成 `warning` |
| Error | `error` | |
| Fatal | `panic` | Go 实现里 Fatal 走的是 zap `Panic`，故现网值是 `panic`。Python 若没有等价 panic，Fatal 请仍输出 `panic`，以对齐检索 |

---

## 3. 完整对照 Example（按这个核对）

下面这一条是 **字段最全** 的现网形态：Go 怎么打、stdout 实际长什么样、Python 应对齐成什么样。  
`time` / `caller` 随运行变化，其余字段名、嵌套、类型必须一致。

### 3.1 Go 怎么打

```go
package main

import (
	"github.com/ringbrew/gsv-contrib/logger/zaplogger"
	"github.com/ringbrew/gsv/logger"
)

func main() {
	logger.SetLogger(zaplogger.New())

	entry := logger.NewEntry()
	entry.TraceId = "4bf92f3577b34da6a3ce929d0e0e4736"
	entry.SpanId = "00f067aa0ba902b7"
	entry.ParentId = "00f067aa0ba902b6"
	entry.WithExtra("start_time", "2026-09-04T16:46:37.000000001+08:00")
	entry.WithExtra("duration", "12.345ms")
	entry.WithExtra("host", "127.0.0.1:8003")
	entry.WithExtra("method", "POST")
	entry.WithExtra("path", "/config.Service/Get")
	entry.WithExtra("status", 200)
	entry.WithExtra("size", 128)
	entry.WithExtra("header", `{"Content-Type":["application/json"]}`)
	entry.WithExtra("body", `{"app_id":"demo","admin_id":1}`)
	entry.WithExtra("name", "config")

	logger.Info(entry.WithMessage("success"))
}
```

### 3.2 stdout 实际打出来的一行（Filebeat 看到的就是这一行）

下面是 zaplogger **原样输出**，无空格、单行、以 `\n` 结束：

```text
{"level":"info","time":"2026-09-04T16:52:28.450216+08:00","caller":"cmd/app.go:31","msg":"success","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7","parent_id":"00f067aa0ba902b6","extra":{"body":"{\"app_id\":\"demo\",\"admin_id\":1}","duration":"12.345ms","header":"{\"Content-Type\":[\"application/json\"]}","host":"127.0.0.1:8003","method":"POST","name":"config","path":"/config.Service/Get","size":128,"start_time":"2026-09-04T16:46:37.000000001+08:00","status":200}}
```

### 3.3 同一条拆开核对（pretty 仅供人看，不要按这个格式输出）

```json
{
  "level": "info",
  "time": "2026-09-04T16:52:28.450216+08:00",
  "caller": "cmd/app.go:31",
  "msg": "success",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "span_id": "00f067aa0ba902b7",
  "parent_id": "00f067aa0ba902b6",
  "extra": {
    "body": "{\"app_id\":\"demo\",\"admin_id\":1}",
    "duration": "12.345ms",
    "header": "{\"Content-Type\":[\"application/json\"]}",
    "host": "127.0.0.1:8003",
    "method": "POST",
    "name": "config",
    "path": "/config.Service/Get",
    "size": 128,
    "start_time": "2026-09-04T16:46:37.000000001+08:00",
    "status": 200
  }
}
```

| 路径 | 类型 | 本例值 | 核对要点 |
|---|---|---|---|
| `level` | string | `info` | 小写，不是 `INFO` |
| `time` | string | `2026-09-04T16:52:28.450216+08:00` | RFC3339，必须带 `+08:00` / `Z` |
| `caller` | string | `cmd/app.go:31` | `文件:行号`，短路径 |
| `msg` | string | `success` | 键名是 `msg`，不是 `message` |
| `trace_id` | string | `4bf92f3577b34da6a3ce929d0e0e4736` | 根上，不是 extra 里 |
| `span_id` | string | `00f067aa0ba902b7` | 根上 |
| `parent_id` | string | `00f067aa0ba902b6` | 根上 |
| `extra` | object | 见上 | **嵌套对象**，不要摊平到根 |
| `extra.method` | string | `POST` | |
| `extra.path` | string | `/config.Service/Get` | |
| `extra.host` | string | `127.0.0.1:8003` | |
| `extra.status` | number | `200` | 是数字，不是 `"200"` |
| `extra.size` | number | `128` | 是数字 |
| `extra.duration` | string | `12.345ms` | 是字符串，Go Duration 格式 |
| `extra.start_time` | string | `2026-09-04T16:46:37.000000001+08:00` | RFC3339Nano |
| `extra.name` | string | `config` | |
| `extra.header` | string | `{"Content-Type":["application/json"]}` | **字符串**，里面是 JSON 文本 |
| `extra.body` | string | `{"app_id":"demo","admin_id":1}` | **字符串**，里面是 JSON 文本 |

### 3.4 Python 应对齐成这样打

```python
info(
    "success",
    trace_id="4bf92f3577b34da6a3ce929d0e0e4736",
    span_id="00f067aa0ba902b7",
    parent_id="00f067aa0ba902b6",
    extra={
        "start_time": "2026-09-04T16:46:37.000000001+08:00",
        "duration": "12.345ms",
        "host": "127.0.0.1:8003",
        "method": "POST",
        "path": "/config.Service/Get",
        "status": 200,          # int
        "size": 128,            # int
        "header": '{"Content-Type":["application/json"]}',  # str
        "body": '{"app_id":"demo","admin_id":1}',            # str
        "name": "config",
    },
)
```

stdout 应长这样（`time` / `caller` 按运行时填，其它必须对得上）：

```text
{"level":"info","time":"2026-09-04T16:52:28.450216+08:00","caller":"app/handler.py:128","msg":"success","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7","parent_id":"00f067aa0ba902b6","extra":{"body":"{\"app_id\":\"demo\",\"admin_id\":1}","duration":"12.345ms","header":"{\"Content-Type\":[\"application/json\"]}","host":"127.0.0.1:8003","method":"POST","name":"config","path":"/config.Service/Get","size":128,"start_time":"2026-09-04T16:46:37.000000001+08:00","status":200}}
```

### 3.5 其它常见形态（同一编码器实打）

最小一条（无链路、无 extra，这两个键直接不出现）：

```text
{"level":"info","time":"2026-09-04T16:52:28.450024+08:00","caller":"cmd/app.go:12","msg":"rpc server start listen on: [9003]"}
```

gRPC 成功：

```text
{"level":"info","time":"2026-09-04T16:52:28.450203+08:00","caller":"cmd/app.go:22","msg":"success","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7","parent_id":"00f067aa0ba902b6","extra":{"code":0,"codeString":"OK","duration":"12.345ms","method":"/config.Service/Get"}}
```

gRPC 失败（有 trace / extra，没有 parent_id 就不要出这个键）：

```text
{"level":"error","time":"2026-09-04T16:52:28.450222+08:00","caller":"cmd/app.go:40","msg":"rpc error: code = Internal desc = db failure","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7","extra":{"code":13,"codeString":"Internal","duration":"3.21ms","method":"/config.Service/Get"}}
```

debug / warn：

```text
{"level":"debug","time":"2026-09-04T16:52:28.450229+08:00","caller":"cmd/app.go:42","msg":"debug sample"}
{"level":"warn","time":"2026-09-04T16:52:28.450234+08:00","caller":"cmd/app.go:43","msg":"warn sample"}
```

---

## 4. Python 参考实现

直接往 **stdout** 打 compact JSON，不要走默认 `logging` Formatter（默认会加 `INFO:logger:` 前缀，Filebeat 解失败）。

```python
import json
import sys
import traceback
from datetime import datetime, timezone, timedelta
from typing import Any, Mapping, Optional

# 与 zap LowercaseLevelEncoder 对齐
_LEVELS = {
    "debug": "debug",
    "info": "info",
    "warn": "warn",
    "warning": "warn",   # 输入兼容，输出仍必须是 warn
    "error": "error",
    "fatal": "panic",
    "panic": "panic",
}

# 东八区；有更好的 tz 库也行，但必须带 offset
_CST = timezone(timedelta(hours=8))


def _now() -> str:
    # 等价 Go time.RFC3339Nano：带偏移，微/纳秒末尾 0 可保留
    return datetime.now(_CST).isoformat()


def _caller() -> str:
    # 跳过本模块栈，取业务调用点；格式对齐 zap ShortCallerEncoder
    for frame in traceback.extract_stack()[:-2]:
        filename = frame.filename.replace("\\", "/")
        short = "/".join(filename.split("/")[-2:])
        return f"{short}:{frame.lineno}"
    return "unknown:0"


def log(
    level: str,
    msg: str,
    *,
    trace_id: str = "",
    span_id: str = "",
    parent_id: str = "",
    extra: Optional[Mapping[str, Any]] = None,
) -> None:
    record: dict[str, Any] = {
        "level": _LEVELS[level.lower()],
        "time": _now(),
        "caller": _caller(),
        "msg": msg,
    }
    if trace_id:
        record["trace_id"] = trace_id
    if span_id:
        record["span_id"] = span_id
    if parent_id:
        record["parent_id"] = parent_id
    if extra:
        record["extra"] = dict(extra)

    line = json.dumps(record, ensure_ascii=False, separators=(",", ":"))
    sys.stdout.write(line + "\n")
    sys.stdout.flush()


def info(msg: str, **kwargs: Any) -> None:
    log("info", msg, **kwargs)


def error(msg: str, **kwargs: Any) -> None:
    log("error", msg, **kwargs)
```

调用：

```python
info("rpc server start listen on: [9003]")

info(
    "success",
    trace_id="4bf92f3577b34da6a3ce929d0e0e4736",
    span_id="00f067aa0ba902b7",
    parent_id="00f067aa0ba902b6",
    extra={
        "method": "/config.Service/Get",
        "duration": "12.345ms",
        "codeString": "OK",
        "code": 0,
    },
)
```

若团队已有 `logging` Handler，自定义 Handler 的 `emit()` 里同样只 `sys.stdout.write(json + "\n")`，**不要**再套 `"%(levelname)s %(message)s"`。

---

## 5. 自测清单（交给开发自检）

对 stdout 抽几条，用 `jq` / `json.loads` 验证：

1. 整行是一个 JSON object，行首是 `{`，行尾是 `}`，没有其它前缀。
2. 存在 `level`、`time`、`caller`、`msg`。
3. `level` ∈ `debug|info|warn|error|panic`。
4. `time` 能被 RFC3339 解析，且带 `+08:00` / `Z` 等时区。
5. 正文在 `msg`，没有 `message` 顶栏字段。
6. 扩展字段在 `extra` 对象里，不在根上。
7. 无链路时没有 `trace_id` / `span_id` / `parent_id` 三个键。
8. 容器 `labels` 含 `filebeat: enable`，日志走 stdout。

对照一条 Go 现网日志（Kibana 里任意 `configsvc` / `iamsvc` 的 `success`）字段名应能一一对上。
