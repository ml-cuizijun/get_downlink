package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"get_downlink/config"
)

type FeishuService struct {
	cfg         *config.Config
	token       string
	tokenExpiry time.Time
	mu          sync.Mutex
}

func NewFeishuService(cfg *config.Config) *FeishuService {
	return &FeishuService{cfg: cfg}
}

func (f *FeishuService) GetTenantAccessToken() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.token != "" && time.Now().Before(f.tokenExpiry) {
		return f.token, nil
	}

	body := map[string]string{
		"app_id":     f.cfg.Feishu.AppID,
		"app_secret": f.cfg.Feishu.AppSecret,
	}
	jsonBody, _ := json.Marshal(body)

	resp, err := http.Post(
		"https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	if err != nil {
		return "", fmt.Errorf("获取 token 失败: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("解析 token 响应失败: %w", err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("获取 token 错误: code=%d, msg=%s", result.Code, result.Msg)
	}

	f.token = result.TenantAccessToken
	f.tokenExpiry = time.Now().Add(time.Duration(result.Expire-60) * time.Second)
	log.Printf("[飞书] 获取 tenant_access_token 成功，有效期 %d 秒", result.Expire)
	return f.token, nil
}

func (f *FeishuService) SendCardByID(chatID string) error {
	token, err := f.GetTenantAccessToken()
	if err != nil {
		return err
	}

	cardContent := map[string]interface{}{
		"type": "template",
		"data": map[string]interface{}{
			"template_id": f.cfg.CardID,
		},
	}
	cardJSON, _ := json.Marshal(cardContent)

	bodyMap := map[string]string{
		"receive_id": chatID,
		"msg_type":   "interactive",
		"content":    string(cardJSON),
	}
	jsonBody, _ := json.Marshal(bodyMap)

	url := "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id"
	req, _ := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送卡片失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[飞书] 发送卡片响应: %s", string(respBody))

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	json.Unmarshal(respBody, &result)
	if result.Code != 0 {
		return fmt.Errorf("发送卡片错误: code=%d, msg=%s", result.Code, result.Msg)
	}

	return nil
}

func buildDeployCard() map[string]interface{} {
	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"title":    map[string]interface{}{"tag": "plain_text", "content": "🚀 Jenkins 发布"},
			"template": "blue",
		},
		"elements": []interface{}{
			map[string]interface{}{
				"tag":  "markdown",
				"content": "请填写以下发布信息，然后点击「开始构建」按钮：",
			},
			map[string]interface{}{
				"tag":  "form",
				"name": "deploy_form",
				"elements": []interface{}{
					map[string]interface{}{
						"tag":  "select_static",
						"name": "ENV",
						"placeholder": map[string]interface{}{
							"tag": "plain_text", "content": "选择环境",
						},
						"options": []interface{}{
							map[string]interface{}{"text": map[string]interface{}{"tag": "plain_text", "content": "gc"}, "value": "gc"},
							map[string]interface{}{"text": map[string]interface{}{"tag": "plain_text", "content": "gap"}, "value": "gap"},
						},
					},
					map[string]interface{}{
						"tag":  "select_static",
						"name": "Build_Type",
						"placeholder": map[string]interface{}{
							"tag": "plain_text", "content": "选择构建类型",
						},
						"options": []interface{}{
							map[string]interface{}{"text": map[string]interface{}{"tag": "plain_text", "content": "full"}, "value": "full"},
							map[string]interface{}{"text": map[string]interface{}{"tag": "plain_text", "content": "incremental"}, "value": "incremental"},
						},
					},
					map[string]interface{}{
						"tag":           "input",
						"name":          "Branch",
						"placeholder":   map[string]interface{}{"tag": "plain_text", "content": "请输入分支名"},
						"default_value": "master",
					},
					map[string]interface{}{
						"tag":         "input",
						"name":        "Server",
						"placeholder": map[string]interface{}{"tag": "plain_text", "content": "请输入目标服务器"},
					},
					map[string]interface{}{
						"tag":         "button",
						"text":        map[string]interface{}{"tag": "plain_text", "content": "🚀 开始构建"},
						"type":        "primary",
						"action_type": "form_submit",
						"name":        "submit_deploy",
					},
				},
			},
		},
	}
}

func (f *FeishuService) ReplyCardWithTemplate(messageID, cardID string) (string, error) {
	token, err := f.GetTenantAccessToken()
	if err != nil {
		return "", err
	}

	cardContent := map[string]interface{}{
		"type": "template",
		"data": map[string]interface{}{
			"template_id": cardID,
		},
	}
	cardJSON, _ := json.Marshal(cardContent)

	body := map[string]string{
		"msg_type": "interactive",
		"content":  string(cardJSON),
	}
	jsonBody, _ := json.Marshal(body)

	url := fmt.Sprintf("https://open.feishu.cn/open-apis/im/v1/messages/%s/reply", messageID)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("回复卡片失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[飞书] 回复卡片响应: %s", string(respBody))

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	json.Unmarshal(respBody, &result)
	if result.Code != 0 {
		return "", fmt.Errorf("回复卡片错误: code=%d, msg=%s", result.Code, result.Msg)
	}
	return result.Data.MessageID, nil
}

func (f *FeishuService) ReplyCard(messageID string) error {
	_, err := f.ReplyCardWithTemplate(messageID, f.cfg.CardID)
	return err
}

func (f *FeishuService) ReplyText(messageID, text string) error {
	token, err := f.GetTenantAccessToken()
	if err != nil {
		return err
	}

	content, _ := json.Marshal(map[string]string{"text": text})
	body := map[string]string{
		"msg_type": "text",
		"content":  string(content),
	}
	jsonBody, _ := json.Marshal(body)

	url := fmt.Sprintf("https://open.feishu.cn/open-apis/im/v1/messages/%s/reply", messageID)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("回复消息失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[飞书] 回复消息响应: %s", string(respBody))
	return nil
}

func (f *FeishuService) SendInlineCard(chatID string, card map[string]interface{}) error {
	token, err := f.GetTenantAccessToken()
	if err != nil {
		return err
	}

	cardJSON, _ := json.Marshal(card)
	body := map[string]string{
		"receive_id": chatID,
		"msg_type":   "interactive",
		"content":    string(cardJSON),
	}
	jsonBody, _ := json.Marshal(body)

	url := "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id"
	req, _ := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送卡片失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[飞书] 发送内联卡片响应: %s", string(respBody))
	return nil
}

func (f *FeishuService) UpdateCardMessage(messageID string, card map[string]interface{}) error {
	token, err := f.GetTenantAccessToken()
	if err != nil {
		return err
	}

	cardJSON, _ := json.Marshal(card)
	body := map[string]string{
		"msg_type": "interactive",
		"content":  string(cardJSON),
	}
	jsonBody, _ := json.Marshal(body)

	url := fmt.Sprintf("https://open.feishu.cn/open-apis/im/v1/messages/%s", messageID)
	req, _ := http.NewRequest("PATCH", url, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("更新卡片失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[飞书] 更新卡片响应: %s", string(respBody))
	return nil
}

func (f *FeishuService) SendText(chatID, text string) error {
	token, err := f.GetTenantAccessToken()
	if err != nil {
		return err
	}

	content, _ := json.Marshal(map[string]string{"text": text})
	body := map[string]string{
		"receive_id": chatID,
		"msg_type":   "text",
		"content":    string(content),
	}
	jsonBody, _ := json.Marshal(body)

	url := "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id"
	req, _ := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送消息失败: %w", err)
	}
	defer resp.Body.Close()
	return nil
}
