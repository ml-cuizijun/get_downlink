# get_downlink — 一个基于GO server的飞书交互卡片机器人

一个基于飞书 WebSocket 长连接模式的 Go 机器人，在飞书群内通过触发词唤起交互卡片表单，用户填写参数后自动调用 Jenkins API 触发构建任务，并将结果以卡片消息反馈到群内。

## 功能

| 触发词 | 卡片模板 | Jenkins Job | 说明 |
|--------|----------|-------------|------|
| `/apk` | `xxx` | `Third_Party_Business/客户端apk包下载链接` | 客户端 APK 子包下载链接生成 |
| `/exe` | `xxx` | `Third_Party_Business/客户端pc启动器下载链接` | PC 启动器下载链接生成 |
| `/help` | — | — | 显示帮助信息 |

**流程：**
1. 用户在飞书群 `@机器人 /apk` 或 `@机器人 /exe`
2. 机器人回复对应的 CardKit 交互卡片（表单）
3. 用户填写 `env`、`branch` 等参数后点击提交
4. 机器人接收表单数据，调用 Jenkins `buildWithParameters` API 触发构建
5. 构建触发结果以新卡片消息发送到群内

## 架构

```
┌──────────────┐     WebSocket 长连接     ┌───────────────────┐
│  飞书服务端    │◄──────────────────────►│   get_downlink     │
│              │   消息事件 / 卡片回调      │   (Go 程序)        │
└──────────────┘                         └────────┬──────────┘
                                                  │
                                         HTTP POST (Basic Auth)
                                                  │
                                                  ▼
                                         ┌────────────────┐
                                         │  Jenkins 服务器  │
                                         └────────────────┘
```

**关键特性：**
- **无需公网 IP**：使用飞书 WebSocket 长连接模式，Go 程序主动连接飞书服务器，适合内网部署
- **多任务支持**：通过 `sync.Map` 将卡片消息 ID 映射到 Job 类型，同一个机器人支持多个触发词对应不同的卡片和 Jenkins 任务
- **Token 自动刷新**：飞书 `tenant_access_token` 带过期缓存，自动续期
- **嵌套 Job 路径**：支持 Jenkins 文件夹嵌套路径（`a/b/c` → `/job/a/job/b/job/c`）

## 项目结构

```
get_downlink/
├── main.go              # 入口：WebSocket 事件分发、消息处理、卡片回调
├── config/
│   └── config.go        # 配置：飞书凭据、Jenkins 凭据、卡片 ID、Job 路径
├── service/
│   ├── feishu.go        # 飞书 API：Token 管理、发送/回复卡片、发送消息
│   └── jenkins.go       # Jenkins API：参数化构建触发
├── start.sh             # 启动/停止/重启脚本
├── go.mod
└── go.sum
```

## 配置

所有配置通过硬编码默认值 + 环境变量覆盖：

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `FEISHU_APP_ID` | `xxx` | 飞书应用 App ID |
| `FEISHU_APP_SECRET` | *(内置)* | 飞书应用 App Secret |
| `JENKINS_URL` | `https://jenkins.xx.xx.net` | Jenkins 地址 |
| `JENKINS_USER` | `zijuncui` | Jenkins 用户名 |
| `JENKINS_TOKEN` | *(内置)* | Jenkins API Token |
| `JENKINS_PARAM_ENV` | `env` | Jenkins 环境参数名 |
| `JENKINS_PARAM_BRANCH` | `branch` | Jenkins 分支参数名 |
| `CARD_ID` | `xxx` | APK 卡片模板 ID |
| `CARD_ID_EXE` | `xxx` | EXE 卡片模板 ID |
| `DEFAULT_JOB` | `Third_Party_Business/客户端apk包下载链接` | APK 默认 Job |
| `DEFAULT_JOB_EXE` | `Third_Party_Business/客户端pc启动器下载链接` | EXE 默认 Job |

## 编译与部署

**本机编译（macOS）：**

```bash
go build -o bot .
./bot
```

**交叉编译（Linux amd64）：**

```bash
GOOS=linux GOARCH=amd64 go build -o bot-linux .
```

**部署到服务器：**

```bash
scp bot-linux start.sh user@server:/data/app/crontab/get_downlink/
ssh user@server
cd /data/app/crontab/get_downlink
chmod +x bot-linux start.sh
./start.sh start
```

**管理命令：**

```bash
./start.sh start     # 启动
./start.sh stop      # 停止
./start.sh restart   # 重启
./start.sh status    # 查看状态
```

日志文件：`bot-linux.log`

## 飞书配置要求

1. 在飞书开放平台创建企业自建应用，开启**机器人**能力
2. 添加权限：`im:message`、`im:chat`、`card.action.trigger`
3. 在 CardKit 编辑器中创建卡片模板，表单项标识（name）须与 Jenkins 参数名一致（本项目为小写 `env`、`branch`）
4. 卡片模板的"发布"→"使用范围"中授权对应的 App ID
5. 应用发布时需关闭"对外共享"以通过企业审核。
6. 

