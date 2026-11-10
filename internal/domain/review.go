package domain

import "time"

// DiffSide is the side of a diff a Review Thread is anchored to: LEFT is the
// base, RIGHT the head.
type DiffSide string

const (
	Left  DiffSide = "LEFT"
	Right DiffSide = "RIGHT"
)

// Reaction is one of GitHub's eight fixed emoji responses.
type Reaction string

const (
	ThumbsUp   Reaction = "THUMBS_UP"
	ThumbsDown Reaction = "THUMBS_DOWN"
	Laugh      Reaction = "LAUGH"
	Hooray     Reaction = "HOORAY"
	Confused   Reaction = "CONFUSED"
	Heart      Reaction = "HEART"
	Rocket     Reaction = "ROCKET"
	Eyes       Reaction = "EYES"
)

// Reactions lists every Reaction in GitHub's display order.
var Reactions = []Reaction{ThumbsUp, ThumbsDown, Laugh, Hooray, Confused, Heart, Rocket, Eyes}

var reactionEmoji = map[Reaction]string{
	ThumbsUp: "👍", ThumbsDown: "👎", Laugh: "😄", Hooray: "🎉",
	Confused: "😕", Heart: "❤️", Rocket: "🚀", Eyes: "👀",
}

// Emoji returns the Reaction's emoji, or "" if it is not one of the eight.
func (r Reaction) Emoji() string { return reactionEmoji[r] }

// Valid reports whether r is one of the eight Reactions.
func (r Reaction) Valid() bool { return reactionEmoji[r] != "" }

// ReactionGroup is the count of one Reaction on a comment or PR.
type ReactionGroup struct {
	Content          Reaction
	Count            int
	ViewerHasReacted bool
}

// Comment is a Review Comment or a Conversation Comment.
type Comment struct {
	ID              string // node ID
	Author          string
	Body            string // Markdown source
	BodyHTML        string // rendered and sanitised by the code host
	CreatedAt       time.Time
	Pending         bool // part of the viewer's Pending Review
	ViewerDidAuthor bool
	Reactions       []ReactionGroup
}

// ReviewThread is a line-anchored conversation on a PR's diff. Line and
// StartLine are line numbers in the file on Side at the PR's current head;
// Line is zero when the thread is outdated.
type ReviewThread struct {
	ID                 string
	Path               string
	Line, StartLine    int
	Side, StartSide    DiffSide
	Resolved           bool
	Outdated           bool
	ViewerCanResolve   bool
	ViewerCanUnresolve bool
	Comments           []Comment
}

// PendingReview is the viewer's not-yet-submitted review.
type PendingReview struct {
	ID       string
	Commit   SHA // the commit its comments are anchored to
	Comments int
}

// ViewedState is whether the viewer has marked a file Viewed.
type ViewedState string

const (
	Viewed    ViewedState = "VIEWED"
	Unviewed  ViewedState = "UNVIEWED"
	Dismissed ViewedState = "DISMISSED" // was Viewed, then the file changed
)

// ReviewEvent is how a review is submitted.
type ReviewEvent string

const (
	Approve        ReviewEvent = "APPROVE"
	RequestChanges ReviewEvent = "REQUEST_CHANGES"
	CommentOnly    ReviewEvent = "COMMENT"
)

// StackPR is one PR in a Stack.
type StackPR struct {
	Number   int
	Title    string
	State    PRState
	Position int // 1 is closest to the base branch
}

// Stack is GitHub's native ordered chain of PRs, seen from one of them.
type Stack struct {
	Position, Size int
	Entries        []StackPR // by Position
}

// Parent is the PR directly below this one, or nil at the bottom.
func (s *Stack) Parent() *StackPR { return s.at(s.Position - 1) }

// Child is the PR directly above this one, or nil at the top.
func (s *Stack) Child() *StackPR { return s.at(s.Position + 1) }

func (s *Stack) at(position int) *StackPR {
	for i := range s.Entries {
		if s.Entries[i].Position == position {
			return &s.Entries[i]
		}
	}
	return nil
}

// ReviewState is the review activity on a PR, as seen by the viewer.
type ReviewState struct {
	PRID         string // the PR's node ID
	Viewer       string
	Reactions    []ReactionGroup // on the PR itself
	Threads      []ReviewThread
	Conversation []Comment // Conversation Comments, oldest first
	Pending      *PendingReview
	Viewed       map[string]ViewedState // by path
	// LastReviewed is the head commit when the viewer last submitted a
	// review, or empty if they have not.
	LastReviewed SHA
	Stack        *Stack // nil when the PR is not in a Stack
}
