package review

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Coombszy/scrutiny/internal/cache"
	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
)

var (
	// ErrCannotComment is returned for a new Review Thread that GitHub would
	// anchor to different lines than the ones shown.
	ErrCannotComment = errors.New("cannot comment here")
	// ErrInvalidReaction is returned for a Reaction outside GitHub's eight.
	ErrInvalidReaction = errors.New("invalid reaction")
	// ErrInvalidEvent is returned for an unknown review submission event.
	ErrInvalidEvent = errors.New("invalid review event")
	// ErrCannotChangeComment is returned for an edit or delete the code host
	// does not allow the viewer, or an empty edit.
	ErrCannotChangeComment = errors.New("cannot change this comment")
	// ErrThreadNotFound is returned when a thread or comment is not on the PR.
	ErrThreadNotFound = errors.New("not found on this pull request")
)

// snapshot is the review state last read for a PR, with the commits its
// thread line numbers refer to. It is cached so that file fragments can
// place threads without calling the code host.
type snapshot struct {
	State     domain.ReviewState
	Head      domain.SHA // the PR's head when State was read
	MergeBase domain.SHA
	Checks    domain.Checks
	Offline   bool // the code host was unreachable at the last poll
}

func snapshotKey(ref domain.PRRef) string { return "review:" + ref.String() }

// snapshot returns the cached snapshot for ref, reading it fresh if absent.
func (s *Service) snapshot(ctx context.Context, ref domain.PRRef) (snapshot, error) {
	if snap, ok, err := s.cachedSnapshot(ctx, ref); err != nil || ok {
		return snap, err
	}
	pr, err := s.CodeHost.PullRequest(ctx, ref)
	if err != nil {
		return snapshot{}, err
	}
	mb, err := s.mergeBase(ctx, pr)
	if err != nil {
		return snapshot{}, err
	}
	return s.readState(ctx, ref, pr.HeadSHA, mb)
}

// readState reads the review state from the code host and caches it,
// keeping the cached Checks.
func (s *Service) readState(ctx context.Context, ref domain.PRRef, head, mergeBase domain.SHA) (snapshot, error) {
	st, err := s.CodeHost.ReviewState(ctx, ref)
	if err != nil {
		return snapshot{}, err
	}
	snap := snapshot{State: st, Head: head, MergeBase: mergeBase}
	if prev, ok, err := s.cachedSnapshot(ctx, ref); err == nil && ok {
		snap.Checks = prev.Checks
	}
	return snap, s.saveSnapshot(ctx, ref, snap)
}

func (s *Service) cachedSnapshot(ctx context.Context, ref domain.PRRef) (snapshot, bool, error) {
	e, ok, err := s.Cache.Get(ctx, snapshotKey(ref))
	if err != nil || !ok {
		return snapshot{}, false, err
	}
	var snap snapshot
	if err := json.Unmarshal(e.Value, &snap); err != nil {
		return snapshot{}, false, nil
	}
	return snap, true, nil
}

func (s *Service) saveSnapshot(ctx context.Context, ref domain.PRRef, snap snapshot) error {
	b, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return s.Cache.Put(ctx, snapshotKey(ref), cache.Entry{Value: b, FetchedAt: s.Now()})
}

// write runs a code host write against the current snapshot, then re-reads
// the review state so the code host stays the source of truth. The state is
// re-read even when the write fails, since part of it may have happened
// (e.g. a Pending Review created before its first thread was refused).
func (s *Service) write(ctx context.Context, ref domain.PRRef, fn func(snapshot) error) (snapshot, error) {
	snap, err := s.snapshot(ctx, ref)
	if err != nil {
		return snapshot{}, err
	}
	if err := fn(snap); err != nil {
		_, _ = s.readState(ctx, ref, snap.Head, snap.MergeBase)
		return snapshot{}, err
	}
	return s.readState(ctx, ref, snap.Head, snap.MergeBase)
}

// AddThread starts a Review Thread in the viewer's Pending Review, creating
// the review at head if there is none. GitHub anchors a thread to its
// review's commit and LEFT lines to the PR's merge base, so the thread is
// refused when either would differ from the Comparison shown.
// It returns the thread it added. Threads may only go on files in the
// Comparison.
func (s *Service) AddThread(ctx context.Context, ref domain.PRRef, base, head domain.SHA, t codehost.NewThread) ([]domain.ReviewThread, error) {
	changes, err := s.Repos.Changes(ctx, ref.Repo, base, head)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(changes, func(c domain.FileChange) bool { return c.Path == t.Path }) {
		return nil, fmt.Errorf("%w: %s is not changed in this Comparison", ErrCannotComment, t.Path)
	}
	known := map[string]bool{}
	snap, err := s.write(ctx, ref, func(snap snapshot) error {
		for _, th := range snap.State.Threads {
			known[th.ID] = true
		}
		if (t.Side == domain.Left || t.StartSide == domain.Left) && base != snap.MergeBase {
			return fmt.Errorf("%w: the left side of this Comparison is not the pull request's base", ErrCannotComment)
		}
		reviewID := ""
		switch p := snap.State.Pending; {
		case p == nil:
			id, err := s.CodeHost.StartReview(ctx, snap.State.PRID, head)
			if err != nil {
				return err
			}
			reviewID = id
		case p.Commit != head:
			return fmt.Errorf("%w: your Pending Review is on commit %s; view a Comparison ending there, or submit it first",
				ErrCannotComment, p.Commit.Short())
		default:
			reviewID = p.ID
		}
		return s.CodeHost.AddThread(ctx, reviewID, t)
	})
	if err != nil {
		return nil, err
	}
	var added []domain.ReviewThread
	for _, th := range snap.State.Threads {
		if !known[th.ID] && th.Path == t.Path {
			added = append(added, th)
		}
	}
	return added, nil
}

// Anchors are the commits Review Threads are anchored to, as last read.
type Anchors struct {
	Head, MergeBase domain.SHA
	Pending         *domain.PendingReview
}

// Anchors returns the PR's Anchors from the last read of its review state.
func (s *Service) Anchors(ctx context.Context, ref domain.PRRef) (Anchors, error) {
	snap, err := s.snapshot(ctx, ref)
	return Anchors{Head: snap.Head, MergeBase: snap.MergeBase, Pending: snap.State.Pending}, err
}

// Reply adds a Review Comment to a thread within the viewer's Pending
// Review, starting one at the PR's head if needed.
func (s *Service) Reply(ctx context.Context, ref domain.PRRef, threadID, body string) (domain.ReviewThread, error) {
	snap, err := s.write(ctx, ref, func(snap snapshot) error {
		if _, err := findThread(snap.State, threadID); err != nil {
			return err
		}
		reviewID := ""
		if p := snap.State.Pending; p != nil {
			reviewID = p.ID
		} else {
			id, err := s.CodeHost.StartReview(ctx, snap.State.PRID, snap.Head)
			if err != nil {
				return err
			}
			reviewID = id
		}
		return s.CodeHost.Reply(ctx, reviewID, threadID, body)
	})
	if err != nil {
		return domain.ReviewThread{}, err
	}
	return findThread(snap.State, threadID)
}

// SetResolved resolves or unresolves a thread.
func (s *Service) SetResolved(ctx context.Context, ref domain.PRRef, threadID string, resolved bool) (domain.ReviewThread, error) {
	snap, err := s.write(ctx, ref, func(snap snapshot) error {
		if _, err := findThread(snap.State, threadID); err != nil {
			return err
		}
		return s.CodeHost.SetThreadResolved(ctx, threadID, resolved)
	})
	if err != nil {
		return domain.ReviewThread{}, err
	}
	return findThread(snap.State, threadID)
}

// SetReaction adds or removes the viewer's Reaction on a comment or on the
// PR (subjectID is then the PR's node ID), returning the subject's Reactions.
func (s *Service) SetReaction(ctx context.Context, ref domain.PRRef, subjectID string, r domain.Reaction, on bool) ([]domain.ReactionGroup, error) {
	if !r.Valid() {
		return nil, fmt.Errorf("%w: %q", ErrInvalidReaction, r)
	}
	snap, err := s.write(ctx, ref, func(snap snapshot) error {
		if subjectID != snap.State.PRID && !slices.ContainsFunc(allComments(snap.State), func(c domain.Comment) bool { return c.ID == subjectID }) {
			return fmt.Errorf("comment %s: %w", subjectID, ErrThreadNotFound)
		}
		return s.CodeHost.SetReaction(ctx, subjectID, r, on)
	})
	if err != nil {
		return nil, err
	}
	if subjectID == snap.State.PRID {
		return snap.State.Reactions, nil
	}
	for _, c := range allComments(snap.State) {
		if c.ID == subjectID {
			return c.Reactions, nil
		}
	}
	return nil, fmt.Errorf("comment %s: %w", subjectID, ErrThreadNotFound)
}

// AddConversationComment posts a Conversation Comment, returning all of
// them and the submitted reviews shown among them.
func (s *Service) AddConversationComment(ctx context.Context, ref domain.PRRef, body string) ([]domain.Comment, []domain.SubmittedReview, error) {
	snap, err := s.write(ctx, ref, func(snap snapshot) error {
		return s.CodeHost.AddConversationComment(ctx, snap.State.PRID, body)
	})
	return snap.State.Conversation, snap.State.Reviews, err
}

// SetViewed marks or unmarks a file as Viewed.
func (s *Service) SetViewed(ctx context.Context, ref domain.PRRef, path string, viewed bool) error {
	_, err := s.write(ctx, ref, func(snap snapshot) error {
		return s.CodeHost.SetViewed(ctx, snap.State.PRID, path, viewed)
	})
	return err
}

// Submit submits the viewer's Pending Review with event, or when there is
// none, creates and submits a review at head.
func (s *Service) Submit(ctx context.Context, ref domain.PRRef, head domain.SHA, event domain.ReviewEvent, body string) error {
	switch event {
	case domain.Approve, domain.RequestChanges, domain.CommentOnly:
	default:
		return fmt.Errorf("%w: %q", ErrInvalidEvent, event)
	}
	_, err := s.write(ctx, ref, func(snap snapshot) error {
		if p := snap.State.Pending; p != nil {
			return s.CodeHost.SubmitReview(ctx, snap.State.PRID, p.ID, "", event, body)
		}
		return s.CodeHost.SubmitReview(ctx, snap.State.PRID, "", head, event, body)
	})
	return err
}

func findThread(st domain.ReviewState, id string) (domain.ReviewThread, error) {
	for _, t := range st.Threads {
		if t.ID == id {
			return t, nil
		}
	}
	return domain.ReviewThread{}, fmt.Errorf("thread %s: %w", id, ErrThreadNotFound)
}

func allComments(st domain.ReviewState) []domain.Comment {
	cs := append([]domain.Comment(nil), st.Conversation...)
	for _, t := range st.Threads {
		cs = append(cs, t.Comments...)
	}
	return cs
}

// PendingComment is a comment in the viewer's Pending Review: a new
// thread's first comment, or a reply to an existing thread.
type PendingComment struct {
	ThreadID string
	Path     string
	Line     int // zero for outdated threads
	Body     string
	Reply    bool
}

// PendingComments lists the viewer's Pending Review comments, by file and
// line, as last read.
func (s *Service) PendingComments(ctx context.Context, ref domain.PRRef) ([]PendingComment, error) {
	snap, err := s.snapshot(ctx, ref)
	if err != nil {
		return nil, err
	}
	return pendingComments(snap.State), nil
}

func pendingComments(st domain.ReviewState) []PendingComment {
	var out []PendingComment
	for _, t := range st.Threads {
		for i, c := range t.Comments {
			if c.Pending {
				out = append(out, PendingComment{ThreadID: t.ID, Path: t.Path, Line: t.Line, Body: c.Body, Reply: i > 0})
			}
		}
	}
	slices.SortStableFunc(out, func(a, b PendingComment) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line))
	})
	return out
}

// CommentChange is what an edited or deleted comment belongs to, re-read
// afterwards: its thread, or the Conversation.
type CommentChange struct {
	InConversation bool
	Conversation   []domain.Comment
	Reviews        []domain.SubmittedReview // shown in the Conversation too
	ThreadID       string
	// Thread is nil when deleting its last comment deleted the thread.
	Thread *domain.ReviewThread
}

// EditComment replaces the body of a Review Comment (pending or submitted)
// or a Conversation Comment.
func (s *Service) EditComment(ctx context.Context, ref domain.PRRef, commentID, body string) (CommentChange, error) {
	if strings.TrimSpace(body) == "" {
		return CommentChange{}, fmt.Errorf("%w: a comment cannot be empty", ErrCannotChangeComment)
	}
	return s.changeComment(ctx, ref, commentID, func(c domain.Comment, inConversation bool) error {
		switch {
		case !c.ViewerCanUpdate:
			return fmt.Errorf("%w: GitHub does not let you edit it", ErrCannotChangeComment)
		case inConversation:
			return s.CodeHost.EditConversationComment(ctx, commentID, body)
		default:
			return s.CodeHost.EditReviewComment(ctx, commentID, body)
		}
	})
}

// DeleteComment deletes a Review Comment (pending or submitted) or a
// Conversation Comment.
func (s *Service) DeleteComment(ctx context.Context, ref domain.PRRef, commentID string) (CommentChange, error) {
	return s.changeComment(ctx, ref, commentID, func(c domain.Comment, inConversation bool) error {
		switch {
		case !c.ViewerCanDelete:
			return fmt.Errorf("%w: GitHub does not let you delete it", ErrCannotChangeComment)
		case inConversation:
			return s.CodeHost.DeleteConversationComment(ctx, commentID)
		default:
			return s.CodeHost.DeleteReviewComment(ctx, commentID)
		}
	})
}

// changeComment finds the comment and runs change on it, returning what it
// belongs to as re-read afterwards.
func (s *Service) changeComment(ctx context.Context, ref domain.PRRef, commentID string, change func(c domain.Comment, inConversation bool) error) (CommentChange, error) {
	var out CommentChange
	snap, err := s.write(ctx, ref, func(snap snapshot) error {
		for _, c := range snap.State.Conversation {
			if c.ID == commentID {
				out.InConversation = true
				return change(c, true)
			}
		}
		for _, t := range snap.State.Threads {
			for _, c := range t.Comments {
				if c.ID == commentID {
					out.ThreadID = t.ID
					return change(c, false)
				}
			}
		}
		return fmt.Errorf("comment %s: %w", commentID, ErrThreadNotFound)
	})
	if err != nil {
		return CommentChange{}, err
	}
	if out.InConversation {
		out.Conversation, out.Reviews = snap.State.Conversation, snap.State.Reviews
	} else if t, err := findThread(snap.State, out.ThreadID); err == nil {
		out.Thread = &t
	}
	return out, nil
}
