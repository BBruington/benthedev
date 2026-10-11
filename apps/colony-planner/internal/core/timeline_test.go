package core_test

import (
	"testing"
	"time"

	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

// Players' tabs toast only entries written after the newest one they have seen (web/src/lib/colony.ts),
// so created_at must follow commit order: a note waits for a colony change that holds the lock, and
// each entry is stamped when it is written, not when its transaction began.
func TestTimelineEntriesAreStampedInCommitOrder(t *testing.T) {
	test := newColonyTest(t)
	transaction, err := test.database.Pool.Begin(test.context)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = transaction.Rollback(test.context) }()
	locked := store.New(transaction)
	if _, err := locked.GetColonyForUpdate(test.context); err != nil {
		t.Fatalf("lock the colony: %v", err)
	}
	var transactionStart time.Time
	if err := transaction.QueryRow(test.context, "SELECT now()").Scan(&transactionStart); err != nil {
		t.Fatalf("read the transaction's start: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	lockedEntry, err := locked.CreateTimelineEvent(test.context, store.CreateTimelineEventParams{
		Day:        0,
		Kind:       string(core.TimelineKindCustom),
		Title:      "Written under the lock",
		Visibility: core.TimelineVisibilityPublic,
		Deltas:     []byte("{}"),
		Data:       []byte("{}"),
	})
	if err != nil {
		t.Fatalf("write an entry under the lock: %v", err)
	}
	if !lockedEntry.CreatedAt.After(transactionStart.Add(10 * time.Millisecond)) {
		t.Fatalf("entry stamped %v, transaction began %v; want the time it was written", lockedEntry.CreatedAt, transactionStart)
	}

	type outcome struct {
		entry core.TimelineEntry
		err   error
	}
	noteWritten := make(chan outcome, 1)
	go func() {
		entry, err := test.service.CreateCustomEntry(test.context, test.dm, core.TimelineEntryInput{
			Day:        0,
			Title:      "A note from another tab",
			Visibility: core.TimelineVisibilityPublic,
		})
		noteWritten <- outcome{entry: entry, err: err}
	}()
	select {
	case written := <-noteWritten:
		t.Fatalf("the note was written while a colony change held the lock (%v)", written.err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := transaction.Commit(test.context); err != nil {
		t.Fatalf("commit: %v", err)
	}
	select {
	case written := <-noteWritten:
		if written.err != nil {
			t.Fatalf("write the note: %v", written.err)
		}
		if !written.entry.CreatedAt.After(lockedEntry.CreatedAt) {
			t.Fatalf("note stamped %v, earlier entry %v; want the note later", written.entry.CreatedAt, lockedEntry.CreatedAt)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the note never got the lock")
	}
}
