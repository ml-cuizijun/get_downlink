package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"get_downlink/config"
	"get_downlink/service"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// cardMsgJobMap 记录发出的卡片消息 ID 对应哪个 Job（"apk" 或 "exe"）
var cardMsgJobMap sync.Map

func main() {
	cfg := config.Load()

	client := lark.NewClient(cfg.Feishu.AppID, cfg.Feishu.AppSecret)
	feishuSvc := service.NewFeishuService(cfg)
	jenkinsSvc := service.NewJenkinsService(cfg)

	token, err := feishuSvc.GetTenantAccessToken()
	if err != nil {
		log.Printf("⚠️  获取飞书 token 失败: %v", err)
	} else {
		log.Printf("✅ 飞书 token 获取成功: %s...", token[:20])
	}

	eventDispatcher := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			return handleMessage(ctx, event, client, cfg, feishuSvc)
		}).
		OnP2CardActionTrigger(func(ctx context.Context, event *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
			return handleCardAction(ctx, event, feishuSvc, jenkinsSvc, cfg)
		})

	wsClient := larkws.NewClient(cfg.Feishu.AppID, cfg.Feishu.AppSecret,
		larkws.WithEventHandler(eventDispatcher),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	log.Printf("========================================")
	log.Printf("🤖 客户端下载链接机器人")
	log.Printf("========================================")
	log.Printf("模式: WebSocket 长连接")
	log.Printf("飞书 App ID: %s", cfg.Feishu.AppID)
	log.Printf("Jenkins: %s", cfg.Jenkins.URL)
	log.Printf("[apk] Job: %s  Card: %s", cfg.DefaultJob, cfg.CardID)
	log.Printf("[exe] Job: %s  Card: %s", cfg.DefaultJobExe, cfg.CardIDExe)
	log.Printf("========================================")
	log.Printf("在群里 @ 机器人：")
	log.Printf("  /apk — 客户端 APK 下载链接")
	log.Printf("  /exe — 客户端 PC 启动器下载链接")
	log.Printf("========================================")

	err = wsClient.Start(context.Background())
	if err != nil {
		log.Fatalf("❌ WebSocket 连接失败: %v", err)
	}
}

func handleMessage(ctx context.Context, event *larkim.P2MessageReceiveV1, client *lark.Client, cfg *config.Config, feishuSvc *service.FeishuService) error {
	msgType := *event.Event.Message.MessageType
	chatID := *event.Event.Message.ChatId
	messageID := *event.Event.Message.MessageId

	log.Printf("[消息] chat_id=%s, message_id=%s, type=%s", chatID, messageID, msgType)

	if msgType != "text" {
		return nil
	}

	var content struct {
		Text string `json:"text"`
	}
	json.Unmarshal([]byte(*event.Event.Message.Content), &content)
	text := strings.TrimSpace(content.Text)

	for _, prefix := range []string{"@_user_1 ", "@_all "} {
		text = strings.TrimPrefix(text, prefix)
	}
	text = strings.TrimSpace(text)

	log.Printf("[消息] 解析后文本: '%s'", text)

	lower := strings.ToLower(text)

	type triggerDef struct {
		keywords []string
		cardID   string
		jobType  string
		label    string
	}
	triggerDefs := []triggerDef{
		{keywords: []string{"/exe"}, cardID: cfg.CardIDExe, jobType: "exe", label: "PC 启动器"},
		{keywords: []string{"/apk", "/clientapk"}, cardID: cfg.CardID, jobType: "apk", label: "APK"},
	}

	for _, td := range triggerDefs {
		for _, kw := range td.keywords {
			if strings.Contains(lower, kw) {
				log.Printf("[消息] 触发 %s 流程，回复卡片 message_id=%s", td.label, messageID)
				cardMsgID, err := feishuSvc.ReplyCardWithTemplate(messageID, td.cardID)
				if err != nil {
					log.Printf("[错误] 回复卡片失败: %v", err)
					replyText(client, messageID, "❌ 发送表单失败: "+err.Error())
				} else if cardMsgID != "" {
					cardMsgJobMap.Store(cardMsgID, td.jobType)
					log.Printf("[消息] 卡片 %s 绑定 jobType=%s", cardMsgID, td.jobType)
				}
				return nil
			}
		}
	}

	if strings.Contains(text, "/help") || strings.Contains(text, "帮助") {
		helpText := "🤖 客户端下载链接机器人\n\n" +
			"命令：\n" +
			"  /apk — 客户端 APK 下载链接\n" +
			"  /exe — 客户端 PC 启动器下载链接\n" +
			"  /help — 帮助\n\n" +
			"CardKit 表单项标识须与 Jenkins 参数名一致。"
		replyText(client, messageID, helpText)
		return nil
	}

	return nil
}

func replyText(client *lark.Client, messageID, text string) {
	content, _ := json.Marshal(map[string]string{"text": text})
	req := larkim.NewReplyMessageReqBuilder().
		MessageId(messageID).
		Body(larkim.NewReplyMessageReqBodyBuilder().
			MsgType("text").
			Content(string(content)).
			Build()).
		Build()

	resp, err := client.Im.Message.Reply(context.Background(), req)
	if err != nil {
		log.Printf("[错误] 回复消息失败: %v", err)
		return
	}
	if !resp.Success() {
		log.Printf("[错误] 回复消息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
}

// 本 Job Jenkins 参数为小写 env、branch；飞书表单项标识请同名（仍兼容 ENV/Branch 读取）
func handleCardAction(ctx context.Context, event *callback.CardActionTriggerEvent, feishuSvc *service.FeishuService, jenkinsSvc *service.JenkinsService, cfg *config.Config) (*callback.CardActionTriggerResponse, error) {
	_ = ctx
	action := event.Event.Action
	log.Printf("[卡片回调] tag=%s, name=%s, value=%+v, form_value=%+v",
		action.Tag, action.Name, action.Value, action.FormValue)

	if action.Tag == "select_static" || action.Tag == "select_person" ||
		action.Tag == "date_picker" || action.Tag == "picker_time" ||
		action.Tag == "picker_datetime" || action.Tag == "overflow" ||
		action.Tag == "input" {
		log.Printf("[卡片回调] 组件交互，忽略")
		return nil, nil
	}

	formValues := action.FormValue
	if formValues == nil {
		formValues = action.Value
	}
	if formValues == nil {
		formValues = map[string]interface{}{}
	}
	log.Printf("[卡片回调] 构建参数: %+v", formValues)

	openMsgID := ""
	if event.Event.Context != nil {
		openMsgID = event.Event.Context.OpenMessageID
	}

	defaultJob := cfg.DefaultJob
	if v, ok := cardMsgJobMap.Load(openMsgID); ok {
		if v.(string) == "exe" {
			defaultJob = cfg.DefaultJobExe
		}
	}

	jobName := getFormString(formValues, "JobName", "job_name")
	if jobName == "" {
		jobName = defaultJob
	}

	params := service.BuildParams{
		JobName: jobName,
		Env:     getFormString(formValues, "env", "ENV"),
		Branch:  getFormString(formValues, "branch", "Branch"),
	}

	chatID := ""
	operatorID := ""
	if event.Event.Context != nil {
		chatID = event.Event.Context.OpenChatID
	}
	if event.Event.Operator != nil {
		operatorID = event.Event.Operator.OpenID
	}

	log.Printf("[卡片回调] 用户 %s 提交: %+v, chat_id=%s", operatorID, params, chatID)

	go func() {
		result, err := jenkinsSvc.TriggerBuild(params)
		if chatID == "" {
			return
		}
		if err != nil {
			feishuSvc.SendInlineCard(chatID, buildStatusCard("error", params, "", err.Error()))
		} else if result.Success {
			feishuSvc.SendInlineCard(chatID, buildStatusCard("success", params, result.BuildURL, ""))
		} else {
			feishuSvc.SendInlineCard(chatID, buildStatusCard("error", params, "", result.Message))
		}
	}()

	return &callback.CardActionTriggerResponse{
		Toast: &callback.Toast{
			Type:    "info",
			Content: "✅ 发布请求已提交！",
		},
	}, nil
}

// 结果卡片仅展示 Job + env + branch
func buildStatusCard(status string, params service.BuildParams, buildURL string, errMsg string) map[string]interface{} {
	var headerTitle, headerTemplate string

	switch status {
	case "success":
		headerTitle = "✅ Jenkins 构建已触发"
		headerTemplate = "green"
	case "error":
		headerTitle = "❌ Jenkins 构建失败"
		headerTemplate = "red"
	}

	elements := []interface{}{
		map[string]interface{}{
			"tag":              "column_set",
			"flex_mode":        "none",
			"background_style": "default",
			"columns": []interface{}{
				map[string]interface{}{
					"tag":            "column",
					"width":          "weighted",
					"weight":         1,
					"vertical_align": "top",
					"elements": []interface{}{
						map[string]interface{}{"tag": "markdown", "content": "**Job 名称**"},
						map[string]interface{}{"tag": "markdown", "content": "**环境 (env)**"},
						map[string]interface{}{"tag": "markdown", "content": "**分支 (branch)**"},
					},
				},
				map[string]interface{}{
					"tag":            "column",
					"width":          "weighted",
					"weight":         2,
					"vertical_align": "top",
					"elements": []interface{}{
						map[string]interface{}{"tag": "markdown", "content": params.JobName},
						map[string]interface{}{"tag": "markdown", "content": params.Env},
						map[string]interface{}{"tag": "markdown", "content": params.Branch},
					},
				},
			},
		},
		map[string]interface{}{"tag": "hr"},
	}

	switch status {
	case "success":
		content := "✅ 构建已成功触发！"
		if buildURL != "" {
			content += fmt.Sprintf("\n🔗 [点击查看构建详情](%s)", buildURL)
		}
		elements = append(elements, map[string]interface{}{
			"tag":     "markdown",
			"content": content,
		})
	case "error":
		elements = append(elements, map[string]interface{}{
			"tag":     "markdown",
			"content": "❌ 构建失败：" + errMsg,
		})
	}

	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"title":    map[string]interface{}{"tag": "plain_text", "content": headerTitle},
			"template": headerTemplate,
		},
		"elements": elements,
	}
}

// 与 feishu-jenkins-bot 的 getFormString 一致
func getFormString(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch val := v.(type) {
			case string:
				return val
			case []interface{}:
				if len(val) > 0 {
					if s, ok := val[0].(string); ok {
						return s
					}
				}
			}
		}
	}
	return ""
}
