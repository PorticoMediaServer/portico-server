//go:build devtrust && !release

package networking

import "os"

// A development build on one machine with no private address (a laptop on a
// phone's hotspot) can publish http://127.0.0.1:<port> as a LAN route with
// PORTICO_DEV_LOOPBACK_ROUTE=1, so a browser or simulator on the same machine
// reaches it through the signed route list. Release builds never do.
func init() { devLoopbackRoute = os.Getenv("PORTICO_DEV_LOOPBACK_ROUTE") == "1" }
