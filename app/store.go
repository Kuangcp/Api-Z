package app

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	mu   sync.Mutex
	db   *sql.DB
	path string
}

func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, path: path}
	if err := s.migrateSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Path() string { return s.path }

func (s *Store) migrateSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_meta (
			key TEXT PRIMARY KEY NOT NULL,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS collections (
			id TEXT PRIMARY KEY NOT NULL,
			name TEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			meta_json TEXT NOT NULL DEFAULT '{}'
		)`,
		`CREATE TABLE IF NOT EXISTS folders (
			id TEXT PRIMARY KEY NOT NULL,
			collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
			parent_folder_id TEXT REFERENCES folders(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			meta_json TEXT NOT NULL DEFAULT '{}'
		)`,
		`CREATE TABLE IF NOT EXISTS requests (
			id TEXT PRIMARY KEY NOT NULL,
			collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
			folder_id TEXT REFERENCES folders(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			method TEXT NOT NULL,
			url TEXT NOT NULL,
			headers_text TEXT NOT NULL DEFAULT '',
			params_text TEXT NOT NULL DEFAULT '',
			body_text TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			meta_json TEXT NOT NULL DEFAULT '{}'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_folders_collection ON folders(collection_id)`,
		`CREATE INDEX IF NOT EXISTS idx_folders_parent ON folders(parent_folder_id)`,
		`CREATE INDEX IF NOT EXISTS idx_requests_collection ON requests(collection_id)`,
		`CREATE INDEX IF NOT EXISTS idx_requests_folder ON requests(folder_id)`,
		`CREATE TABLE IF NOT EXISTS exchanges (
			id TEXT PRIMARY KEY NOT NULL,
			request_id TEXT NOT NULL,
			saved_at INTEGER NOT NULL,
			status INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL,
			size_bytes INTEGER NOT NULL,
			is_sse INTEGER NOT NULL,
			ttft_ms INTEGER NOT NULL,
			event_count INTEGER NOT NULL,
			har_path TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_exchanges_request ON exchanges(request_id, saved_at DESC)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("store: schema: %w", err)
		}
	}
	return nil
}

func (s *Store) Meta(key string) string {
	var v string
	err := s.db.QueryRow(`SELECT value FROM schema_meta WHERE key = ?`, key).Scan(&v)
	if err != nil {
		return ""
	}
	return v
}

func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO schema_meta(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) Count(table string) (int, error) {
	switch table {
	case "collections", "folders", "requests", "exchanges":
	default:
		return 0, fmt.Errorf("store: unknown table %q", table)
	}
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n)
	return n, err
}

func (s *Store) LoadTree() (Tree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var t Tree
	rows, err := s.db.Query(`SELECT id, name, sort_order, created_at, updated_at, meta_json
		FROM collections ORDER BY sort_order, id`)
	if err != nil {
		return t, err
	}
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.Name, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt, &c.MetaJSON); err != nil {
			rows.Close()
			return t, err
		}
		t.Collections = append(t.Collections, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return t, err
	}
	rows.Close()

	rows, err = s.db.Query(`SELECT id, collection_id, parent_folder_id, name, sort_order,
		created_at, updated_at, meta_json FROM folders ORDER BY sort_order, id`)
	if err != nil {
		return t, err
	}
	for rows.Next() {
		var f Folder
		var parent sql.NullString
		if err := rows.Scan(&f.ID, &f.CollectionID, &parent, &f.Name, &f.SortOrder,
			&f.CreatedAt, &f.UpdatedAt, &f.MetaJSON); err != nil {
			rows.Close()
			return t, err
		}
		f.ParentID = parent.String
		t.Folders = append(t.Folders, f)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return t, err
	}
	rows.Close()

	rows, err = s.db.Query(`SELECT id, collection_id, folder_id, name, method, url,
		headers_text, params_text, body_text, sort_order, created_at, updated_at, meta_json
		FROM requests ORDER BY sort_order, id`)
	if err != nil {
		return t, err
	}
	for rows.Next() {
		var r Request
		var folder sql.NullString
		if err := rows.Scan(&r.ID, &r.CollectionID, &folder, &r.Name, &r.Method, &r.URL,
			&r.HeadersText, &r.ParamsText, &r.BodyText, &r.SortOrder,
			&r.CreatedAt, &r.UpdatedAt, &r.MetaJSON); err != nil {
			rows.Close()
			return t, err
		}
		r.FolderID = folder.String
		t.Requests = append(t.Requests, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return t, err
	}
	rows.Close()
	return t, nil
}

func (s *Store) GetRequest(id string) (Request, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var r Request
	var folder sql.NullString
	err := s.db.QueryRow(`SELECT id, collection_id, folder_id, name, method, url,
		headers_text, params_text, body_text, sort_order, created_at, updated_at, meta_json
		FROM requests WHERE id = ?`, id).Scan(&r.ID, &r.CollectionID, &folder, &r.Name,
		&r.Method, &r.URL, &r.HeadersText, &r.ParamsText, &r.BodyText, &r.SortOrder,
		&r.CreatedAt, &r.UpdatedAt, &r.MetaJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return Request{}, false, nil
	}
	if err != nil {
		return Request{}, false, err
	}
	r.FolderID = folder.String
	return r, true, nil
}

func (s *Store) SaveRequest(r Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE requests SET name = ?, method = ?, url = ?, headers_text = ?,
		params_text = ?, body_text = ?, updated_at = ?, meta_json = ? WHERE id = ?`,
		r.Name, r.Method, r.URL, r.HeadersText, r.ParamsText, r.BodyText,
		time.Now().UnixMilli(), r.MetaJSON, r.ID)
	return err
}

func (s *Store) CreateRequest(collectionID, folderID, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		name = "新请求"
	}
	id := newID()
	now := time.Now().UnixMilli()
	var sort int
	var err error
	if folderID == "" {
		err = s.db.QueryRow(`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM requests
			WHERE collection_id = ? AND folder_id IS NULL`, collectionID).Scan(&sort)
	} else {
		err = s.db.QueryRow(`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM requests
			WHERE collection_id = ? AND folder_id = ?`, collectionID, folderID).Scan(&sort)
	}
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(`INSERT INTO requests(id, collection_id, folder_id, name, method, url,
		headers_text, params_text, body_text, sort_order, created_at, updated_at, meta_json)
		VALUES(?, ?, NULLIF(?, ''), ?, 'GET', 'https://httpbin.org/get', 'Accept: */*', '',
		'', ?, ?, ?, '{}')`, id, collectionID, folderID, name, sort, now, now)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) CreateCollection(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		name = "新集合"
	}
	id := newID()
	now := time.Now().UnixMilli()
	var sort int
	_ = s.db.QueryRow(`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM collections`).Scan(&sort)
	_, err := s.db.Exec(`INSERT INTO collections(id, name, sort_order, created_at, updated_at, meta_json)
		VALUES(?, ?, ?, ?, ?, '{}')`, id, name, sort, now, now)
	return id, err
}

func (s *Store) AddExchange(e Exchange) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO exchanges(id, request_id, saved_at, status, duration_ms,
		size_bytes, is_sse, ttft_ms, event_count, har_path)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET status = excluded.status, duration_ms = excluded.duration_ms,
		size_bytes = excluded.size_bytes, is_sse = excluded.is_sse, ttft_ms = excluded.ttft_ms,
		event_count = excluded.event_count, har_path = excluded.har_path`,
		e.ID, e.RequestID, e.SavedAt, e.Status, e.DurationMS, e.SizeBytes,
		boolInt(e.IsSSE), e.TTFTMS, e.EventCount, e.HARPath)
	return err
}

func (s *Store) ListExchanges(requestID string, limit int) ([]Exchange, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(`SELECT id, request_id, saved_at, status, duration_ms, size_bytes,
		is_sse, ttft_ms, event_count, har_path FROM exchanges WHERE request_id = ?
		ORDER BY saved_at DESC LIMIT ?`, requestID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Exchange
	for rows.Next() {
		var e Exchange
		var sse int
		if err := rows.Scan(&e.ID, &e.RequestID, &e.SavedAt, &e.Status, &e.DurationMS,
			&e.SizeBytes, &sse, &e.TTFTMS, &e.EventCount, &e.HARPath); err != nil {
			return nil, err
		}
		e.IsSSE = sse != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) LoadEnvironments() (EnvState, error) {
	var st EnvState
	b, err := os.ReadFile(s.envPath())
	if errors.Is(err, os.ErrNotExist) {
		return EnvState{Version: 1}, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return EnvState{Version: 1}, fmt.Errorf("store: environments: %w", err)
	}
	return st.Normalize(), nil
}

func (s *Store) SaveEnvironments(st EnvState) error {
	st = st.Normalize()
	b, err := json.MarshalIndent(st, "", "    ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.envPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.envPath(), b, 0o600)
}

func (s *Store) envPath() string {
	return filepath.Join(filepath.Dir(s.path), "environments.json")
}

func newID() string {
	// UUID v4 without a dependency on a generator package.
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		now := time.Now().UnixNano()
		return fmt.Sprintf("%d", now)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
