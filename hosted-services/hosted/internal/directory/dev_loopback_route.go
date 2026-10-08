//go:build devtrust && !release

package directory

import "os"

// A development Hosted accepts a loopback LAN route (a server and its clients
// on one machine with no private address) with PORTICO_DEV_LOOPBACK_ROUTE=1.
// Release builds never do.
func init() { devLoopbackRoutes = os.Getenv("PORTICO_DEV_LOOPBACK_ROUTE") == "1" }
