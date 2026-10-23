package postgres

import (
	"errors"
)

var ErrMembershipBusy = errors.New("account or server membership operation is in progress")
