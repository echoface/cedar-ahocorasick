package cedar

import (
	"math/rand"
	"sort"
	"testing"
)

// FuzzCedarInsertGet 模糊测试 Insert/Get 的基本正确性
func FuzzCedarInsertGet(f *testing.F) {
	// 种子语料
	f.Add([]byte("hello"), 1)
	f.Add([]byte("world"), 2)
	f.Add([]byte("中国"), 3)
	f.Add([]byte("日本語"), 4)

	f.Fuzz(func(t *testing.T, key []byte, value int) {
		if len(key) == 0 {
			return // 空 key 不被支持
		}
		if value < 0 || value >= ValLimit {
			return // 跳过无效值
		}

		cd := New()

		// Insert
		err := cd.Insert(key, value)
		if err != nil {
			t.Fatalf("Insert(%q, %d) failed: %v", key, value, err)
		}

		// Get 应该返回相同的值
		got, err := cd.Get(key)
		if err != nil {
			t.Fatalf("Get(%q) failed after Insert: %v", key, err)
		}
		if got != value {
			t.Fatalf("Get(%q) = %d, want %d", key, got, value)
		}

		// ExactMatch 应该返回 true
		val, ok := cd.ExactMatch(key)
		if !ok {
			t.Fatalf("ExactMatch(%q) returned false after Insert", key)
		}
		if val != value {
			t.Fatalf("ExactMatch(%q) = %d, want %d", key, val, value)
		}
	})
}

// FuzzCedarInsertDelete 模糊测试 Insert/Delete 的正确性
func FuzzCedarInsertDelete(f *testing.F) {
	f.Add([]byte("test"), 42)

	f.Fuzz(func(t *testing.T, key []byte, value int) {
		if value < 0 || value >= ValLimit {
			return
		}

		cd := New()

		// Insert
		if err := cd.Insert(key, value); err != nil {
			t.Fatalf("Insert failed: %v", err)
		}

		// Delete
		if err := cd.Delete(key); err != nil {
			t.Fatalf("Delete(%q) failed: %v", key, err)
		}

		// Get 应该失败
		_, err := cd.Get(key)
		if err == nil {
			t.Fatalf("Get(%q) should fail after Delete", key)
		}

		// ExactMatch 应该返回 false
		_, ok := cd.ExactMatch(key)
		if ok {
			t.Fatalf("ExactMatch(%q) should return false after Delete", key)
		}
	})
}

// FuzzCedarPrefixMatch 模糊测试前缀匹配
func FuzzCedarPrefixMatch(f *testing.F) {
	f.Add([]byte("abc"), []byte("abcdef"), 1, 2)

	f.Fuzz(func(t *testing.T, prefix []byte, full []byte, v1 int, v2 int) {
		if v1 < 0 || v1 >= ValLimit || v2 < 0 || v2 >= ValLimit {
			return
		}
		if len(prefix) == 0 || len(full) == 0 {
			return
		}
		if len(prefix) > len(full) {
			return
		}

		cd := New()

		// Insert prefix 和 full
		if err := cd.Insert(prefix, v1); err != nil {
			t.Fatalf("Insert prefix failed: %v", err)
		}
		if err := cd.Insert(full, v2); err != nil {
			t.Fatalf("Insert full failed: %v", err)
		}

		// PrefixMatch on full 应该至少匹配 prefix
		ids := cd.PrefixMatch(full, 0)
		found := false
		for _, id := range ids {
			val, err := cd.Value(id)
			if err != nil {
				continue
			}
			if val == v1 {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("PrefixMatch(%q) should include prefix %q (value=%d), got ids=%v",
				full, prefix, v1, ids)
		}
	})
}

// FuzzCedarPrefixPredict 模糊测试前缀预测
func FuzzCedarPrefixPredict(f *testing.F) {
	f.Add([]byte("abc"), []byte("abcdef"), 1, 2)

	f.Fuzz(func(t *testing.T, prefix []byte, full []byte, v1 int, v2 int) {
		if v1 < 0 || v1 >= ValLimit || v2 < 0 || v2 >= ValLimit {
			return
		}
		if len(prefix) == 0 || len(full) == 0 {
			return
		}
		if len(prefix) > len(full) {
			return
		}

		cd := New()

		if err := cd.Insert(prefix, v1); err != nil {
			t.Fatalf("Insert prefix failed: %v", err)
		}
		if err := cd.Insert(full, v2); err != nil {
			t.Fatalf("Insert full failed: %v", err)
		}

		// PrefixPredict on prefix 应该包含 full
		ids := cd.PrefixPredict(prefix, 0)
		found := false
		for _, id := range ids {
			val, err := cd.Value(id)
			if err != nil {
				continue
			}
			if val == v2 {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("PrefixPredict(%q) should include %q (value=%d), got ids=%v",
				prefix, full, v2, ids)
		}
	})
}

// FuzzCedarUpdate 模糊测试 Update 累加
func FuzzCedarUpdate(f *testing.F) {
	f.Add([]byte("counter"), 1)

	f.Fuzz(func(t *testing.T, key []byte, delta int) {
		if delta <= 0 || delta >= ValLimit {
			return
		}

		cd := New()

		// 第一次 Update（相当于 Insert）
		if err := cd.Update(key, delta); err != nil {
			t.Fatalf("Update(%q, %d) failed: %v", key, delta, err)
		}

		val, err := cd.Get(key)
		if err != nil {
			t.Fatalf("Get after Update failed: %v", err)
		}
		if val != delta {
			t.Fatalf("Get after first Update = %d, want %d", val, delta)
		}

		// 第二次 Update（累加）
		if err := cd.Update(key, delta); err != nil {
			t.Fatalf("Second Update(%q, %d) failed: %v", key, delta, err)
		}

		val, err = cd.Get(key)
		if err != nil {
			t.Fatalf("Get after second Update failed: %v", err)
		}
		if val != delta*2 {
			t.Fatalf("Get after second Update = %d, want %d", val, delta*2)
		}
	})
}

// FuzzCedarJump 模糊测试 Jump 导航
func FuzzCedarJump(f *testing.F) {
	f.Add([]byte("hello"), []byte("world"), 1, 2)

	f.Fuzz(func(t *testing.T, key1 []byte, key2 []byte, v1 int, v2 int) {
		if v1 < 0 || v1 >= ValLimit || v2 < 0 || v2 >= ValLimit {
			return
		}
		if len(key1) == 0 || len(key2) == 0 {
			return
		}

		cd := New()

		// 插入 key1 和 key1+key2
		combined := append([]byte{}, key1...)
		combined = append(combined, key2...)

		if err := cd.Insert(key1, v1); err != nil {
			t.Fatalf("Insert key1 failed: %v", err)
		}
		if err := cd.Insert(combined, v2); err != nil {
			t.Fatalf("Insert combined failed: %v", err)
		}

		// Jump 到 key1
		node, err := cd.Jump(key1, 0)
		if err != nil {
			t.Fatalf("Jump(%q, 0) failed: %v", key1, err)
		}

		// 从 key1 节点继续 Jump key2
		node2, err := cd.Jump(key2, node)
		if err != nil {
			t.Fatalf("Jump(%q, %d) failed: %v", key2, node, err)
		}

		// node2 的值应该是 v2
		val, err := cd.Value(node2)
		if err != nil {
			t.Fatalf("Value(%d) failed: %v", node2, err)
		}
		if val != v2 {
			t.Fatalf("Value at combined node = %d, want %d", val, v2)
		}
	})
}

// FuzzCedarChinese 模糊测试中文关键词
func FuzzCedarChinese(f *testing.F) {
	// 种子：常见中文词
	f.Add([]byte("中国"), 1)
	f.Add([]byte("中国人"), 2)
	f.Add([]byte("中华人民共和国"), 3)
	f.Add([]byte("你好世界"), 4)
	f.Add([]byte("搜索引擎"), 5)

	f.Fuzz(func(t *testing.T, key []byte, value int) {
		if value < 0 || value >= ValLimit {
			return
		}
		if len(key) == 0 {
			return
		}

		cd := New()

		if err := cd.Insert(key, value); err != nil {
			t.Fatalf("Insert(%q, %d) failed: %v", key, value, err)
		}

		// 验证 Get
		got, err := cd.Get(key)
		if err != nil {
			t.Fatalf("Get(%q) failed: %v", key, err)
		}
		if got != value {
			t.Fatalf("Get(%q) = %d, want %d", key, got, value)
		}

		// 验证 PrefixMatch（key 自身应该是自己的前缀）
		ids := cd.PrefixMatch(key, 0)
		found := false
		for _, id := range ids {
			val, err := cd.Value(id)
			if err != nil {
				continue
			}
			if val == value {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("PrefixMatch(%q) should include itself (value=%d)", key, value)
		}
	})
}

// FuzzCedarACBasic 模糊测试 Aho-Corasick 基本正确性
func FuzzCedarACBasic(f *testing.F) {
	f.Add([]byte("he"), []byte("she"), []byte("his"), []byte("ushers"))

	f.Fuzz(func(t *testing.T, p1, p2, p3, text []byte) {
		if len(p1) == 0 || len(p2) == 0 || len(p3) == 0 || len(text) == 0 {
			return
		}

		ac := NewAhoCorasick()
		patterns := [][]byte{p1, p2, p3}

		for i, p := range patterns {
			if err := ac.Insert(p, i); err != nil {
				t.Fatalf("Insert pattern %d failed: %v", i, err)
			}
		}
		ac.Build()

		results := ac.Match(text)

		// 验证每个匹配结果的 Position 在有效范围内
		for _, r := range results {
			if r.Position < 1 || r.Position > len(text) {
				t.Errorf("invalid position %d (text len=%d)", r.Position, len(text))
			}
			if r.Value < 0 || r.Value >= len(patterns) {
				t.Errorf("invalid value %d (num patterns=%d)", r.Value, len(patterns))
			}
		}

		// 验证 MatchAll 的结果是 Match 的子集
		matchValues := make(map[int]bool)
		for _, r := range results {
			matchValues[r.Value] = true
		}
		for _, v := range ac.MatchAll(text) {
			if !matchValues[v] {
				t.Errorf("MatchAll returned value %d not in Match results", v)
			}
		}
	})
}

// FuzzCedarACDeterminism 模糊测试 Aho-Corasick 确定性
// 多次运行相同输入，结果必须一致
func FuzzCedarACDeterminism(f *testing.F) {
	f.Add([]byte("abc"), []byte("bc"), []byte("c"), []byte("abcabc"))

	f.Fuzz(func(t *testing.T, p1, p2, p3, text []byte) {
		if len(p1) == 0 || len(p2) == 0 || len(p3) == 0 || len(text) == 0 {
			return
		}

		patterns := [][]byte{p1, p2, p3}
		var baseline []MatchResult

		for round := 0; round < 5; round++ {
			ac := NewAhoCorasick()
			for i, p := range patterns {
				ac.Insert(p, i)
			}
			ac.Build()

			results := ac.Match(text)

			if round == 0 {
				baseline = results
				continue
			}

			if len(results) != len(baseline) {
				t.Errorf("round %d: got %d results, baseline has %d (non-deterministic!)",
					round, len(results), len(baseline))
				return
			}

			for i := range results {
				if results[i] != baseline[i] {
					t.Errorf("round %d: result[%d] = %+v, baseline[%d] = %+v (non-deterministic!)",
						round, i, results[i], i, baseline[i])
					return
				}
			}
		}
	})
}

// FuzzCedarACChinese 模糊测试 Aho-Corasick 中文匹配
func FuzzCedarACChinese(f *testing.F) {
	f.Add([]byte("中国"), []byte("中国人"), []byte("国民"), []byte("中国人民"))

	f.Fuzz(func(t *testing.T, p1, p2, p3, text []byte) {
		if len(p1) == 0 || len(p2) == 0 || len(p3) == 0 || len(text) == 0 {
			return
		}

		ac := NewAhoCorasick()
		patterns := [][]byte{p1, p2, p3}

		for i, p := range patterns {
			if err := ac.Insert(p, i); err != nil {
				t.Fatalf("Insert pattern %d failed: %v", i, err)
			}
		}
		ac.Build()

		results := ac.Match(text)

		// 验证：如果 text 包含某个 pattern 作为子串，应该被匹配到
		for i, p := range patterns {
			if containsBytes(text, p) {
				found := false
				for _, r := range results {
					if r.Value == i {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("pattern %d (%q) is a substring of %q but was not matched",
						i, p, text)
				}
			}
		}

		// 验证 MatchAll 一致性
		matchValues := make(map[int]bool)
		for _, r := range results {
			matchValues[r.Value] = true
		}
		for _, v := range ac.MatchAll(text) {
			if !matchValues[v] {
				t.Errorf("MatchAll inconsistency: value %d not in Match results", v)
			}
		}
	})
}

// FuzzCedarACInsertOrder 模糊测试插入顺序不影响匹配结果
func FuzzCedarACInsertOrder(f *testing.F) {
	f.Add([]byte("ab"), []byte("abc"), []byte("bc"), []byte("abcdabc"))

	f.Fuzz(func(t *testing.T, p1, p2, p3, text []byte) {
		if len(p1) == 0 || len(p2) == 0 || len(p3) == 0 || len(text) == 0 {
			return
		}

		patterns := [][]byte{p1, p2, p3}

		// 正向插入
		ac1 := NewAhoCorasick()
		for i, p := range patterns {
			ac1.Insert(p, i)
		}
		ac1.Build()
		r1 := ac1.Match(text)

		// 反向插入
		ac2 := NewAhoCorasick()
		for i := len(patterns) - 1; i >= 0; i-- {
			ac2.Insert(patterns[i], i)
		}
		ac2.Build()
		r2 := ac2.Match(text)

		// 比较匹配数量（value 不同因为插入顺序不同，但数量应一致）
		if len(r1) != len(r2) {
			t.Errorf("insert order affects match count: forward=%d, reverse=%d",
				len(r1), len(r2))
		}
	})
}

// FuzzCedarACEmptyAndSingle 模糊测试边界情况：空文本、单字符模式
func FuzzCedarACEmptyAndSingle(f *testing.F) {
	f.Add([]byte("a"), []byte(""))

	f.Fuzz(func(t *testing.T, pattern []byte, text []byte) {
		if len(pattern) == 0 {
			return
		}

		ac := NewAhoCorasick()
		ac.Insert(pattern, 0)
		ac.Build()

		results := ac.Match(text)

		// 空文本不应有匹配
		if len(text) == 0 && len(results) != 0 {
			t.Errorf("empty text should have 0 matches, got %d", len(results))
		}

		// 如果 pattern 是 text 的子串，应该被匹配
		if containsBytes(text, pattern) && len(results) == 0 {
			t.Errorf("pattern %q is substring of %q but not matched", pattern, text)
		}

		// 验证 Position 范围
		for _, r := range results {
			if r.Position < 1 || r.Position > len(text) {
				t.Errorf("position %d out of range [1, %d]", r.Position, len(text))
			}
		}
	})
}

// FuzzCedarACReducedVsNonReduced 模糊测试 Reduced/NonReduced 模式一致性
func FuzzCedarACReducedVsNonReduced(f *testing.F) {
	f.Add([]byte("ab"), []byte("abc"), []byte("bc"), []byte("abcdabc"))

	f.Fuzz(func(t *testing.T, p1, p2, p3, text []byte) {
		if len(p1) == 0 || len(p2) == 0 || len(p3) == 0 || len(text) == 0 {
			return
		}

		patterns := [][]byte{p1, p2, p3}

		// Reduced 模式
		acReduced := NewAhoCorasick(true)
		for i, p := range patterns {
			acReduced.Insert(p, i)
		}
		acReduced.Build()
		rReduced := acReduced.Match(text)

		// Non-Reduced 模式
		acNonReduced := NewAhoCorasick(false)
		for i, p := range patterns {
			acNonReduced.Insert(p, i)
		}
		acNonReduced.Build()
		rNonReduced := acNonReduced.Match(text)

		// 两种模式应该产生相同数量的匹配
		if len(rReduced) != len(rNonReduced) {
			t.Errorf("Reduced=%d matches, NonReduced=%d matches (should be equal)",
				len(rReduced), len(rNonReduced))
		}
	})
}

// 辅助函数

// containsBytes 检查 text 是否包含 pattern 作为连续子序列
func containsBytes(text, pattern []byte) bool {
	if len(pattern) == 0 {
		return false
	}
	if len(pattern) > len(text) {
		return false
	}
	for i := 0; i <= len(text)-len(pattern); i++ {
		match := true
		for j := 0; j < len(pattern); j++ {
			if text[i+j] != pattern[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// 确保 sort 被使用（用于 badcase_test.go）
var _ = sort.Strings
var _ = rand.Int
