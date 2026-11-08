package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var ErrDestinationNotEmpty = errors.New("api-z store already has data")

type MigrateStats struct {
	Source       string
	AlreadyDone  bool
	Collections  int
	Folders      int
	Requests     int
	Artifacts    int
	Exchanges    int
	Environments int
}

// MigrateFromApiX copies api-x's read-only database, environments and
// response artifacts into a fresh api-z store. It never writes to src.
func MigrateFromApiX(dst *Store, paths Paths, srcDir string) (MigrateStats, error) {
	var stats MigrateStats
	stats.Source = srcDir
	if strings.TrimSpace(srcDir) == "" {
		return stats, errors.New("api-x data directory is empty")
	}
	dbPath := filepath.Join(srcDir, "collections.db")
	if _, err := os.Stat(dbPath); err != nil {
		return stats, fmt.Errorf("api-x collections.db: %w", err)
	}
	if dst.Meta("migrated_from") != "" {
		stats.AlreadyDone = true
		return stats, nil
	}
	n, err := dst.Count("collections")
	if err != nil {
		return stats, err
	}
	if n > 0 {
		return stats, ErrDestinationNotEmpty
	}

	// Work on a copy: opening a WAL database read-only still creates -shm
	// and -wal beside it on some systems. The api-x directory stays untouched.
	tmpDir, err := os.MkdirTemp("", "api-z-migrate-")
	if err != nil {
		return stats, err
	}
	defer os.RemoveAll(tmpDir)
	tmpDB := filepath.Join(tmpDir, "collections.db")
	if err := copyFile(dbPath, tmpDB); err != nil {
		return stats, err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(dbPath + suffix); err == nil {
			_ = copyFile(dbPath+suffix, tmpDB+suffix)
		}
	}
	src, err := sql.Open("sqlite", "file:"+filepath.ToSlash(tmpDB)+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return stats, err
	}
	defer src.Close()
	if err := src.Ping(); err != nil {
		return stats, fmt.Errorf("open api-x db read-only: %w", err)
	}

	tx, err := dst.db.Begin()
	if err != nil {
		return stats, err
	}
	defer func() { _ = tx.Rollback() }()
	// Folders can reference parents that come later in the source table;
	// check the whole graph at commit, not row by row.
	if _, err := tx.Exec(`PRAGMA defer_foreign_keys = ON`); err != nil {
		return stats, err
	}

	if err := copyCollections(src, tx, &stats); err != nil {
		return stats, err
	}
	if err := copyFolders(src, tx, &stats); err != nil {
		return stats, err
	}
	if err := copyRequests(src, tx, &stats); err != nil {
		return stats, err
	}
	if err := tx.Commit(); err != nil {
		return stats, err
	}

	// Environments: copy the file as-is so tags and ordering survive.
	srcEnv := filepath.Join(srcDir, "environments.json")
	if b, err := os.ReadFile(srcEnv); err == nil {
		if _, err := os.Stat(paths.EnvPath); errors.Is(err, os.ErrNotExist) {
			if err := os.MkdirAll(filepath.Dir(paths.EnvPath), 0o755); err == nil {
				if err := os.WriteFile(paths.EnvPath, b, 0o600); err == nil {
					var st EnvState
					if json.Unmarshal(b, &st) == nil {
						stats.Environments = len(st.Environments)
					}
				}
			}
		}
	}

	stats.Artifacts, _ = copyArtifacts(filepath.Join(srcDir, "request"), paths.Artifacts)
	stats.Exchanges = backfillExchanges(dst, paths)

	if err := dst.SetMeta("migrated_from", srcDir); err != nil {
		return stats, err
	}
	_ = dst.SetMeta("migrated_at", time.Now().Format(time.RFC3339))
	_ = dst.SetMeta("api_z_schema", "1")
	LogInfo("migrate", "api-x data migrated", "source", srcDir,
		"collections", stats.Collections, "folders", stats.Folders,
		"requests", stats.Requests, "artifacts", stats.Artifacts, "exchanges", stats.Exchanges)
	return stats, nil
}

func copyCollections(src *sql.DB, tx *sql.Tx, stats *MigrateStats) error {
	rows, err := src.Query(`SELECT id, name, sort_order, created_at, updated_at, COALESCE(meta_json,'{}') FROM collections`)
	if err != nil {
		return fmt.Errorf("read api-x collections: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, meta string
		var sortOrder int
		var created, updated int64
		if err := rows.Scan(&id, &name, &sortOrder, &created, &updated, &meta); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO collections(id, name, sort_order, created_at, updated_at, meta_json)
			VALUES(?, ?, ?, ?, ?, ?)`, id, name, sortOrder, created, updated, meta); err != nil {
			return err
		}
		stats.Collections++
	}
	return rows.Err()
}

func copyFolders(src *sql.DB, tx *sql.Tx, stats *MigrateStats) error {
	rows, err := src.Query(`SELECT id, collection_id, parent_folder_id, name, sort_order,
		created_at, updated_at, COALESCE(meta_json,'{}') FROM folders`)
	if err != nil {
		return fmt.Errorf("read api-x folders: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, collectionID, name, meta string
		var parent sql.NullString
		var sortOrder int
		var created, updated int64
		if err := rows.Scan(&id, &collectionID, &parent, &name, &sortOrder, &created, &updated, &meta); err != nil {
			return err
		}
		var parentArg any
		if parent.Valid && parent.String != "" {
			parentArg = parent.String
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO folders(id, collection_id, parent_folder_id, name,
			sort_order, created_at, updated_at, meta_json) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
			id, collectionID, parentArg, name, sortOrder, created, updated, meta); err != nil {
			return err
		}
		stats.Folders++
	}
	return rows.Err()
}

func copyRequests(src *sql.DB, tx *sql.Tx, stats *MigrateStats) error {
	paramsExpr := "''"
	if hasColumn(src, "requests", "params_text") {
		paramsExpr = "COALESCE(params_text,'')"
	}
	q := `SELECT id, collection_id, folder_id, name, method, url,
		COALESCE(headers_text,''), ` + paramsExpr + `, COALESCE(body_text,''),
		sort_order, created_at, updated_at, COALESCE(meta_json,'{}') FROM requests`
	rows, err := src.Query(q)
	if err != nil {
		return fmt.Errorf("read api-x requests: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, collectionID, name, method, url, headers, params, body, meta string
		var folder sql.NullString
		var sortOrder int
		var created, updated int64
		if err := rows.Scan(&id, &collectionID, &folder, &name, &method, &url,
			&headers, &params, &body, &sortOrder, &created, &updated, &meta); err != nil {
			return err
		}
		var folderArg any
		if folder.Valid && folder.String != "" {
			folderArg = folder.String
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO requests(id, collection_id, folder_id, name,
			method, url, headers_text, params_text, body_text, sort_order, created_at,
			updated_at, meta_json) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, collectionID, folderArg, name, method, url, headers, params, body,
			sortOrder, created, updated, meta); err != nil {
			return err
		}
		stats.Requests++
	}
	return rows.Err()
}

func hasColumn(db *sql.DB, table, column string) bool {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt sql.NullString
		if rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk) == nil && name == column {
			return true
		}
	}
	return false
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, cerr := io.Copy(out, in)
	_ = out.Close()
	return cerr
}

func copyArtifacts(src, dst string) (int, error) {
	n := 0
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil || rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if _, err := os.Stat(target); err == nil {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return nil
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			_ = in.Close()
			return nil
		}
		_, cerr := io.Copy(out, in)
		_ = out.Close()
		_ = in.Close()
		if cerr == nil {
			n++
		}
		return nil
	})
	return n, err
}

func backfillExchanges(dst *Store, paths Paths) int {
	n := 0
	_ = filepath.WalkDir(paths.Artifacts, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".har") {
			return nil
		}
		rel, err := filepath.Rel(paths.Artifacts, path)
		if err != nil {
			return nil
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) < 3 || parts[1] != "response" {
			return nil
		}
		requestID := parts[0]
		stem := strings.TrimSuffix(parts[len(parts)-1], filepath.Ext(parts[len(parts)-1]))
		var doc struct {
			Log struct {
				APIX struct {
					SavedAtEpochMS        int64  `json:"savedAtEpochMs"`
					IsSSEResponse         bool   `json:"isSseResponse"`
					ResponseSSEEventCount string `json:"responseSseEventCountText"`
				} `json:"_apiX"`
				Entries []struct {
					Time     int64 `json:"time"`
					Response struct {
						Status   int   `json:"status"`
						BodySize int64 `json:"bodySize"`
						Content  struct {
							Size int64 `json:"size"`
						} `json:"content"`
					} `json:"response"`
				} `json:"entries"`
			} `json:"log"`
		}
		if b, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(b, &doc)
		}
		e := Exchange{
			ID:        requestID + "-" + stem,
			RequestID: requestID,
			HARPath:   path,
			IsSSE:     doc.Log.APIX.IsSSEResponse,
		}
		if doc.Log.APIX.SavedAtEpochMS > 0 {
			e.SavedAt = doc.Log.APIX.SavedAtEpochMS
		} else if ms, err := strconv.ParseInt(stem, 10, 64); err == nil {
			e.SavedAt = ms
		}
		if len(doc.Log.Entries) > 0 {
			e.Status = doc.Log.Entries[0].Response.Status
			e.DurationMS = doc.Log.Entries[0].Time
			e.SizeBytes = doc.Log.Entries[0].Response.BodySize
			if e.SizeBytes == 0 {
				e.SizeBytes = doc.Log.Entries[0].Response.Content.Size
			}
		}
		e.EventCount = parseEventText(doc.Log.APIX.ResponseSSEEventCount)
		if e.SavedAt == 0 {
			if info, err := d.Info(); err == nil {
				e.SavedAt = info.ModTime().UnixMilli()
			}
		}
		if err := dst.AddExchange(e); err == nil {
			n++
		}
		return nil
	})
	return n
}

func parseEventText(s string) int {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "Event ")
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
