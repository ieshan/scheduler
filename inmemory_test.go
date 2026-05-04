package scheduler

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ieshan/idx"
)

// mustID converts a string to idx.ID, panicking on invalid input (for test use only).
func mustID(s string) idx.ID {
	id, err := idx.FromString(s)
	if err != nil {
		panic(fmt.Sprintf("invalid ULID %q: %v", s, err))
	}
	return id
}

var _ JobStore = (*InMemoryJobStore)(nil)

func TestInMemoryJobStore_SaveAndGet(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store := NewInMemoryJobStore()
	job := &Job{ID: mustID("01HZY0CWD00000000000000005"), Name: "test"}
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Get(ctx, mustID("01HZY0CWD00000000000000005"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Verify sentinel error for missing job.
	_, err = store.Get(ctx, mustID("01HZY0CWD00000000000000006"))
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound, got %v", err)
	}
	if got.Name != "test" {
		t.Errorf("Name = %q, want test", got.Name)
	}
}

func TestInMemoryJobStore_List(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store := NewInMemoryJobStore()
	store.Save(ctx, &Job{ID: mustID("01HZY0CWD00000000000000007"), Enabled: true})
	store.Save(ctx, &Job{ID: mustID("01HZY0CWD00000000000000008"), Enabled: false})
	store.Save(ctx, &Job{ID: mustID("01HZY0CWD00000000000000009"), Enabled: true})
	jobs, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(jobs) != 3 {
		t.Errorf("List len = %d, want 3", len(jobs))
	}
}

func TestInMemoryJobStore_Delete(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store := NewInMemoryJobStore()
	store.Save(ctx, &Job{ID: mustID("01HZY0CWD0000000000000000A")})
	if err := store.Delete(ctx, mustID("01HZY0CWD0000000000000000A")); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := store.Get(ctx, mustID("01HZY0CWD0000000000000000A"))
	if err == nil {
		t.Fatal("expected error after delete")
	}
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound after delete, got %v", err)
	}
}

func TestInMemoryJobStore_UpdateState(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store := NewInMemoryJobStore()
	store.Save(ctx, &Job{ID: mustID("01HZY0CWD0000000000000000B")})
	err := store.UpdateState(ctx, mustID("01HZY0CWD0000000000000000B"), JobState{
		LastStatus: StatusSuccess,
		RunCount:   1,
	})
	if err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	got, _ := store.Get(ctx, mustID("01HZY0CWD0000000000000000B"))
	if got.State.LastStatus != StatusSuccess {
		t.Errorf("LastStatus = %q, want success", got.State.LastStatus)
	}
	if got.State.RunCount != 1 {
		t.Errorf("RunCount = %d, want 1", got.State.RunCount)
	}

	// Verify ErrJobNotFound for non-existent job
	err = store.UpdateState(ctx, mustID("01HZY0CWD0000000000000000C"), JobState{})
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound for non-existent job, got %v", err)
	}
}

func TestInMemoryJobStore_SaveOverwrite(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store := NewInMemoryJobStore()
	store.Save(ctx, &Job{ID: mustID("01HZY0CWD0000000000000000D"), Name: "original"})
	store.Save(ctx, &Job{ID: mustID("01HZY0CWD0000000000000000D"), Name: "updated"})
	got, err := store.Get(ctx, mustID("01HZY0CWD0000000000000000D"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "updated" {
		t.Errorf("Name = %q, want %q", got.Name, "updated")
	}
}

func TestInMemoryJobStore_ListEmpty(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store := NewInMemoryJobStore()
	jobs, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("List len = %d, want 0", len(jobs))
	}
}

func TestInMemoryJobStore_GetReturnsCopy(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store := NewInMemoryJobStore()
	store.Save(ctx, &Job{ID: mustID("01HZY0CWD0000000000000000E"), Name: "original"})
	got, _ := store.Get(ctx, mustID("01HZY0CWD0000000000000000E"))
	got.Name = "modified"
	orig, _ := store.Get(ctx, mustID("01HZY0CWD0000000000000000E"))
	if orig.Name != "original" {
		t.Errorf("modifying Get result affected store: got %q, want %q", orig.Name, "original")
	}
}

func TestInMemoryJobStore_DeleteNonexistent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store := NewInMemoryJobStore()
	err := store.Delete(ctx, mustID("01HZY0CWD0000000000000000F"))
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound, got %v", err)
	}
}
