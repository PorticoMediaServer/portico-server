package postgres

import (
	"strconv"
	"time"
)

// pgInterval renders a duration as a PostgreSQL interval literal in whole seconds.
func pgInterval(d time.Duration) string {
	return strconv.FormatInt(int64(d/time.Second), 10) + " seconds"
}
