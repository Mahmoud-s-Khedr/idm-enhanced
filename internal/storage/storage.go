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

//go:embed migrations/002.sql
var migrationM3 string

//go:embed migrations/003.sql
var migrationM4 string

//go:embed migrations/004.sql
var migrationM5 string

//go:embed migrations/005.sql
var migrationM7 string

type Interval struct {
	Start, End int64
	Digest     string
}
type Job struct {
	Kind, MediaChoice                                            string
	Media                                                        json.RawMessage `json:"-"`
	MediaLoaded                                                  bool            `json:"-"`
	MediaObjects, MediaComplete                                  int
	MediaBytes                                                   int64
	Version                                                      int
	ID, Destination, Source, Effective, ETag, Encoding, Checksum string
	Partial, Identity, Ownership, State, Phase, Error            string
	Length, Durable, Generation                                  int64
	Intervals                                                    []Interval
	Digest                                                       string
	Created, Updated                                             time.Time
	QueueOrder                                                   int64
	RetryCount                                                   int
	RetryAt                                                      time.Time
	ScheduledAt                                                  time.Time
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
	if version > 5 || (readonly && version < 1) {
		return nil, errors.New("unsupported database version")
	}
	if !readonly {
		if err = s.configure("journal_mode", "WAL", "wal"); err != nil {
			return nil, err
		}
		if err = s.configure("synchronous", "FULL", "2"); err != nil {
			return nil, err
		}
		for target, script := range []string{migration, migrationM3, migrationM4, migrationM5, migrationM7} {
			if version > target {
				continue
			}
			tx, e := s.db.Begin()
			if e != nil {
				return nil, e
			}
			if _, e = tx.Exec(script); e != nil {
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
func (s *Store) Save(ctx context.Context, j Job) error { return s.SaveBatch(ctx, []Job{j}) }

// SaveBatch commits a manager-owned queue reorder atomically.
func (s *Store) SaveBatch(ctx context.Context, jobs []Job) error {
	return s.saveBatch(ctx, jobs, "", "")
}

func (s *Store) saveBatch(ctx context.Context, jobs []Job, requestID, fingerprint string) (err error) {
	// database/sql may win the cancellation race by rolling the transaction
	// back before Commit, returning ErrTxDone instead of the context cause.
	// Preserve both so callers still checkpoint/pause and classify interruption.
	defer func() {
		if errors.Is(err, sql.ErrTxDone) {
			err = errors.Join(err, ctx.Err())
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if requestID != "" {
		_, err = tx.ExecContext(ctx, "INSERT INTO browser_requests VALUES(?,?,?)", requestID, fingerprint, jobs[0].ID)
		if err != nil {
			return errors.Join(err, tx.Rollback())
		}
	}
	for _, j := range jobs {
		if j.Version != 1 {
			return errors.Join(errors.New("unsupported checkpoint version"), tx.Rollback())
		}
		j.Updated = time.Now().UTC()
		b, err := json.Marshal(j)
		if err != nil {
			return errors.Join(err, tx.Rollback())
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO jobs VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET state=excluded.state,generation=excluded.generation,durable=excluded.durable,record=excluded.record`, j.ID, j.Version, j.State, j.Generation, j.Durable, b)
		if err != nil {
			return errors.Join(err, tx.Rollback())
		}
		if j.MediaLoaded {
			if len(j.Media) == 0 {
				_, err = tx.ExecContext(ctx, "DELETE FROM media_checkpoints WHERE job_id=?", j.ID)
			} else if j.Kind != "hls" || len(j.Media) > 4<<20 || !json.Valid(j.Media) {
				err = errors.New("invalid media checkpoint")
			} else {
				_, err = tx.ExecContext(ctx, "INSERT INTO media_checkpoints VALUES(?,?) ON CONFLICT(job_id) DO UPDATE SET record=excluded.record", j.ID, []byte(j.Media))
			}
			if err != nil {
				return errors.Join(err, tx.Rollback())
			}
		}
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
		j, err := decodeJob(b)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}
func (s *Store) Get(ctx context.Context, id string) (Job, error) {
	var b []byte
	if err := s.db.QueryRowContext(ctx, "SELECT record FROM jobs WHERE id=?", id).Scan(&b); err != nil {
		return Job{}, err
	}
	j, err := decodeJob(b)
	if err != nil {
		return Job{}, err
	}
	if j.Kind == "hls" {
		j.MediaLoaded = true
		err = s.db.QueryRowContext(ctx, "SELECT record FROM media_checkpoints WHERE job_id=?", id).Scan(&j.Media)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
	}
	return j, err
}
func decodeJob(b []byte) (Job, error) {
	var j Job
	if json.Unmarshal(b, &j) != nil {
		return j, errors.New("invalid job record")
	}
	if j.Version != 1 {
		return j, errors.New("unsupported checkpoint version")
	}
	return j, nil
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

// Preferences contains application settings, separate from transfer checkpoints.
func (s *Store) Preferences(ctx context.Context) ([]byte, error) {
	var b []byte
	err := s.db.QueryRowContext(ctx, "SELECT record FROM preferences WHERE id=1").Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return b, err
}
func (s *Store) SavePreferences(ctx context.Context, b []byte) error {
	if len(b) > 4096 || !json.Valid(b) {
		return errors.New("invalid preferences")
	}
	_, err := s.db.ExecContext(ctx, "INSERT INTO preferences VALUES(1,?) ON CONFLICT(id) DO UPDATE SET record=excluded.record", b)
	return err
}

// Delete removes metadata only; filesystem retention is owned by the manager.
func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM jobs WHERE id=?", id)
	return err
}

// BrowserRequest returns the durable receipt even if history was removed.
func (s *Store) BrowserRequest(ctx context.Context, id string) (fingerprint, jobID string, err error) {
	err = s.db.QueryRowContext(ctx, "SELECT fingerprint,job_id FROM browser_requests WHERE request_id=?", id).Scan(&fingerprint, &jobID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}
func (s *Store) SaveBrowser(ctx context.Context, j Job, id, fingerprint string) error {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM browser_requests").Scan(&count); err != nil {
		return err
	}
	if count >= 10000 {
		return errors.New("browser receipt limit reached")
	}
	return s.saveBatch(ctx, []Job{j}, id, fingerprint)
}
