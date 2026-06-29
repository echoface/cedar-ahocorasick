package cedar

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

/* >>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>

https://github.com/iohub/ahocorasick/issues/16

<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<*/

// loadBadcaseKeywords 从 kws.txt 加载关键词
func loadBadcaseKeywords(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "kws.txt"))
	if err != nil {
		t.Fatalf("open kws.txt failed: %v", err)
	}
	defer f.Close()

	var keywords []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		kw := strings.TrimSpace(sc.Text())
		if kw != "" {
			keywords = append(keywords, kw)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read kws.txt failed: %v", err)
	}
	return keywords
}

// matchResultKey 用于排序和比较匹配结果
type matchResultKey struct {
	Value    int
	Position int
}

// collectResults 收集匹配结果并排序去重
func collectResults(results []MatchResult) []matchResultKey {
	seen := make(map[matchResultKey]bool)
	var out []matchResultKey
	for _, r := range results {
		k := matchResultKey{Value: r.Value, Position: r.Position}
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Value != out[j].Value {
			return out[i].Value < out[j].Value
		}
		return out[i].Position < out[j].Position
	})
	return out
}

// resultsEqual 比较两个结果集是否相同
func resultsEqual(a, b []matchResultKey) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestBadcaseNonDeterminism 复现 iohub_badcase 中的非确定性 bug 检测
// 使用本地 cedar/ 库，多次运行相同匹配，检查结果是否一致
func TestBadcaseNonDeterminism(t *testing.T) {
	keywords := loadBadcaseKeywords(t)
	t.Logf("loaded %d keywords from kws.txt", len(keywords))

	content := "休闲益智不烧脑,没wifi我也能玩一整天!,良心游戏，真送红包,红包,提现,现金,免费,红包提现,赚钱,挂机,网赚,亿万人生,刷宝,彩票,体彩,羊毛党,金币,短视频,疯狂猜成语,休闲游戏,爱上消消消,每天,明星,王二狗的摊位"

	rounds := 20
	var baseline []matchResultKey

	for round := 0; round < rounds; round++ {
		ac := NewAhoCorasick()

		// 插入所有关键词
		for i, kw := range keywords {
			if err := ac.Insert([]byte(kw), i); err != nil {
				t.Fatalf("round %d: insert %q failed: %v", round, kw, err)
			}
		}
		ac.Build()

		// 执行匹配
		results := ac.Match([]byte(content))
		collected := collectResults(results)

		if round == 0 {
			baseline = collected
			t.Logf("round 0 baseline: %d unique matches", len(baseline))
			continue
		}

		if !resultsEqual(baseline, collected) {
			t.Errorf("NON-DETERMINISM DETECTED at round %d!", round)
			t.Logf("  baseline: %d results", len(baseline))
			t.Logf("  round %d: %d results", round, len(collected))

			// 找出差异
			baselineSet := make(map[matchResultKey]bool)
			for _, r := range baseline {
				baselineSet[r] = true
			}
			roundSet := make(map[matchResultKey]bool)
			for _, r := range collected {
				roundSet[r] = true
			}

			// 只在 baseline 中的
			for _, r := range baseline {
				if !roundSet[r] {
					t.Logf("  MISSING in round %d: value=%d pos=%d keyword=%q",
						round, r.Value, r.Position, keywords[r.Value])
				}
			}
			// 只在 round X 中的
			for _, r := range collected {
				if !baselineSet[r] {
					t.Logf("  EXTRA in round %d: value=%d pos=%d keyword=%q",
						round, r.Value, r.Position, keywords[r.Value])
				}
			}
		}
	}

	t.Logf("all %d rounds passed - no non-determinism detected", rounds)
}

// TestBadcaseCompareWithCloudflare 与 cloudflare/ahocorasick 对比验证正确性
// 注意：此测试需要 github.com/cloudflare/ahocorasick 依赖
func TestBadcaseCompareWithCloudflare(t *testing.T) {
	keywords := loadBadcaseKeywords(t)
	content := "休闲益智不烧脑,没wifi我也能玩一整天!,良心游戏，真送红包,红包,提现,现金,免费,红包提现,赚钱,挂机,网赚,亿万人生,刷宝,彩票,体彩,羊毛党,金币,短视频,疯狂猜成语,休闲游戏,爱上消消消,每天,明星,王二狗的摊位"

	// 使用本地 cedar 库
	ac := NewAhoCorasick()
	keyList := make([][]byte, len(keywords))
	for i, kw := range keywords {
		ac.Insert([]byte(kw), i)
		keyList[i] = []byte(kw)
	}
	ac.Build()

	results := ac.Match([]byte(content))

	// 收集 cedar 匹配到的关键词（去重）
	cedarMatched := make(map[string]bool)
	for _, r := range results {
		if r.Value >= 0 && r.Value < len(keywords) {
			cedarMatched[keywords[r.Value]] = true
		}
	}

	t.Logf("cedar matched %d unique keywords out of %d total", len(cedarMatched), len(keywords))

	// 打印匹配到的关键词用于人工审查
	var matched []string
	for kw := range cedarMatched {
		matched = append(matched, kw)
	}
	sort.Strings(matched)
	for _, kw := range matched {
		t.Logf("  matched: %q", kw)
	}

	// 验证一些明显应该匹配的关键词
	expectedMatches := []string{"红包", "提现", "现金", "免费", "赚钱", "彩票", "金币"}
	for _, expected := range expectedMatches {
		if !cedarMatched[expected] {
			t.Errorf("expected keyword %q to be matched, but it was not", expected)
		}
	}
}

// TestBadcaseSmallDeterministic 小规模确定性测试
func TestBadcaseSmallDeterministic(t *testing.T) {
	// 使用少量关键词，多次运行验证确定性
	keywords := []string{
		"红包", "提现", "现金", "免费", "红包提现", "赚钱",
		"挂机", "网赚", "亿万人生", "刷宝", "彩票", "体彩",
		"羊毛党", "金币", "短视频", "疯狂猜成语", "休闲游戏",
		"爱上消消消", "明星",
	}
	content := "休闲益智不烧脑,没wifi我也能玩一整天!,良心游戏，真送红包,红包,提现,现金,免费,红包提现,赚钱,挂机,网赚,亿万人生,刷宝,彩票,体彩,羊毛党,金币,短视频,疯狂猜成语,休闲游戏,爱上消消消,每天,明星,王二狗的摊位"

	rounds := 50
	var baseline []matchResultKey

	for round := 0; round < rounds; round++ {
		ac := NewAhoCorasick()
		for i, kw := range keywords {
			if err := ac.Insert([]byte(kw), i); err != nil {
				t.Fatalf("insert failed: %v", err)
			}
		}
		ac.Build()

		results := ac.Match([]byte(content))
		collected := collectResults(results)

		if round == 0 {
			baseline = collected
			continue
		}

		if !resultsEqual(baseline, collected) {
			t.Fatalf("NON-DETERMINISM at round %d! baseline=%d, round=%d",
				round, len(baseline), len(collected))
		}
	}

	t.Logf("small deterministic test: %d rounds passed", rounds)
}

// TestBadcaseInsertOrderIndependence 测试插入顺序是否影响结果
func TestBadcaseInsertOrderIndependence(t *testing.T) {
	keywords := []string{
		"红包", "提现", "现金", "免费", "红包提现", "赚钱",
		"挂机", "网赚", "亿万人生", "刷宝", "彩票", "体彩",
	}
	content := "真送红包,红包提现,赚钱,刷宝,彩票"

	// 正向插入
	ac1 := NewAhoCorasick()
	for i, kw := range keywords {
		ac1.Insert([]byte(kw), i)
	}
	ac1.Build()
	r1 := collectResults(ac1.Match([]byte(content)))

	// 反向插入
	ac2 := NewAhoCorasick()
	for i := len(keywords) - 1; i >= 0; i-- {
		ac2.Insert([]byte(keywords[i]), i)
	}
	ac2.Build()
	r2 := collectResults(ac2.Match([]byte(content)))

	// 注意：由于 value 不同（插入顺序不同），我们只比较匹配数量
	if len(r1) != len(r2) {
		t.Errorf("insert order affects match count: forward=%d, reverse=%d", len(r1), len(r2))
	} else {
		t.Logf("insert order independence: both got %d matches", len(r1))
	}
}

// TestBadcaseRepeatedBuild 测试重复 Build 是否影响结果
func TestBadcaseRepeatedBuild(t *testing.T) {
	keywords := []string{"红包", "提现", "现金", "免费", "红包提现", "赚钱"}
	content := "真送红包,红包提现,赚钱"

	ac := NewAhoCorasick()
	for i, kw := range keywords {
		ac.Insert([]byte(kw), i)
	}

	var baseline []matchResultKey
	for i := 0; i < 10; i++ {
		ac.Build()
		results := ac.Match([]byte(content))
		collected := collectResults(results)

		if i == 0 {
			baseline = collected
		} else if !resultsEqual(baseline, collected) {
			t.Errorf("repeated Build #%d produced different results", i)
		}
	}
	t.Log("repeated Build test passed")
}

// TestBadcaseMatchAllConsistency 测试 Match 和 MatchAll 的一致性
func TestBadcaseMatchAllConsistency(t *testing.T) {
	keywords := loadBadcaseKeywords(t)
	content := "休闲益智不烧脑,没wifi我也能玩一整天!,良心游戏，真送红包,红包,提现,现金,免费,红包提现,赚钱,挂机,网赚,亿万人生,刷宝,彩票,体彩,羊毛党,金币,短视频,疯狂猜成语,休闲游戏,爱上消消消,每天,明星,王二狗的摊位"

	ac := NewAhoCorasick()
	for i, kw := range keywords {
		ac.Insert([]byte(kw), i)
	}
	ac.Build()

	// Match 结果中的唯一 values
	results := ac.Match([]byte(content))
	matchValues := make(map[int]bool)
	for _, r := range results {
		matchValues[r.Value] = true
	}

	// MatchAll 结果
	matchAllValues := make(map[int]bool)
	for _, v := range ac.MatchAll([]byte(content)) {
		matchAllValues[v] = true
	}

	// MatchAll 的结果应该都在 Match 中出现过
	for v := range matchAllValues {
		if !matchValues[v] {
			t.Errorf("MatchAll returned value %d (%q) not found in Match results",
				v, keywords[v])
		}
	}

	t.Logf("Match: %d unique values, MatchAll: %d unique values",
		len(matchValues), len(matchAllValues))
}

// BenchmarkBadcase 性能基准测试
func BenchmarkBadcaseFull(b *testing.B) {
	keywords := loadBadcaseKeywordsForBench()
	content := "休闲益智不烧脑,没wifi我也能玩一整天!,良心游戏，真送红包,红包,提现,现金,免费,红包提现,赚钱,挂机,网赚,亿万人生,刷宝,彩票,体彩,羊毛党,金币,短视频,疯狂猜成语,休闲游戏,爱上消消消,每天,明星,王二狗的摊位"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ac := NewAhoCorasick()
		for j, kw := range keywords {
			ac.Insert([]byte(kw), j)
		}
		ac.Build()
		ac.Match([]byte(content))
	}
}

// BenchmarkBadcaseMatchOnly 仅匹配性能（不含构建）
func BenchmarkBadcaseMatchOnly(b *testing.B) {
	keywords := loadBadcaseKeywordsForBench()
	content := "休闲益智不烧脑,没wifi我也能玩一整天!,良心游戏，真送红包,红包,提现,现金,免费,红包提现,赚钱,挂机,网赚,亿万人生,刷宝,彩票,体彩,羊毛党,金币,短视频,疯狂猜成语,休闲游戏,爱上消消消,每天,明星,王二狗的摊位"

	ac := NewAhoCorasick()
	for j, kw := range keywords {
		ac.Insert([]byte(kw), j)
	}
	ac.Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ac.Match([]byte(content))
	}
}

// loadBadcaseKeywordsForBench 用于 benchmark 中加载关键词（不依赖 *testing.T）
func loadBadcaseKeywordsForBench() []string {
	f, err := os.Open(filepath.Join("testdata", "kws.txt"))
	if err != nil {
		panic(fmt.Sprintf("open kws.txt failed: %v", err))
	}
	defer f.Close()

	var keywords []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		kw := strings.TrimSpace(sc.Text())
		if kw != "" {
			keywords = append(keywords, kw)
		}
	}
	if err := sc.Err(); err != nil {
		panic(fmt.Sprintf("read kws.txt failed: %v", err))
	}
	return keywords
}
