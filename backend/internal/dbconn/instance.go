package dbconn

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
	"time"
)

// Instance pins the server connection that owns the single Console/importer
// lock. This protects process-local schedulers and offline imports as well as
// task recovery. Business transactions continue on their independent pools.
type Instance struct {
	conn   *sql.Conn
	pool   connection
	driver Driver
	ctx    context.Context
	cancel context.CancelCauseFunc
	done   chan struct{}
	once   sync.Once
}

func (db *Database) AcquireInstance(ctx context.Context) (*Instance, error) {
	if db.driver == SQLite {
		return nil, errors.New("SQLite 实例由本地服务入口管理")
	}
	// This connection must not consume a business-pool slot: a configured
	// one-connection pool still needs to run startup checks and transactions.
	config := db.config
	config.MaxOpenConns, config.MaxIdleConns = 1, 1
	pool, err := openServer(ctx, config)
	if err != nil {
		return nil, err
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		_ = pool.Close()
		return nil, errors.New("无法取得数据库实例锁连接")
	}
	query := `SELECT pg_try_advisory_lock(719882047)`
	// MySQL lock names are limited to 64 bytes.
	if db.driver == MySQL {
		query = `SELECT GET_LOCK(CONCAT('console_',LEFT(SHA2(DATABASE(),256),56)),0)`
	}
	var acquired bool
	if err := conn.QueryRowContext(ctx, query).Scan(&acquired); err != nil || !acquired {
		_ = conn.Close()
		_ = pool.Close()
		return nil, errors.New("数据库已有控制台或迁移进程，请先停止原进程")
	}
	leaseCtx, cancel := context.WithCancelCause(ctx)
	lease := &Instance{conn: conn, pool: pool, driver: db.driver, ctx: leaseCtx, cancel: cancel, done: make(chan struct{})}
	go lease.monitor()
	return lease, nil
}

func (i *Instance) Context() context.Context { return i.ctx }

func (i *Instance) monitor() {
	defer close(i.done)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-i.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(i.ctx, 5*time.Second)
			err := i.conn.PingContext(ctx)
			cancel()
			if err != nil {
				i.cancel(errors.New("数据库实例锁连接已中断，停止当前服务"))
				return
			}
		}
	}
}

func (i *Instance) Close() error {
	var result error
	i.once.Do(func() {
		i.cancel(context.Canceled)
		<-i.done
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		query := `SELECT pg_advisory_unlock(719882047)`
		if i.driver == MySQL {
			query = `SELECT RELEASE_LOCK(CONCAT('console_',LEFT(SHA2(DATABASE(),256),56)))`
		}
		if _, err := i.conn.ExecContext(ctx, query); err != nil {
			_ = i.conn.Raw(func(any) error { return driver.ErrBadConn })
			result = errors.New("释放数据库实例锁失败，连接已关闭")
		}
		result = errors.Join(result, i.conn.Close(), i.pool.Close())
	})
	return result
}
