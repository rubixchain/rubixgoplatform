package core

import "testing"

// entriesAfterLocalPrefix replaces enriched[len(localChain):], which panicked when local was longer than remote.
func TestEntriesAfterLocalPrefix(t *testing.T) {
	remote := []string{"tx0", "tx1", "tx2"}

	t.Run("local longer than remote returns nil without panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("unexpected panic: %v", r)
			}
		}()
		if got := entriesAfterLocalPrefix(remote, len(remote)+1); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("local equal to remote returns nil", func(t *testing.T) {
		if got := entriesAfterLocalPrefix(remote, len(remote)); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("remote ahead returns only the tail", func(t *testing.T) {
		got := entriesAfterLocalPrefix(remote, 1)
		if len(got) != 2 || got[0] != "tx1" || got[1] != "tx2" {
			t.Fatalf("expected [tx1 tx2], got %v", got)
		}
	})

	t.Run("empty local returns the whole remote", func(t *testing.T) {
		if got := entriesAfterLocalPrefix(remote, 0); len(got) != len(remote) {
			t.Fatalf("expected %d entries, got %d", len(remote), len(got))
		}
	})

	t.Run("empty remote returns nil", func(t *testing.T) {
		if got := entriesAfterLocalPrefix([]string{}, 0); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})
}
