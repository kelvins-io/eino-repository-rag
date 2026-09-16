package memory

import "unicode"

// ApproxTokens 估算混合中英文文本的 token 数（无需外部分词器）。
// ASCII 约 4 字符/token，CJK 约 1.5 字符/token。
func ApproxTokens(s string) int {
	if s == "" {
		return 0
	}
	ascii, other := 0, 0
	for _, r := range s {
		if r <= unicode.MaxASCII {
			ascii++
		} else {
			other++
		}
	}
	tokens := ascii/4 + (other*2+2)/3
	if tokens < 1 {
		return 1
	}
	return tokens
}

func estimateHistoryTokens(summary string, turns []ChatTurn) int {
	n := 0
	if summary != "" {
		n += ApproxTokens(summary) + 8 // 「历史对话摘要」开销
	}
	for _, t := range turns {
		n += ApproxTokens(t.Content) + 4 // role 开销
	}
	return n
}

// FitToTokenBudget 将摘要 + 原文历史裁剪到 token 预算内：优先丢弃最旧原文，必要时截断摘要。
// budget <= 0 表示不限制。
func FitToTokenBudget(summary string, turns []ChatTurn, budget int) (string, []ChatTurn) {
	if budget <= 0 {
		return summary, turns
	}

	out := append([]ChatTurn(nil), turns...)
	sum := summary

	for estimateHistoryTokens(sum, out) > budget && len(out) > 0 {
		out = out[1:]
	}

	for estimateHistoryTokens(sum, out) > budget && sum != "" {
		r := []rune(sum)
		if len(r) <= 32 {
			sum = ""
			break
		}
		sum = string(r[:len(r)*3/4]) + "…"
	}

	return sum, out
}
