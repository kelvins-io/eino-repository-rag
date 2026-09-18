package service

import (
	"testing"

	"github.com/kelvins-io/eino-repository-rag/internal/model"
)

func TestMergeFeedbackKeepsUntouchedField(t *testing.T) {
	cur := model.MessageFeedback{Vote: model.FeedbackVoteUp, Score: 4}
	vote := model.FeedbackVoteDown
	got, err := mergeFeedback(cur, &vote, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Vote != model.FeedbackVoteDown || got.Score != 4 {
		t.Fatalf("%+v", got)
	}

	score := 0
	got, err = mergeFeedback(got, nil, &score)
	if err != nil {
		t.Fatal(err)
	}
	if got.Vote != model.FeedbackVoteDown || got.Score != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestMergeFeedbackRejectsBadValues(t *testing.T) {
	if _, err := mergeFeedback(model.MessageFeedback{}, nil, nil); err == nil {
		t.Fatal("expected missing field error")
	}
	badVote := "love"
	if _, err := mergeFeedback(model.MessageFeedback{}, &badVote, nil); err == nil {
		t.Fatal("expected bad vote")
	}
	badScore := 6
	if _, err := mergeFeedback(model.MessageFeedback{}, nil, &badScore); err == nil {
		t.Fatal("expected bad score")
	}
}

func TestNormalizeVoteClears(t *testing.T) {
	got, err := normalizeVote("  ")
	if err != nil || got != "" {
		t.Fatalf("vote=%q err=%v", got, err)
	}
}
