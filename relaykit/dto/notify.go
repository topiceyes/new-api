package dto

type Notify struct {
	Type    string        `json:"type"`
	Title   string        `json:"title"`
	Content string        `json:"content"`
	Values  []interface{} `json:"values"`
}

const ContentValueParam = "{{value}}"

const (
	NotifyTypeQuotaExceed   = "quota_exceed"
	NotifyTypeChannelUpdate = "channel_update"
	NotifyTypeChannelTest   = "channel_test"
	// NotifyTypePlanUsageThreshold 套餐用量超阈值告警。实际使用的 type 会追加
	// _<planId>_<period> 后缀,让每个套餐每个周期的告警独立计数限流。
	NotifyTypePlanUsageThreshold = "plan_usage_threshold"
)

func NewNotify(t string, title string, content string, values []interface{}) Notify {
	return Notify{
		Type:    t,
		Title:   title,
		Content: content,
		Values:  values,
	}
}
