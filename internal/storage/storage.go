// Package storage owns protected SQLite persistence and transactional checkpoints.
package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/localfs"
	_ "modernc.org/sqlite"
)

//go:embed migrations/001.sql
var migration string

type Interval struct {
	Start, End int64
	Digest     string
}
type Job struct {
	Version                                                      int
	ID, Destination, Source, Effective, ETag, Encoding, Checksum string
	Partial, Identity, State, Phase, Error                       string
	Length, Durable, Generation                                  int64
	Intervals                                                    []Interval
	Digest                                                       string
	Created, Updated                                             time.Time
}
type Store struct {
	db   *sql.DB
	lock *os.File
}

func Open(dir string, readonly bool) (_ *Store, err error) {
	if !readonly {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	// Refuse symlinks in the state directory chain before opening any files.
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for p := absolute; ; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		if e != nil {
			return nil, e
		}
		if info.Mode()&os.ModeSymlink != 0 && !localfs.SystemAlias(p) {
			return nil, errors.New("symlink in state path")
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	if err := protect(dir, true, readonly); err != nil {
		return nil, err
	}
	s := &Store{}
	defer func() {
		if err != nil {
			err = errors.Join(err, s.Close())
		}
	}()
	if !readonly {
		path := filepath.Join(dir, "owner.lock")
		if info, e := os.Lstat(path); e == nil && !info.Mode().IsRegular() {
			return nil, errors.New("unsafe lock path")
		}
		s.lock, err = os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		if err = protect(path, false, readonly); err != nil {
			return nil, err
		}
		if err = localfs.Lock(s.lock); err != nil {
			return nil, errors.New("another modifying command owns this state directory")
		}
	}
	path := filepath.Join(absolute, "jobs.db")
	if !readonly {
		f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if e == nil {
			e = f.Close()
		}
		if e != nil && !errors.Is(e, os.ErrExist) {
			return nil, e
		}
	}
	if err = protect(path, false, readonly); err != nil {
		return nil, err
	}
	// SQLite sidecars must also be regular, protected files when present.
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, e := os.Lstat(path + suffix); e == nil {
			if err = protect(path+suffix, false, readonly); err != nil {
				return nil, err
			}
		}
	}
	sqlitePath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" {
		sqlitePath = "/" + sqlitePath
	}
	u := url.URL{Scheme: "file", Path: sqlitePath}
	mode := "rwc"
	if readonly {
		mode = "ro"
	}
	s.db, err = sql.Open("sqlite", u.String()+"?mode="+mode)
	if err != nil {
		return nil, err
	}
	s.db.SetMaxOpenConns(1)
	s.db.SetMaxIdleConns(1)
	for _, setting := range []struct{ name, value string }{{"busy_timeout", "5000"}, {"foreign_keys", "1"}} {
		if err = s.configure(setting.name, setting.value, setting.value); err != nil {
			return nil, err
		}
	}
	var version int
	if err = s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return nil, err
	}
	if version > 1 || (readonly && version != 1) {
		return nil, errors.New("unsupported database version")
	}
	if !readonly {
		if err = s.configure("journal_mode", "WAL", "wal"); err != nil {
			return nil, err
		}
		if err = s.configure("synchronous", "FULL", "2"); err != nil {
			return nil, err
		}
		if version == 0 {
			tx, e := s.db.Begin()
			if e != nil {
				return nil, e
			}
			if _, e = tx.Exec(migration); e != nil {
				return nil, errors.Join(e, tx.Rollback())
			}
			if err = tx.Commit(); err != nil {
				return nil, err
			}
		}
	}
	return s, nil
}
func (s *Store) Close() error {
	var err error
	if s.db != nil {
		err = s.db.Close()
	}
	if s.lock != nil {
		err = errors.Join(err, s.lock.Close())
	}
	return err
}
func (s *Store) Save(ctx context.Context, j Job) error {
	if j.Version != 1 {
		return errors.New("unsupported checkpoint version")
	}
	j.Updated = time.Now().UTC()
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO jobs VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET state=excluded.state,generation=excluded.generation,durable=excluded.durable,record=excluded.record`, j.ID, j.Version, j.State, j.Generation, j.Durable, b)
	if err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}
func (s *Store) List(ctx context.Context) (jobs []Job, err error) {
	rows, err := s.db.QueryContext(ctx, "SELECT record FROM jobs ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		var j Job
		if err := json.Unmarshal(b, &j); err != nil {
			return nil, errors.New("invalid job record")
		}
		if j.Version != 1 {
			return nil, errors.New("unsupported checkpoint version")
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}
func (s *Store) Get(ctx context.Context, id string) (Job, error) {
	jobs, err := s.List(ctx)
	if err != nil {
		return Job{}, err
	}
	for _, j := range jobs {
		if j.ID == id {
			return j, nil
		}
	}
	return Job{}, errors.New("job not found")
}

func protect(path string, directory, readonly bool) error {
	if readonly {
		return localfs.CheckProtected(path, directory)
	}
	return localfs.Protect(path, directory)
}

func (s *Store) configure(name, value, want string) error {
	if _, err := s.db.Exec("PRAGMA " + name + "=" + value); err != nil {
		return err
	}
	var got string
	if err := s.db.QueryRow("PRAGMA " + name).Scan(&got); err != nil {
		return err
	}
	if got != want {
		return errors.New("SQLite setting rejected: " + name)
	}
	return nil
}
