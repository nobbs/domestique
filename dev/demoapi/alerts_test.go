package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Names must match dev/demoapi/tasks.go, not the pre-split "sync" the shipped
// build no longer has.
func TestDemoAlertCatalogueNamesTheSplitTasks(t *testing.T) {
	t.Parallel()

	seen := map[string]bool{}
	for _, alert := range demoAlertCatalogue {
		assert.NotEqual(t, "sync", alert.task, "the catalogue still names the pre-split task")
		seen[alert.task] = true
	}
	assert.True(t, seen["sync:source"], "sync:source is missing from the catalogue")
	assert.True(t, seen["sync:target"], "sync:target is missing from the catalogue")
	assert.True(t, seen["sync:clear"], "sync:clear is missing from the catalogue")
}

func TestDemoAlertCatalogueDeclaresStaleOnlyForTheRead(t *testing.T) {
	t.Parallel()

	for _, alert := range demoAlertCatalogue {
		if alert.alert != "stale" {
			continue
		}
		assert.Equal(t, "sync:source", alert.task, "stale declared for a task that cannot go stale")
	}
}
