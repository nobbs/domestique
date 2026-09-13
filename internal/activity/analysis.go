package activity

import "time"

// Analysis is what a language model made of one ride, with what produced it.
type Analysis struct {
	AnalysedAt     time.Time
	Text           string
	Model          string
	PromptRevision int
}

// PendingAnalysis is a derived ride owed an analysis.
type PendingAnalysis struct {
	StartedAt time.Time
	ID        int64
}
