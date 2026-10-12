// Package dbtransfer performs explicit, offline SQLite-to-server transfers.
package dbtransfer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
	sqlite "modernc.org/sqlite"
)

type Sources struct{ Config, Business, Tasks string }

type Snapshot struct {
	Original Sources
	Prepared Sources
	locks    []*sql.Conn
	pools    []*sql.DB
}

func (s *Snapshot) Close() error {
	var result error
	for _, lock := range s.locks {
		_, err := lock.ExecContext(context.Background(), "ROLLBACK")
		result = errors.Join(result, err, lock.Close())
	}
	for _, pool := range s.pools {
		result = errors.Join(result, pool.Close())
	}
	s.locks, s.pools = nil, nil
	return result
}

// SnapshotSources acquires every write lock before copying any source. The
// caller must first stop Console: SQLite locks alone cannot finish an already
// interrupted application operation that spans the three databases.
// Locks remain held until Close; original backups are never schema-upgraded.
func SnapshotSources(ctx context.Context, sources Sources, directory string) (_ *Snapshot, finalErr error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("备份目录必须是新的绝对路径")
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		return nil, errors.New("备份目录必须不存在且可以创建")
	}
	if err := os.Mkdir(filepath.Join(directory, "prepared"), 0700); err != nil {
		return nil, err
	}
	snapshot := &Snapshot{}
	defer func() {
		if finalErr != nil {
			_ = snapshot.Close()
		}
	}()
	paths := []string{sources.Config, sources.Business, sources.Tasks}
	seen := map[string]bool{}
	for index, path := range paths {
		info, err := os.Lstat(path)
		if err != nil || !filepath.IsAbs(path) || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("源库 %d 必须是已存在的绝对普通文件路径", index+1)
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || seen[resolved] {
			return nil, errors.New("三个源库必须是不同的现有文件")
		}
		seen[resolved] = true
		pool, err := sql.Open("sqlite", sqliteutil.DSN(path, "mode=rw&_pragma=busy_timeout(1000)"))
		if err != nil {
			return nil, errors.New("无法打开源数据库")
		}
		snapshot.pools = append(snapshot.pools, pool)
		conn, err := pool.Conn(ctx)
		if err != nil {
			return nil, errors.New("无法连接源数据库")
		}
		if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
			conn.Close()
			return nil, errors.New("源库仍有写入，请停止控制台后重试")
		}
		snapshot.locks = append(snapshot.locks, conn)
	}
	outputs := []*string{&snapshot.Original.Config, &snapshot.Original.Business, &snapshot.Original.Tasks}
	prepared := []*string{&snapshot.Prepared.Config, &snapshot.Prepared.Business, &snapshot.Prepared.Tasks}
	for index, name := range []string{"config.sqlite3", "business.sqlite3", "tasks.sqlite3"} {
		target := filepath.Join(directory, name)
		reader, err := snapshot.pools[index].Conn(ctx)
		if err != nil {
			return nil, errors.New("无法读取源数据库快照")
		}
		err = backupConnection(ctx, reader, target)
		closeErr := reader.Close()
		if err != nil || closeErr != nil {
			return nil, fmt.Errorf("源库 %d 备份失败；原库未修改", index+1)
		}
		*outputs[index] = target
		*prepared[index] = filepath.Join(directory, "prepared", name)
		if err := copyPrivateFile(target, *prepared[index]); err != nil {
			return nil, err
		}
	}
	return snapshot, nil
}

func backupConnection(ctx context.Context, conn *sql.Conn, target string) error {
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return conn.Raw(func(raw any) error {
		backuper, ok := raw.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return errors.New("SQLite 驱动不支持在线备份")
		}
		backup, err := backuper.NewBackup(sqliteutil.DSN(target, "mode=rw"))
		if err != nil {
			return err
		}
		for {
			if err := ctx.Err(); err != nil {
				_ = backup.Finish()
				return err
			}
			more, err := backup.Step(256)
			if err != nil {
				_ = backup.Finish()
				return err
			}
			if !more {
				return backup.Finish()
			}
		}
	})
}

func copyPrivateFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	syncErr := output.Sync()
	return errors.Join(copyErr, syncErr, output.Close())
}
