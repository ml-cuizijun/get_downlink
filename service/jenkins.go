package service

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"get_downlink/config"
)

type JenkinsService struct {
	cfg *config.Config
}

type BuildParams struct {
	JobName string `json:"job_name"`
	Env     string `json:"env"`
	Branch  string `json:"branch"`
}

type BuildResult struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	BuildURL string `json:"build_url"`
}

func NewJenkinsService(cfg *config.Config) *JenkinsService {
	return &JenkinsService{cfg: cfg}
}

// nestedJobAPIPath 将 "a/b/c" 转为 /job/a/job/b/job/c（与 feishu-jenkins-bot 的单段 job 不同，此处支持文件夹下的 Job）
func nestedJobAPIPath(jobFullName string) string {
	parts := strings.Split(jobFullName, "/")
	var b strings.Builder
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		b.WriteString("/job/")
		b.WriteString(url.PathEscape(p))
	}
	return b.String()
}

func (j *JenkinsService) TriggerBuild(params BuildParams) (*BuildResult, error) {
	jobFullName := strings.TrimSpace(params.JobName)
	if jobFullName == "" {
		jobFullName = j.cfg.DefaultJob
	}

	log.Printf("[Jenkins] 触发构建: job=%s, env=%s, branch=%s",
		jobFullName, params.Env, params.Branch)

	base := strings.TrimRight(j.cfg.Jenkins.URL, "/")
	apiPath := nestedJobAPIPath(jobFullName)
	buildURL := base + apiPath + "/buildWithParameters"

	envKey := strings.TrimSpace(j.cfg.Jenkins.ParamEnv)
	if envKey == "" {
		envKey = "env"
	}
	branchKey := strings.TrimSpace(j.cfg.Jenkins.ParamBranch)
	if branchKey == "" {
		branchKey = "branch"
	}

	formData := url.Values{}
	if params.Env != "" {
		formData.Set(envKey, params.Env)
	}
	if params.Branch != "" {
		formData.Set(branchKey, params.Branch)
	}
	log.Printf("[Jenkins] buildWithParameters 表单键: %s, %s；body=%s", envKey, branchKey, formData.Encode())

	req, err := http.NewRequest("POST", buildURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.SetBasicAuth(j.cfg.Jenkins.Username, j.cfg.Jenkins.Token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Jenkins 失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[Jenkins] 响应状态: %d, body: %s", resp.StatusCode, truncateBody(string(body)))

	if resp.StatusCode == 201 || resp.StatusCode == 200 || resp.StatusCode == 302 {
		jobURL := base + apiPath
		return &BuildResult{
			Success:  true,
			Message:  "Jenkins 构建任务已触发",
			BuildURL: jobURL,
		}, nil
	}

	return &BuildResult{
		Success: false,
		Message: fmt.Sprintf("Jenkins 触发失败 (HTTP %d): %s", resp.StatusCode, truncateBody(string(body))),
	}, nil
}

func truncateBody(s string) string {
	if len(s) > 800 {
		return s[:800] + "...(truncated)"
	}
	return s
}
