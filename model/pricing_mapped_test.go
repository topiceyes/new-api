package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 模型广场"实际模型"映射求值: 链式重定向取链尾,纯自映射与成环都视为未映射。
func TestResolveChannelMappedModel(t *testing.T) {
	tests := []struct {
		name     string
		modelMap map[string]string
		origin   string
		want     string
		mapped   bool
	}{
		{"无映射", map[string]string{"a": "b"}, "x", "x", false},
		{"直接映射", map[string]string{"a": "b"}, "a", "b", true},
		{"链式到链尾", map[string]string{"a": "b", "b": "c"}, "a", "c", true},
		{"纯自映射视为未映射", map[string]string{"a": "a"}, "a", "a", false},
		{"链尾回到原点(中继合法)", map[string]string{"a": "b", "b": "a"}, "a", "", false},
		{"成环不含原点", map[string]string{"a": "b", "b": "c", "c": "b"}, "a", "", false},
		{"映射值为空串终止", map[string]string{"a": "", "b": "c"}, "a", "a", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, mapped := resolveChannelMappedModel(tt.modelMap, tt.origin)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.mapped, mapped)
		})
	}
}

// 模型广场只展示优先级最高的两档映射: 调用命中最高档,重试才落次档。
func TestPickMappedTiers(t *testing.T) {
	tiers := map[int64]map[string]bool{
		100: {"b": true, "a": true},
		50:  {"c": true},
		0:   {"d": true},
	}
	primary, fallback := pickMappedTiers(tiers)
	assert.Equal(t, []string{"a", "b"}, primary) // 档内排序
	assert.Equal(t, []string{"c"}, fallback)     // 只取次高,最低档不展示

	primary, fallback = pickMappedTiers(map[int64]map[string]bool{10: {"x": true}})
	assert.Equal(t, []string{"x"}, primary)
	assert.Nil(t, fallback)

	primary, fallback = pickMappedTiers(map[int64]map[string]bool{})
	assert.Nil(t, primary)
	assert.Nil(t, fallback)
}
