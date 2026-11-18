package tenant

import "time"

// TrialExpired reports whether the practice is on its trial and the trial has ended.
//
// The end is inclusive: at trial_ends_at the trial is over. Only a practice in the
// trial state can expire. An active practice has a current subscription and is never
// expired by this rule; a suspended one is decided elsewhere.
//
// An expired practice can still be read. Writes are refused by the API (T46), which is
// the only place that knows the request is a write.
func (t Tenant) TrialExpired(now time.Time) bool {
	return t.Status == StatusTrial && !now.Before(t.TrialEndsAt)
}
