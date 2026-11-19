package audit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCheckAnchor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"one", "two", "three"} {
		l.Record(m)
	}
	n, hash := l.Head()
	if n != 3 {
		t.Fatalf("head n = %d", n)
	}
	l.Close()
	if err := CheckAnchor(path, n, hash); err != nil {
		t.Fatalf("a good anchor failed: %v", err)
	}
	if err := CheckAnchor(path, n, strings.Repeat("a", 64)); err == nil {
		t.Error("a wrong hash passed")
	}
	// Cut the last entry off: the file is still a valid chain, but the anchor catches it.
	b, _ := os.ReadFile(path)
	lines := strings.SplitAfter(strings.TrimRight(string(b), "\n"), "\n")
	if err := os.WriteFile(path, []byte(strings.Join(lines[:2], "")), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Verify(path); err != nil {
		t.Fatalf("the truncated file should still verify alone: %v", err)
	}
	if err := CheckAnchor(path, n, hash); err == nil || !strings.Contains(err.Error(), "cut off") {
		t.Errorf("a truncated file passed or said the wrong thing: %v", err)
	}
}

func TestRunAnchorsPostsOnlyWhenTheHeadMoved(t *testing.T) {
	l, err := Open(filepath.Join(t.TempDir(), "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	l.Record("first")
	var mu sync.Mutex
	var got []string
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		RunAnchors(ctx, l, 20*time.Millisecond, time.Hour, nil, func(m string) { mu.Lock(); got = append(got, m); mu.Unlock() })
		close(done)
	}()
	time.Sleep(100 * time.Millisecond) // several ticks, no new entries: still one post
	l.Record("second")
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 || !strings.Contains(got[0], "entry 1,") || !strings.Contains(got[1], "entry 2,") {
		t.Errorf("posts = %q, want one for entry 1 and one for entry 2", got)
	}
}

func TestRunAnchorsPostsSoonAfterAKick(t *testing.T) {
	l, err := Open(filepath.Join(t.TempDir(), "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	l.Record("first")
	var mu sync.Mutex
	var got []string
	kick := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		RunAnchors(ctx, l, time.Hour, 30*time.Millisecond, kick, func(m string) { mu.Lock(); got = append(got, m); mu.Unlock() })
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	l.Record("a user was added")
	kick <- struct{}{}
	time.Sleep(150 * time.Millisecond) // far sooner than the hour-long interval
	cancel()
	<-done
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 || !strings.Contains(got[1], "entry 2,") {
		t.Errorf("posts = %q, want the second right after the kick", got)
	}
}
