package postgres

import (
	"context"
	"time"
)

// Test hooks for the unknown-username cool-off (HOST03).
const (
	UnknownPasswordBuckets = unknownPasswordBuckets
	UnknownPasswordNetwork = unknownPasswordNetwork
	UnknownPasswordLock    = unknownPasswordLock
	UnknownPasswordWide    = unknownPasswordWide
)

func (s *AccountSecurity) AdmitUnknownPasswordAttempt(ctx context.Context, username, network string) (bool, time.Time, error) {
	return s.admitUnknownPasswordAttempt(ctx, username, passwordNetwork(network))
}
func (s *AccountSecurity) UnknownPasswordBucket(kind string, slot uint32) []byte {
	return s.unknownPasswordBucket(kind, slot)
}
