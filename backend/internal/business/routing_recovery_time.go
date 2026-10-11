package business

import (
	"database/sql/driver"
	"encoding/binary"
	"time"

	"modernc.org/sqlite"
)

func init() {
	sqlite.MustRegisterDeterministicScalarFunction("console_recovery_time_v1", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		value, ok := args[0].(string)
		if !ok {
			return nil, nil
		}
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return nil, nil
		}
		return routingRecoveryTimeKey(parsed), nil
	})
}

// This versioned format is persisted in the recovery index. Flipping the sign
// bit makes big-endian seconds sortable before the unsigned nanosecond suffix,
// including UTC offsets that cross years 0 or 9999. Changes require a new index.
func routingRecoveryTimeKey(value time.Time) []byte {
	key := make([]byte, 12)
	binary.BigEndian.PutUint64(key, uint64(value.Unix())^(1<<63))
	binary.BigEndian.PutUint32(key[8:], uint32(value.Nanosecond()))
	return key
}
