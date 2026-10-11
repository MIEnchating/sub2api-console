package taskstore

import (
	"database/sql/driver"
	"time"

	"modernc.org/sqlite"
)

func init() {
	// UTC conversion can cross year 9999, where formatted text no longer sorts chronologically.
	// This predicate is transient and must not be persisted in schema or indexes.
	sqlite.MustRegisterDeterministicScalarFunction("console_history_before", 3, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
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
