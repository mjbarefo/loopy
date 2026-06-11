package loop

import "errors"

// The logbook is the project's durable memory of human decisions:
// .loopy/logbook.md, one appended markdown entry per accept/reject, written
// for humans; review.json stays the structured record. LogbookEntries
// aggregates those records so `loopy logbook --json` needs no second store.
//
// NOTE: stubs — this file is the session's dogfood target, implemented by a
// loopy loop against logbook_test.go (see DECISIONS.md).

// LogbookPath returns .loopy/logbook.md.
func LogbookPath(root string) string {
	return ""
}

// appendLogbook appends one human-readable markdown entry for a decision.
func appendLogbook(root string, l Loop, r Review) error {
	return errors.New("logbook not implemented yet")
}

// LogbookEntries returns every recorded decision across loops, ordered by
// decision time (oldest first), ties broken by loop ID.
func LogbookEntries(root string) ([]Review, error) {
	return nil, errors.New("logbook not implemented yet")
}
