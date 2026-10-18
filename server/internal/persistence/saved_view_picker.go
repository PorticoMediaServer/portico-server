package persistence

import (
	"database/sql/driver"

	"portico.local/server/internal/thirdparty/sqlite"
)

// SQL maintenance of the derived picker must use exactly the same durable
// identity encoding as authenticated Go callers, including JSON escaping.
func init() {
	sqlite.MustRegisterDeterministicScalarFunction("portico_personal_owner_key", 3, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		authority, _ := args[0].(string)
		account, _ := args[1].(string)
		profile, _ := args[2].(string)
		return PersonalOwnerKey(authority, account, profile), nil
	})
}
