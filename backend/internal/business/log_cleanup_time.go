package business

import (
	"database/sql/driver"
	"time"

	"modernc.org/sqlite"
)

func init() {
	// Transient cleanup predicate only: do not persist this function in schema.
	// SQLite date functions lose nanosecond precision. Invalid legacy dates are
	// retained when applying a retention cutoff because their age is unknown.
	sqlite.MustRegisterDeterministicScalarFunction("console_log_before", 3, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		value, ok := args[0].(string)
		seconds, secondsOK := args[1].(int64)
		nanoseconds, nanosecondsOK := args[2].(int64)
		if !ok || !secondsOK || !nanosecondsOK {
			return int64(0), nil
		}
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return int64(0), nil
		}
		if parsed.Before(time.Unix(seconds, nanoseconds)) {
			return int64(1), nil
		}
		return int64(0), nil
	})
}
