# cursor2api

把本机 Cursor CLI 转成 OpenAI 兼容 API，供 OpenClaw/Hermes 等 AI agent 使用。

需要：**Go ≥ 1.21**、**Cursor CLI**、已登录 Cursor 账号。

默认地址：`http://localhost:3010/v1`

---

## macOS

**1. 安装 Cursor CLI 并登录**

```bash
curl https://cursor.com/install -fsS | bash
agent login
```

**2. 启动**

```bash
cp config.example.json config.json
go build -o cursor2api ./src
./cursor2api
```

或：

```bash
chmod +x scripts/run.sh
./scripts/run.sh
```

**3. 开机自启（可选）**

```bash
chmod +x scripts/*.sh
./scripts/install-launchd.sh
```

卸载：`./scripts/uninstall-launchd.sh`

---

## Windows

**1. 安装 Cursor CLI 并登录**

```powershell
irm 'https://cursor.com/install?win32=true' | iex
agent login
```

**2. 启动**

```powershell
copy config.example.json config.json
go build -o cursor2api.exe ./src
.\cursor2api.exe
```

或：

```powershell
powershell -ExecutionPolicy Bypass -File scripts\run.ps1
```

**3. 开机自启（可选）**

任务计划程序 → 创建任务 → 触发器选「登录时」→ 操作填：

```
程序：powershell.exe
参数：-ExecutionPolicy Bypass -File "完整路径\scripts\run.ps1"
起始于：项目目录
```

---

## 客户端配置

| 项 | 填什么 |
|----|--------|
| API 基址 | `http://localhost:3010/v1` |
| API Key | `sk-cursor2api`（与 `config.json` 一致） |
| 测试地址 | `http://localhost:3010/v1/models` |
| 模型 | `auto` |

OpenClaw 示例：

```json
{
  "models": {
    "mode": "merge",
    "providers": {
      "cursor": {
        "baseUrl": "http://localhost:3010/v1",
        "apiKey": "sk-cursor2api",
        "api": "openai-completions",
        "models": [{ "id": "auto", "name": "Cursor Auto", "input": ["text"], "contextWindow": 200000, "maxTokens": 8192 }]
      }
    }
  },
  "agents": {
    "defaults": { "model": { "primary": "cursor/auto" } }
  }
}
```

---

## API

```http
Authorization: Bearer sk-cursor2api
```

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/health` | 健康检查（无需 Key） |
| GET | `/v1/models` | 模型列表 |
| POST | `/v1/chat/completions` | 对话 |

```bash
curl http://localhost:3010/v1/chat/completions \
  -H "Authorization: Bearer sk-cursor2api" \
  -H "Content-Type: application/json" \
  -d '{"model":"auto","user":"my-session","messages":[{"role":"user","content":"你好"}]}'
```

| 字段 | 说明 |
|------|------|
| `model` | 固定 `auto` |
| `messages` | 必填 |
| `user` | 建议填，多轮对话用 |
| `stream` | `true` 流式，默认非流式 |

---

## 说明

- 纯对话（`ask`），不主动改本地文件
- 仅本机使用，默认端口 `3010`
- 多轮对话保持相同 `user` 值
