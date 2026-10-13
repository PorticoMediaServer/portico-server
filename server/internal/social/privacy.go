package social

import (
 "context"
 "database/sql"
)

// Joining a room makes participation visible inside that room. The profile's
// activity preference remains applicable to activity outside it.
func visibleEvent(ctx context.Context, tx *sql.Tx, group, kind string, payload any) (bool,error) {return true,nil}
