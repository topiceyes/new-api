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
