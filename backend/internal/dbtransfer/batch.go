package dbtransfer

import (
	"context"
	"fmt"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

// Bound both parameter count and payload size. A single large source value is
// still sent intact; it is never split or truncated to meet the batch budget.
type importBatch struct {
	tx             *dbconn.Tx
	insert, tuple  string
	report         *TableReport
	values         []any
	rows, byteSize int
}

func (batch *importBatch) add(ctx context.Context, values []any) error {
	size := 0
	for _, value := range values {
		switch value := value.(type) {
		case string:
			size += len(value)
		case []byte:
			size += len(value)
		default:
			size += 8
		}
	}
	if batch.rows > 0 && (batch.rows >= 100 || len(batch.values)+len(values) > 60000 || batch.byteSize+size > 1<<20) {
		if err := batch.flush(ctx); err != nil {
			return err
		}
	}
	batch.values = append(batch.values, values...)
	batch.rows++
	batch.byteSize += size
	return nil
}

func (batch *importBatch) flush(ctx context.Context) error {
	if batch.rows == 0 {
		return nil
	}
	query := batch.insert + ` VALUES ` + strings.TrimSuffix(strings.Repeat(batch.tuple+",", batch.rows), ",")
	if _, err := batch.tx.ExecContext(ctx, query, batch.values...); err != nil {
		return fmt.Errorf("表 %s 第 %d 至 %d 行无法导入（类型、长度、唯一性或关联不兼容）；未输出字段内容", batch.report.Table, batch.report.Rows+1, batch.report.Rows+int64(batch.rows))
	}
	batch.report.Rows += int64(batch.rows)
	clear(batch.values)
	batch.values = batch.values[:0]
	batch.rows, batch.byteSize = 0, 0
	return nil
}
