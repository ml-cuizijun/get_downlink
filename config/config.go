package config

import "os"

type Config struct {
	Port          string
	Feishu        FeishuConfig
	Jenkins       JenkinsConfig
	CardID        string
	DefaultJob    string
	CardIDExe     string
	DefaultJobExe string
}

type FeishuConfig struct {
	AppID     string
	AppSecret string
}

type JenkinsConfig struct {
	URL         string
	Username    string
	Token       string
	ParamEnv    string // buildWithParameters 表单里的名字，须与 Job 参数名一致
	ParamBranch string
}

func Load() *Config {
	return &Config{
		Port: getEnv("PORT", "666"),
		Feishu: FeishuConfig{
			AppID:     getEnv("FEISHU_APP_ID", "xxx"),
			AppSecret: getEnv("FEISHU_APP_SECRET", "xxx"),
		},
		Jenkins: JenkinsConfig{
			URL:         getEnv("JENKINS_URL", "https://jenkins.xxx.xxx.net"),
			Username:    getEnv("JENKINS_USER", "zijuncui"),
			Token:       getEnv("JENKINS_TOKEN", "xxx"),
			ParamEnv:    getEnv("JENKINS_PARAM_ENV", "env"),
			ParamBranch: getEnv("JENKINS_PARAM_BRANCH", "branch"),
		},
		CardID:        getEnv("CARD_ID", "xxx"),
		DefaultJob:    getEnv("DEFAULT_JOB", "Third_Party_Business/客户端apk包下载链接"),
		CardIDExe:     getEnv("CARD_ID_EXE", "xxx"),
		DefaultJobExe: getEnv("DEFAULT_JOB_EXE", "Third_Party_Business/客户端pc启动器下载链接"),
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
