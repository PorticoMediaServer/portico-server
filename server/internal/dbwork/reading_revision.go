package dbwork

import (
	"database/sql/driver"

	"modernc.org/sqlite"
	"portico.local/server/internal/readingidentity"
)

func init() {
	sqlite.MustRegisterDeterministicScalarFunction("portico_reading_revision", 14, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		fields := make([]any, len(args))
		for i, value := range args {
			switch i {
			case 0, 1, 4, 5, 7:
				if _, ok := value.(int64); !ok {
					return nil, nil
				}
			default:
				if _, ok := value.(string); !ok {
					return nil, nil
				}
			}
			fields[i] = value
		}
		return readingidentity.Revision(fields...), nil
	})
}
