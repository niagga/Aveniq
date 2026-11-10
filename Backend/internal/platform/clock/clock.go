// Package clock provides the system clock behind the port the application uses.
package clock

import "time"

// System reports the real current time.
type System struct{}

// New returns the production clock.
func New() System { return System{} }

// Now implements port.Clock.
func (System) Now() time.Time { return time.Now() }
