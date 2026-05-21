package store

import (
	"context"
	"testing"
)

func TestConfigHome_CRUDAndOrdering(t *testing.T) {
	ctx, db := newTestStore(t)

	if err := db.UpsertConfigHome(ctx, UpsertConfigHomeParams{
		Entry: "abcdef1:.config/app", Dst: ".config/app", Section: "mig",
	}); err != nil {
		t.Fatalf("UpsertConfigHome(insert) failed: %v", err)
	}
	if err := db.UpsertConfigHome(ctx, UpsertConfigHomeParams{
		Entry: "bbbbbbb:.config/app", Dst: ".config/app", Section: "mig",
	}); err != nil {
		t.Fatalf("UpsertConfigHome(update) failed: %v", err)
	}
	if err := db.UpsertConfigHome(ctx, UpsertConfigHomeParams{
		Entry: "abcdef1:.local/bin", Dst: ".local/bin", Section: "mig",
	}); err != nil {
		t.Fatalf("UpsertConfigHome(second mig entry) failed: %v", err)
	}
	if err := db.UpsertConfigHome(ctx, UpsertConfigHomeParams{
		Entry: "1234567:.config/app", Dst: ".config/app", Section: "pre_gate",
	}); err != nil {
		t.Fatalf("UpsertConfigHome(second section) failed: %v", err)
	}

	migRows, err := db.ListConfigHomeBySection(ctx, "mig")
	if err != nil {
		t.Fatalf("ListConfigHomeBySection(mig) failed: %v", err)
	}
	if len(migRows) != 2 {
		t.Fatalf("mig rows=%d, want 2", len(migRows))
	}
	if migRows[0].Dst != ".config/app" || migRows[0].Entry != "bbbbbbb:.config/app" {
		t.Fatalf("first mig row = {%q %q}, want updated .config/app entry",
			migRows[0].Dst, migRows[0].Entry)
	}
	if migRows[1].Dst != ".local/bin" {
		t.Fatalf("second mig row dst=%q, want .local/bin", migRows[1].Dst)
	}

	allRows, err := db.ListConfigHome(ctx)
	if err != nil {
		t.Fatalf("ListConfigHome() failed: %v", err)
	}
	if len(allRows) != 3 {
		t.Fatalf("all rows=%d, want 3", len(allRows))
	}
	for i := 1; i < len(allRows); i++ {
		prev := allRows[i-1].Section + ":" + allRows[i-1].Dst
		curr := allRows[i].Section + ":" + allRows[i].Dst
		if prev > curr {
			t.Fatalf("ListConfigHome not ordered by section,dst: %q > %q", prev, curr)
		}
	}

	if err := db.DeleteConfigHome(ctx, DeleteConfigHomeParams{Dst: ".config/app", Section: "mig"}); err != nil {
		t.Fatalf("DeleteConfigHome() failed: %v", err)
	}
	migRows, err = db.ListConfigHomeBySection(ctx, "mig")
	if err != nil {
		t.Fatalf("ListConfigHomeBySection(mig after delete) failed: %v", err)
	}
	if len(migRows) != 1 || migRows[0].Dst != ".local/bin" {
		t.Fatalf("mig rows after delete = %#v, want only .local/bin", migRows)
	}
	if err := db.DeleteConfigHomeBySection(ctx, "pre_gate"); err != nil {
		t.Fatalf("DeleteConfigHomeBySection(pre_gate) failed: %v", err)
	}
	preGateRows, err := db.ListConfigHomeBySection(ctx, "pre_gate")
	if err != nil {
		t.Fatalf("ListConfigHomeBySection(pre_gate) failed: %v", err)
	}
	if len(preGateRows) != 0 {
		t.Fatalf("pre_gate rows after section delete=%d, want 0", len(preGateRows))
	}
}

func TestConfigHome_QueryContractTypes(t *testing.T) {
	type configHomeQuerier interface {
		ListConfigHome(ctx context.Context) ([]ConfigHome, error)
		ListConfigHomeBySection(ctx context.Context, section string) ([]ConfigHome, error)
		UpsertConfigHome(ctx context.Context, arg UpsertConfigHomeParams) error
		DeleteConfigHome(ctx context.Context, arg DeleteConfigHomeParams) error
		DeleteConfigHomeBySection(ctx context.Context, section string) error
	}
	var _ configHomeQuerier = (Querier)(nil)

	var home ConfigHome
	assertType[string](home.Entry)
	assertType[string](home.Dst)
	assertType[string](home.Section)

	var p DeleteConfigHomeParams
	assertType[string](p.Dst)
	assertType[string](p.Section)
}
