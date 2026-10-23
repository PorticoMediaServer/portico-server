//go:build !linux

package decoder

import "context"

func DiscoverHardwareDevices(context.Context) []HardwareDevice { return []HardwareDevice{} }
