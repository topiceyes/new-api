package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
)

// 三级部门筛选/归并的纯函数: 深度计算、三级可见集、成员卷到三级部门。
func TestDeptLevel3RollupHelpers(t *testing.T) {
	// 树: r(全体成员,0) -> A(1) -> B(2) -> C1(3), B(2) -> D(3) -> E(4)
	names := map[string]string{"r": "全体成员", "A": "一级A", "B": "二级B", "C1": "三级C1", "D": "三级D", "E": "四级E"}
	children := map[string][]string{"": {"r"}, "r": {"A"}, "A": {"B"}, "B": {"C1", "D"}, "D": {"E"}}
	parents := buildDeptParentMap(children)

	depths := computeDeptDepths(names, parents, children)
	assert.Equal(t, 0, depths["r"])
	assert.Equal(t, 1, depths["A"])
	assert.Equal(t, 2, depths["B"])
	assert.Equal(t, 3, depths["C1"])
	assert.Equal(t, 3, depths["D"])
	assert.Equal(t, 4, depths["E"])

	// 只有深度=3 的部门(自己挂在父下,无 L3 祖先的浅部门成员不参与)
	l3Map := buildLevel3AncestorMap(parents, depths)
	assert.Equal(t, "C1", l3Map["C1"])
	assert.Equal(t, "D", l3Map["D"])
	assert.Equal(t, "D", l3Map["E"]) // E(4级)卷到最近三级祖先 D
	assert.Equal(t, "", l3Map["B"])  // 二级部门成员不属于任何三级子树
	assert.Equal(t, "", l3Map["A"])

	// admin: 所有三级部门可见
	adminScope := &service.AnalyticsScope{Scope: "admin"}
	visible := computeVisibleLevel3Depts(depths, adminScope, parents)
	assert.Equal(t, map[string]bool{"C1": true, "D": true}, visible)

	// 负责人: 授权 B 子树,则 B 之下的三级部门 C1/D 都可见
	deptScope := &service.AnalyticsScope{Scope: "dept", DeptIds: []string{"B"}}
	visible = computeVisibleLevel3Depts(depths, deptScope, parents)
	assert.Equal(t, map[string]bool{"C1": true, "D": true}, visible)

	// 负责人: 只授权 C1,则只有 C1 可见
	deptScope = &service.AnalyticsScope{Scope: "dept", DeptIds: []string{"C1"}}
	visible = computeVisibleLevel3Depts(depths, deptScope, parents)
	assert.Equal(t, map[string]bool{"C1": true}, visible)
}
