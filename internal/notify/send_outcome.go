package notify

import "fmt"

// sendOutcome accumulates the result of a multi-target send.
//
// Upstream never abandons a notification because one target failed. Its send
// loops set has_error and continue, so a four-target notification with one dead
// target still reaches the other three and reports failure at the end. Bailing
// on the first error produces the same overall verdict -- which is why no
// success/failure comparison catches the difference -- while silently dropping
// every target after the first bad one.
//
// Record every attempt, keep the first error for the message, and report
// failure once the whole send is done.
type sendOutcome struct {
	first    error
	failed   int
	attempts int
}

// record notes one target's result. A nil error is a success.
func (o *sendOutcome) record(err error) {
	o.attempts++
	if err == nil {
		return
	}
	o.failed++
	if o.first == nil {
		o.first = err
	}
}

// err reports the send as a whole: nil when every target succeeded, otherwise
// the first failure, noting how many others went the same way.
func (o *sendOutcome) err() error {
	if o.failed == 0 {
		return nil
	}
	if o.failed == 1 || o.attempts == 1 {
		return o.first
	}
	return fmt.Errorf("%d of %d targets failed, first error: %w",
		o.failed, o.attempts, o.first)
}
