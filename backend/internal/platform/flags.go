package platform

import "sync/atomic"

// Flags holds settings that can change while the process is running.
//
// Separate from Config on purpose. Config is read once at boot and is immutable
// afterwards — that is what makes a misconfigured deployment fail loudly at
// startup rather than behave oddly at 3am. Flags are the deliberate exception:
// values an operator must be able to change *during an incident*, without a
// deploy and without a restart.
//
// The bar for adding one is high. A flag is a branch that will be taken in
// production at a moment nobody is watching, so each one needs a reason it
// cannot be a Config value. There is exactly one today.
type Flags struct {
	// legacyShim controls whether the deprecated /api/go/ prefix is served
	// (WI-21, TRD §6.1).
	//
	// Why this must be togglable at runtime: the shim exists to be switched OFF
	// one day, and the only honest way to find out whether a client still
	// depends on it is to turn it off, watch, and turn it back on quickly if
	// something screams. Making that a redeploy makes the rollback slow enough
	// that nobody will try the experiment, and the shim then lives forever.
	legacyShim atomic.Bool
}

func NewFlags(legacyShim bool) *Flags {
	f := &Flags{}
	f.legacyShim.Store(legacyShim)
	return f
}

func (f *Flags) LegacyShim() bool     { return f.legacyShim.Load() }
func (f *Flags) SetLegacyShim(v bool) { f.legacyShim.Store(v) }
