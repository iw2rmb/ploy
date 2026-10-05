package store

import (
	"encoding/json"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestJobReportReplacementSurvivesCompletion(t *testing.T) {
	ctx, db := newTestStore(t)
	fx := newV1Fixture(t, ctx, db, "https://github.com/example/report-test.git", "main", []byte(`{"steps":[]}`))
	job := createTestJob(t, ctx, db, fx, "report")
	post := func(text string) {
		t.Helper()
		n, err := db.UpdateJobReport(ctx, UpdateJobReportParams{ID: job.ID, Report: text})
		if err != nil || n != 1 {
			t.Fatalf("post: %d %v", n, err)
		}
	}
	complete := func() {
		t.Helper()
		err := db.UpdateJobCompletionWithMeta(ctx, UpdateJobCompletionWithMetaParams{ID: job.ID, Status: types.JobStatusSuccess, Meta: []byte(`{"kind":"mig","mig_step_name":"report","report":"stale"}`)})
		if err != nil {
			t.Fatal(err)
		}
	}
	check := func(want string) {
		t.Helper()
		got, err := db.GetJob(ctx, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		var meta map[string]any
		if err := json.Unmarshal(got.Meta, &meta); err != nil {
			t.Fatal(err)
		}
		if meta["report"] != want || meta["mig_step_name"] != "report" {
			t.Fatalf("metadata lost: %s", got.Meta)
		}
	}
	post("first\nreport")
	complete()
	check("first\nreport")
	post("replacement")
	check("replacement")
	post("")
	complete()
	check("")
	// Both writers lock the same row. Either order must preserve the new report.
	start := make(chan struct{})
	errors := make(chan error, 2)
	go func() {
		<-start
		_, err := db.UpdateJobReport(ctx, UpdateJobReportParams{ID: job.ID, Report: "concurrent"})
		errors <- err
	}()
	go func() {
		<-start
		errors <- db.UpdateJobCompletionWithMeta(ctx, UpdateJobCompletionWithMetaParams{ID: job.ID, Status: types.JobStatusSuccess, Meta: []byte(`{"kind":"mig","mig_step_name":"report","report":"stale"}`)})
	}()
	close(start)
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	check("concurrent")
}
