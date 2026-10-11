package taskstore

import (
	"database/sql/driver"
	"time"

	"modernc.org/sqlite"
)

func init() {
	// Used only by transient history and cleanup queries, never stored in schema or indexes.
	// SQLite's date functions round to milliseconds; fixed-width UTC text keeps
	// both the source precision and chronological order across timezone offsets.
	sqlite.MustRegisterDeterministicScalarFunction("console_history_time", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		value, ok := args[0].(string)
		if !ok {
			return nil, nil
		}
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return nil, nil
		}
		// Most generated results are already fixed-width UTC. Validate them
		// above, then reuse the value instead of allocating an identical string.
		if len(value) == len(storageTimeLayout) && value[19] == '.' && value[len(value)-1] == 'Z' {
			return args[0], nil
		}
		return parsed.UTC().Format(storageTimeLayout), nil
	})
}
