package cedar

import (
	"fmt"
	"math/rand"
	"testing"
)

// 生成随机中文字符串
var zhChars = []byte("的一是不了人我在有他这之大会着自中小个上以说到心多家去出好看可学过你为没主发工里能如面同经然很知后行己月都做于生地什样想而开还头手当て己")

func randChinese(n int) string {
	buf := make([]byte, n*3)
	for i := 0; i < n; i++ {
		// 随机选取一个中文字符 (3 bytes each in the test set)
		idx := rand.Intn(len(zhChars) / 3)
		copy(buf[i*3:(i+1)*3], zhChars[idx*3:(idx+1)*3])
	}
	return string(buf)
}

func randEnglish(n int) string {
	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		buf[i] = byte('a' + rand.Intn(26))
	}
	return string(buf)
}

// -------- AC 自动机 Build benchmark --------

func BenchmarkAC_Build_Small(b *testing.B) {
	patterns := []string{"he", "she", "his", "hers", "him", "her", "hello", "world"}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		ac := NewAhoCorasick()
		for j, p := range patterns {
			ac.Insert([]byte(p), j)
		}
		b.StartTimer()
		ac.Build()
	}
}

func BenchmarkAC_Build_Medium(b *testing.B) {
	// 100 个英文模式
	patterns := make([]string, 100)
	for i := range patterns {
		patterns[i] = randEnglish(5 + rand.Intn(10))
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		ac := NewAhoCorasick()
		for j, p := range patterns {
			ac.Insert([]byte(p), j)
		}
		b.StartTimer()
		ac.Build()
	}
}

func BenchmarkAC_Build_Large(b *testing.B) {
	// 1000 个英文模式
	patterns := make([]string, 1000)
	for i := range patterns {
		patterns[i] = randEnglish(5 + rand.Intn(20))
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		ac := NewAhoCorasick()
		for j, p := range patterns {
			ac.Insert([]byte(p), j)
		}
		b.StartTimer()
		ac.Build()
	}
}

func BenchmarkAC_Build_Chinese(b *testing.B) {
	// 50 个中文模式
	patterns := make([]string, 50)
	for i := range patterns {
		patterns[i] = randChinese(2 + rand.Intn(4))
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		ac := NewAhoCorasick()
		for j, p := range patterns {
			ac.Insert([]byte(p), j)
		}
		b.StartTimer()
		ac.Build()
	}
}

// -------- AC 自动机 Match benchmark --------

func BenchmarkAC_Match_Small(b *testing.B) {
	ac := NewAhoCorasick()
	patterns := []string{"he", "she", "his", "hers"}
	for i, p := range patterns {
		ac.Insert([]byte(p), i)
	}
	ac.Build()

	text := []byte("ushers and his hers are in this sentence")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ac.Match(text)
	}
}

func BenchmarkAC_Match_Medium(b *testing.B) {
	// 100 个模式, 1KB 文本
	ac := NewAhoCorasick()
	patterns := make([]string, 100)
	for i := range patterns {
		patterns[i] = randEnglish(5 + rand.Intn(10))
	}
	for i, p := range patterns {
		ac.Insert([]byte(p), i)
	}
	ac.Build()

	textStr := randEnglish(1000)
	text := []byte(textStr)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ac.Match(text)
	}
}

func BenchmarkAC_Match_Large(b *testing.B) {
	// 1000 个模式, 100KB 文本
	ac := NewAhoCorasick()
	patterns := make([]string, 1000)
	for i := range patterns {
		patterns[i] = randEnglish(5 + rand.Intn(20))
	}
	for i, p := range patterns {
		ac.Insert([]byte(p), i)
	}
	ac.Build()

	textStr := randEnglish(100000)
	text := []byte(textStr)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ac.Match(text)
	}
}

func BenchmarkAC_Match_Chinese(b *testing.B) {
	// 50 个中文模式, 10KB 中文文本
	ac := NewAhoCorasick()
	patterns := make([]string, 50)
	for i := range patterns {
		patterns[i] = randChinese(2 + rand.Intn(4))
	}
	for i, p := range patterns {
		ac.Insert([]byte(p), i)
	}
	ac.Build()

	textStr := randChinese(3000) // ~9KB
	text := []byte(textStr)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ac.Match(text)
	}
}

func BenchmarkAC_Match_ShortText(b *testing.B) {
	ac := NewAhoCorasick()
	patterns := []string{"abc", "def", "ghi", "jkl", "mno"}
	for i, p := range patterns {
		ac.Insert([]byte(p), i)
	}
	ac.Build()

	text := []byte("abc")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ac.Match(text)
	}
}

func BenchmarkAC_Match_NoMatch(b *testing.B) {
	ac := NewAhoCorasick()
	patterns := []string{"xyz", "uvw", "rst"}
	for i, p := range patterns {
		ac.Insert([]byte(p), i)
	}
	ac.Build()

	text := []byte("aaaaaaaabbbbbbbbccccccccdddddddd")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ac.Match(text)
	}
}

func BenchmarkAC_Match_HighMatchRate(b *testing.B) {
	// 大量短模式，高命中率场景（类似敏感词过滤）
	ac := NewAhoCorasick()
	patterns := make([]string, 200)
	for i := range patterns {
		patterns[i] = randEnglish(2 + rand.Intn(5))
	}
	// 让文本中包含部分模式
	for i, j := 0, 0; i < 50; i++ {
		pat := patterns[j]
		ac.Insert([]byte(pat), j)
		j++
		if j >= len(patterns) {
			j = 0
		}
	}
	// 插入剩余模式
	for i := 50; i < len(patterns); i++ {
		ac.Insert([]byte(patterns[i]), i)
	}
	ac.Build()

	// 文本中混入模式词汇
	textParts := make([]byte, 0, 10000)
	for i := 0; i < 1000; i++ {
		if i%3 == 0 {
			textParts = append(textParts, []byte(patterns[i%len(patterns)])...)
		} else {
			textParts = append(textParts, byte('a'+rand.Intn(26)))
		}
	}
	text := textParts
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ac.Match(text)
	}
}

// -------- Benchmark 对比：不同模式集大小 --------

func BenchmarkAC_Match_PatternCount(b *testing.B) {
	// 测试不同模式数量下的 Match 性能
	for _, count := range []int{10, 100, 1000} {
		ac := NewAhoCorasick()
		patterns := make([]string, count)
		for i := range patterns {
			patterns[i] = randEnglish(5 + rand.Intn(10))
		}
		for i, p := range patterns {
			ac.Insert([]byte(p), i)
		}
		ac.Build()

		text := []byte(randEnglish(10000))
		b.Run(fmt.Sprintf("patterns=%d", count), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				ac.Match(text)
			}
		})
	}
}

func BenchmarkAC_Match_TextLength(b *testing.B) {
	// 测试不同文本长度下的 Match 性能
	ac := NewAhoCorasick()
	patterns := make([]string, 100)
	for i := range patterns {
		patterns[i] = randEnglish(5 + rand.Intn(10))
	}
	for i, p := range patterns {
		ac.Insert([]byte(p), i)
	}
	ac.Build()

	for _, length := range []int{100, 1000, 10000} {
		text := []byte(randEnglish(length))
		b.Run(fmt.Sprintf("textlen=%d", length), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				ac.Match(text)
			}
		})
	}
}

// -------- Benchmark: Insert + Build + Match 全流程 --------

func BenchmarkAC_FullPipeline_Small(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ac := NewAhoCorasick()
		patterns := []string{"he", "she", "his", "hers"}
		for j, p := range patterns {
			ac.Insert([]byte(p), j)
		}
		ac.Build()
		ac.Match([]byte("ushers"))
	}
}

func BenchmarkAC_FullPipeline_Medium(b *testing.B) {
	patterns := make([]string, 100)
	for i := range patterns {
		patterns[i] = randEnglish(5 + rand.Intn(10))
	}
	text := []byte(randEnglish(1000))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ac := NewAhoCorasick()
		for j, p := range patterns {
			ac.Insert([]byte(p), j)
		}
		ac.Build()
		ac.Match(text)
	}
}