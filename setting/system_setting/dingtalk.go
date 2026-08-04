package system_setting

import "github.com/QuantumNous/new-api/setting/config"

type DingTalkSettings struct {
	Enabled      bool   `json:"enabled"`
	AppKey       string `json:"app_key"`
	AppSecret    string `json:"app_secret"`
	CorpId       string `json:"corp_id"`
	ClientId     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// 默认配置
var defaultDingTalkSettings = DingTalkSettings{}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("dingtalk", &defaultDingTalkSettings)
}

func GetDingTalkSettings() *DingTalkSettings {
	return &defaultDingTalkSettings
}
