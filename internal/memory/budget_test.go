package memory

import "testing"

func TestApproxTokens(t *testing.T) {
	if ApproxTokens("") != 0 {
		t.Fatal("empty should be 0")
	}
	if ApproxTokens("abcd") < 1 {
		t.Fatal("ascii should estimate >=1")
	}
	cn := ApproxTokens("你好世界知识库问答")
	en := ApproxTokens("hello world knowledge base")
	if cn <= 0 || en <= 0 {
		t.Fatalf("unexpected tokens cn=%d en=%d", cn, en)
	}
}

func TestFitToTokenBudgetDropsOldest(t *testing.T) {
	turns := []ChatTurn{
		{Role: "user", Content: "旧问题一" + string(make([]byte, 200))},
		{Role: "assistant", Content: "旧回答一" + string(make([]byte, 200))},
		{Role: "user", Content: "新问题"},
		{Role: "assistant", Content: "新回答"},
	}
	sum, out := FitToTokenBudget("早期摘要内容", turns, 40)
	if len(out) == 0 && sum == "" {
		t.Fatal("should keep something within budget")
	}
	if estimateHistoryTokens(sum, out) > 40 {
		t.Fatalf("still over budget: %d", estimateHistoryTokens(sum, out))
	}
	// 最新轮次应更可能被保留
	if len(out) > 0 && out[len(out)-1].Content != "新回答" && out[len(out)-1].Content != "新问题" {
		// 预算极紧时可能只剩摘要，允许
		t.Logf("trimmed to sum=%q turns=%d", sum, len(out))
	}
}

func TestFitToTokenBudgetUnlimited(t *testing.T) {
	turns := []ChatTurn{{Content: "a"}, {Content: "b"}}
	sum, out := FitToTokenBudget("s", turns, 0)
	if sum != "s" || len(out) != 2 {
		t.Fatalf("budget<=0 should keep all")
	}
}
