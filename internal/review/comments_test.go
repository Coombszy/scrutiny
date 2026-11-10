package review_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/review"
)

// withOwnComments gives the PR a thread holding someone else's submitted
// comment and the viewer's pending reply, and a Conversation Comment of
// the viewer's.
func withOwnComments(f *fixture) {
	st := f.host.State(ref)
	st.Pending = &domain.PendingReview{ID: "PRR1", Commit: c3, Comments: 1}
	st.Threads = []domain.ReviewThread{{ID: "T1", Path: "a.go", Line: 2, Side: domain.Right, Comments: []domain.Comment{
		{ID: "C1", Author: "pat", Body: "Why?"},
		{ID: "C2", Author: "me", Body: "Because", Pending: true, ViewerCanUpdate: true, ViewerCanDelete: true},
	}}}
	st.Conversation = []domain.Comment{{ID: "IC1", Author: "me", Body: "Thanks", ViewerCanUpdate: true, ViewerCanDelete: true}}
}

func TestEditingAReviewCommentReturnsItsThread(t *testing.T) {
	f := newFixture(t)
	withOwnComments(f)
	ch, err := f.svc.EditComment(context.Background(), ref, "C2", "Because it is new")
	if err != nil {
		t.Fatal(err)
	}
	if ch.InConversation || ch.Thread == nil || ch.Thread.Comments[1].Body != "Because it is new" {
		t.Errorf("change = %+v", ch)
	}
}

func TestEditingAConversationCommentReturnsTheConversation(t *testing.T) {
	f := newFixture(t)
	withOwnComments(f)
	ch, err := f.svc.EditComment(context.Background(), ref, "IC1", "Thanks all")
	if err != nil {
		t.Fatal(err)
	}
	if !ch.InConversation || len(ch.Conversation) != 1 || ch.Conversation[0].Body != "Thanks all" {
		t.Errorf("change = %+v", ch)
	}
}

func TestDeletingAPendingReplyUpdatesThePendingReview(t *testing.T) {
	f := newFixture(t)
	withOwnComments(f)
	ctx := context.Background()
	ch, err := f.svc.DeleteComment(ctx, ref, "C2")
	if err != nil {
		t.Fatal(err)
	}
	if ch.Thread == nil || len(ch.Thread.Comments) != 1 {
		t.Errorf("change = %+v", ch)
	}
	if a, _ := f.svc.Anchors(ctx, ref); a.Pending == nil || a.Pending.Comments != 0 {
		t.Errorf("Pending = %+v, want 0 comments", a.Pending)
	}
}

func TestDeletingAThreadsLastCommentDropsTheThread(t *testing.T) {
	f := newFixture(t)
	withOwnComments(f)
	f.host.State(ref).Threads[0].Comments = f.host.State(ref).Threads[0].Comments[1:]
	ch, err := f.svc.DeleteComment(context.Background(), ref, "C2")
	if err != nil {
		t.Fatal(err)
	}
	if ch.InConversation || ch.Thread != nil || ch.ThreadID != "T1" {
		t.Errorf("change = %+v, want the thread gone", ch)
	}
}

func TestDeletingAConversationComment(t *testing.T) {
	f := newFixture(t)
	withOwnComments(f)
	ch, err := f.svc.DeleteComment(context.Background(), ref, "IC1")
	if err != nil || !ch.InConversation || len(ch.Conversation) != 0 {
		t.Errorf("change = %+v, err %v", ch, err)
	}
}

func TestCommentsTheViewerCannotChangeAreRefused(t *testing.T) {
	f := newFixture(t)
	withOwnComments(f)
	ctx := context.Background()
	if _, err := f.svc.EditComment(ctx, ref, "C1", "Hijacked"); !errors.Is(err, review.ErrCannotChangeComment) {
		t.Errorf("edit someone else's: %v", err)
	}
	if _, err := f.svc.DeleteComment(ctx, ref, "C1"); !errors.Is(err, review.ErrCannotChangeComment) {
		t.Errorf("delete someone else's: %v", err)
	}
	if _, err := f.svc.EditComment(ctx, ref, "C2", "  "); !errors.Is(err, review.ErrCannotChangeComment) {
		t.Errorf("empty edit: %v", err)
	}
	if _, err := f.svc.DeleteComment(ctx, ref, "nope"); !errors.Is(err, review.ErrThreadNotFound) {
		t.Errorf("unknown comment: %v", err)
	}
	if got := f.host.State(ref).Threads[0].Comments[0].Body; got != "Why?" {
		t.Errorf("someone else's comment became %q", got)
	}
}
