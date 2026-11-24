// Package pin defines the record a pin-<host> effect publishes after
// `niks3 push --pin`, whatever the transport.
package pin

import "time"

// Record says that CI pinned Toplevel, built from Commit, for Host at Time.
type Record struct {
	Host     string    `json:"host"`
	Commit   string    `json:"commit"`
	Toplevel string    `json:"toplevel"`
	Time     time.Time `json:"time"`
}
