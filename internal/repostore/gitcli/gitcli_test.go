package gitcli_test

import (
	"context"
	"testing"

	"github.com/Coombszy/scrutiny/internal/domain"
)

func TestFetchPRMakesHeadAndBaseAvailable(t *testing.T) {
	ctx := context.Background()
	r := newRemote(t)
	r.write("a.txt", "one\n")
	base := r.commit("base")
	r.git("checkout", "-q", "-b", "feature")
	r.write("a.txt", "one\ntwo\n")
	head := r.commit("feature")
	r.git("checkout", "-q", "main")
	r.openPR(1, head)

	store := storeFor(t, r)
	if _, err := store.FetchPR(ctx, domain.PRRef{Repo: testRepo, Number: 1}, "main"); err != nil {
		t.Fatal(err)
	}

	got, err := store.MergeBase(ctx, testRepo, base, head)
	if err != nil {
		t.Fatal(err)
	}
	if got != base {
		t.Errorf("MergeBase = %s, want %s", got, base)
	}
}

func TestFetchPRAgainPicksUpNewHead(t *testing.T) {
	ctx := context.Background()
	r := newRemote(t)
	r.write("a.txt", "one\n")
	first := r.commit("first")
	r.openPR(1, first)
	store := storeFor(t, r)
	pr := domain.PRRef{Repo: testRepo, Number: 1}
	if _, err := store.FetchPR(ctx, pr, "main"); err != nil {
		t.Fatal(err)
	}

	r.write("a.txt", "two\n")
	second := r.commit("second")
	r.openPR(1, second)
	if _, err := store.FetchPR(ctx, pr, "main"); err != nil {
		t.Fatal(err)
	}

	got, err := store.Blob(ctx, testRepo, second, "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "two\n" {
		t.Errorf("Blob = %q, want %q", got, "two\n")
	}
}

func TestChangesSummarisesEachFile(t *testing.T) {
	ctx := context.Background()
	r := newRemote(t)
	r.write("keep.txt", "a\nb\nc\n")
	r.write("gone.txt", "bye\n")
	r.write("old/name.txt", "same content\nacross\nthe\nrename\n")
	r.write("img.bin", "\x00\x01\x02")
	base := r.commit("base")
	r.write("keep.txt", "a\nB\nc\nd\n")
	r.git("rm", "-q", "gone.txt")
	r.git("mv", "old/name.txt", "new name.txt")
	r.write("img.bin", "\x00\x01\x03")
	r.write("fresh.txt", "hi\n")
	head := r.commit("head")
	r.openPR(1, head)
	store := storeFor(t, r)
	if _, err := store.FetchPR(ctx, domain.PRRef{Repo: testRepo, Number: 1}, "main"); err != nil {
		t.Fatal(err)
	}

	got, err := store.Changes(ctx, testRepo, base, head)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.FileChange{
		{Path: "fresh.txt", OldPath: "fresh.txt", Status: domain.Added, Additions: 1},
		{Path: "gone.txt", OldPath: "gone.txt", Status: domain.Deleted, Deletions: 1},
		{Path: "img.bin", OldPath: "img.bin", Status: domain.Modified, Binary: true},
		{Path: "keep.txt", OldPath: "keep.txt", Status: domain.Modified, Additions: 2, Deletions: 1},
		{Path: "new name.txt", OldPath: "old/name.txt", Status: domain.Renamed},
	}
	if len(got) != len(want) {
		t.Fatalf("Changes = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Changes[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestFileDiffNumbersEachLine(t *testing.T) {
	ctx := context.Background()
	r := newRemote(t)
	r.write("old.txt", "a\nb\nc\n")
	base := r.commit("base")
	r.git("mv", "old.txt", "new.txt")
	r.write("new.txt", "a\nB\nc\nd\n")
	head := r.commit("head")
	r.openPR(1, head)
	store := storeFor(t, r)
	if _, err := store.FetchPR(ctx, domain.PRRef{Repo: testRepo, Number: 1}, "main"); err != nil {
		t.Fatal(err)
	}
	change := domain.FileChange{Path: "new.txt", OldPath: "old.txt", Status: domain.Renamed}

	got, err := store.FileDiff(ctx, testRepo, base, head, change)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hunks) != 1 {
		t.Fatalf("got %d hunks, want 1: %+v", len(got.Hunks), got.Hunks)
	}
	h := got.Hunks[0]
	if h.OldStart != 1 || h.OldLines != 3 || h.NewStart != 1 || h.NewLines != 4 {
		t.Errorf("hunk range = -%d,%d +%d,%d, want -1,3 +1,4", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
	}
	want := []domain.Line{
		{Kind: domain.Context, OldNo: 1, NewNo: 1, Text: "a"},
		{Kind: domain.Delete, OldNo: 2, Text: "b"},
		{Kind: domain.Add, NewNo: 2, Text: "B"},
		{Kind: domain.Context, OldNo: 3, NewNo: 3, Text: "c"},
		{Kind: domain.Add, NewNo: 4, Text: "d"},
	}
	if len(h.Lines) != len(want) {
		t.Fatalf("lines = %+v, want %+v", h.Lines, want)
	}
	for i := range want {
		if h.Lines[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, h.Lines[i], want[i])
		}
	}
}

func TestListFilesAtSHA(t *testing.T) {
	ctx := context.Background()
	r := newRemote(t)
	r.write("a.go", "package a\n")
	r.write("dir/b.go", "package b\n")
	head := r.commit("head")
	r.openPR(1, head)
	store := storeFor(t, r)
	if _, err := store.FetchPR(ctx, domain.PRRef{Repo: testRepo, Number: 1}, "main"); err != nil {
		t.Fatal(err)
	}

	got, err := store.ListFiles(ctx, testRepo, head)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "a.go" || got[1] != "dir/b.go" {
		t.Errorf("ListFiles = %q, want [a.go dir/b.go]", got)
	}
}

func TestFetchPRReturnsBaseTipAndHasCommit(t *testing.T) {
	ctx := context.Background()
	r := newRemote(t)
	r.write("a.txt", "one\n")
	baseTip := r.commit("base")
	r.openPR(1, baseTip)
	store := storeFor(t, r)

	if ok, _ := store.HasCommit(ctx, testRepo, baseTip); ok {
		t.Error("HasCommit before any fetch = true")
	}
	got, err := store.FetchPR(ctx, domain.PRRef{Repo: testRepo, Number: 1}, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got != baseTip {
		t.Errorf("FetchPR base tip = %s, want %s", got, baseTip)
	}
	if ok, err := store.HasCommit(ctx, testRepo, baseTip); !ok || err != nil {
		t.Errorf("HasCommit after fetch = %v, %v", ok, err)
	}
	if ok, _ := store.HasCommit(ctx, testRepo, "0123456789012345678901234567890123456789"); ok {
		t.Error("HasCommit(unknown) = true")
	}
}

func TestGitAttributeAtSHA(t *testing.T) {
	ctx := context.Background()
	r := newRemote(t)
	r.write(".gitattributes", "gen/** linguist-generated\nvendor.js linguist-generated=false\n")
	r.write("gen/x.go", "package gen\n")
	r.write("vendor.js", "x\n")
	r.write("main.go", "package main\n")
	head := r.commit("head")
	r.openPR(1, head)
	store := storeFor(t, r)
	if _, err := store.FetchPR(ctx, domain.PRRef{Repo: testRepo, Number: 1}, "main"); err != nil {
		t.Fatal(err)
	}

	got, err := store.Attribute(ctx, testRepo, head, "linguist-generated", []string{"gen/x.go", "vendor.js", "main.go"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"gen/x.go": "set", "vendor.js": "false", "main.go": "unspecified"}
	for p, v := range want {
		if got[p] != v {
			t.Errorf("%s = %q, want %q", p, got[p], v)
		}
	}
}
