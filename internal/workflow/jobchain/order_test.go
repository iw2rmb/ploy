package jobchain

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

type testItem struct {
	id   domaintypes.JobID
	next *domaintypes.JobID
}

func TestOrder_ReconstructsLinkedChain(t *testing.T) {
	t.Parallel()

	pre := domaintypes.JobID("pre")
	mig0 := domaintypes.JobID("mig0")
	mig1 := domaintypes.JobID("mig1")
	post := domaintypes.JobID("post")

	items := []testItem{
		{id: post, next: nil},
		{id: mig1, next: &post},
		{id: mig0, next: &mig1},
		{id: pre, next: &mig0},
	}

	got := Order(
		items,
		func(item testItem) domaintypes.JobID { return item.id },
		func(item testItem) *domaintypes.JobID { return item.next },
	)

	if len(got) != 4 {
		t.Fatalf("expected 4 items, got %d", len(got))
	}
	if got[0].id != pre || got[1].id != mig0 || got[2].id != mig1 || got[3].id != post {
		t.Fatalf("unexpected order: got [%s, %s, %s, %s]", got[0].id, got[1].id, got[2].id, got[3].id)
	}
}
