// Package protokol is the only thing both halves import. Everything else lives
// on one side of the pipe and stays there.
package protokol

// Versiya is bumped when the wire changes shape. The two halves compare it in
// the first frame and refuse to talk if they disagree, because a service and a
// UI from different releases will otherwise fail in interesting ways at 3am.
const Versiya = 1
