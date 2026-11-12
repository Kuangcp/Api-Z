package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Data sync mirrors api-x's git-friendly data directory:
//
//	<data>/api-z/data/collection/<collection-id>.json   Postman v2.1
//	<data>/api-z/data/env/environments.json
//
// Push writes every collection; Pull upserts by collection id from the file
// name and merges environments by id without deleting local entries.

type DataPushResult struct {
	Collections int
	EnvWritten  bool
}

type DataPullResult struct {
	Merged    int
	Created   int
	EnvMerged bool
	FileErrs  []string
}

func DataRoot(paths Paths) string { return filepath.Join(paths.DataDir, "data") }

func DataCollectionDir(paths Paths) string {
	return filepath.Join(DataRoot(paths), "collection")
}

func DataEnvFile(paths Paths) string {
	return filepath.Join(DataRoot(paths), "env", "environments.json")
}

// PushDataDir exports all collections and the environment file under the
// api-z data directory. Existing files not belonging to a collection are
// left alone, and no data is deleted.
func (s *Store) PushDataDir(paths Paths) (DataPushResult, error) {
	var out DataPushResult
	collDir := DataCollectionDir(paths)
	if err := os.MkdirAll(collDir, 0o755); err != nil {
		return out, err
	}
	tree, err := s.LoadTree()
	if err != nil {
		return out, err
	}
	for _, col := range tree.Collections {
		portable, err := s.ExportPortableCollection(col.ID)
		if err != nil {
			return out, err
		}
		data, err := PostmanJSON(portable)
		if err != nil {
			return out, err
		}
		path := filepath.Join(collDir, col.ID+".json")
		if err := writeFileAtomic(path, data); err != nil {
			return out, err
		}
		out.Collections++
	}
	envPath := DataEnvFile(paths)
	if err := os.MkdirAll(filepath.Dir(envPath), 0o755); err != nil {
		return out, err
	}
	envBytes, err := os.ReadFile(paths.EnvPath)
	if errors.Is(err, os.ErrNotExist) {
		envBytes = []byte("{\n  \"version\": 1,\n  \"environments\": []\n}\n")
	} else if err != nil {
		return out, err
	}
	if err := writeFileAtomic(envPath, envBytes); err != nil {
		return out, err
	}
	out.EnvWritten = true
	return out, nil
}

// PullDataDir creates missing collections and merges existing ones by id.
// Files that fail to parse are reported without stopping the rest.
func (s *Store) PullDataDir(paths Paths) (DataPullResult, error) {
	var out DataPullResult
	entries, err := os.ReadDir(DataCollectionDir(paths))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return out, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		id := strings.TrimSuffix(name, filepath.Ext(name))
		if strings.TrimSpace(id) == "" {
			continue
		}
		path := filepath.Join(DataCollectionDir(paths), name)
		data, err := os.ReadFile(path)
		if err != nil {
			out.FileErrs = append(out.FileErrs, name+": "+err.Error())
			continue
		}
		portable, err := ParsePostmanCollection(data)
		if err != nil {
			out.FileErrs = append(out.FileErrs, name+": "+err.Error())
			continue
		}
		exists, err := s.CollectionExists(id)
		if err != nil {
			out.FileErrs = append(out.FileErrs, name+": "+err.Error())
			continue
		}
		if exists {
			if err := s.MergePortableCollection(id, portable); err != nil {
				out.FileErrs = append(out.FileErrs, name+": "+err.Error())
				continue
			}
			out.Merged++
		} else {
			if err := s.ImportPortableCollectionWithID(id, portable); err != nil {
				out.FileErrs = append(out.FileErrs, name+": "+err.Error())
				continue
			}
			out.Created++
		}
	}

	envData, err := os.ReadFile(DataEnvFile(paths))
	if err == nil {
		var incoming EnvState
		if err := json.Unmarshal(envData, &incoming); err != nil {
			out.FileErrs = append(out.FileErrs, "env/environments.json: "+err.Error())
		} else {
			current := EnvState{Version: 1}
			if b, err := os.ReadFile(paths.EnvPath); err == nil {
				_ = json.Unmarshal(b, &current)
			}
			merged := mergeEnvStates(current, incoming)
			if err := os.MkdirAll(filepath.Dir(paths.EnvPath), 0o755); err != nil {
				return out, err
			}
			if err := writeJSON(paths.EnvPath, merged); err != nil {
				return out, err
			}
			out.EnvMerged = true
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return out, err
	}
	return out, nil
}

func mergeEnvStates(base, overlay EnvState) EnvState {
	out := base.Clone()
	if out.Version == 0 {
		out.Version = 1
	}
	index := make(map[string]int, len(out.Environments))
	for i, env := range out.Environments {
		index[env.ID] = i
	}
	for _, env := range overlay.Environments {
		if i, ok := index[env.ID]; ok {
			out.Environments[i] = env
			continue
		}
		index[env.ID] = len(out.Environments)
		out.Environments = append(out.Environments, env)
	}
	if out.ActiveEnvironmentID == "" {
		out.ActiveEnvironmentID = overlay.ActiveEnvironmentID
	}
	return out
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// CollectionExists reports whether a collection id is present.
func (s *Store) CollectionExists(id string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM collections WHERE id = ?`, id).Scan(&n)
	return n > 0, err
}

// ImportPortableCollectionWithID creates a collection with the given id,
// keeping child ids (from _api_x_id) when they are free, so later pulls merge.
func (s *Store) ImportPortableCollectionWithID(id string, p PortableCollection) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("集合 id 不能为空")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UnixMilli()
	var sortOrder int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM collections`).Scan(&sortOrder); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO collections(id, name, sort_order, created_at, updated_at, meta_json)
		VALUES(?, ?, ?, ?, ?, ?)`, id, p.Name, sortOrder, now, now, portableMeta(p.MetaJSON, "", p.Auth)); err != nil {
		return err
	}
	if err := insertPortableFoldersWithIDs(tx, id, "", p.Folders, now); err != nil {
		return err
	}
	if err := insertPortableRequestsWithIDs(tx, id, "", p.Requests, now); err != nil {
		return err
	}
	return tx.Commit()
}

// MergePortableCollection upserts a pulled collection by id: collection,
// folder and request rows with known ids are updated; unknown ones are
// inserted; nothing is deleted.
func (s *Store) MergePortableCollection(id string, p PortableCollection) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var existingMeta string
	if err := tx.QueryRow(`SELECT meta_json FROM collections WHERE id = ?`, id).Scan(&existingMeta); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("集合不存在: " + id)
		}
		return err
	}
	now := time.Now().UnixMilli()
	meta := portableMeta(p.MetaJSON, existingMeta, p.Auth)
	if _, err := tx.Exec(`UPDATE collections SET name = ?, meta_json = ?, updated_at = ? WHERE id = ?`,
		p.Name, meta, now, id); err != nil {
		return err
	}
	if err := mergePortableFolders(tx, id, "", p.Folders, now); err != nil {
		return err
	}
	if err := mergePortableRequests(tx, id, "", p.Requests, now); err != nil {
		return err
	}
	return tx.Commit()
}

func portableMeta(incoming, existing string, auth *Auth) string {
	out := incoming
	if strings.TrimSpace(out) == "" {
		out = existing
	}
	if auth != nil {
		return MergeAuth(out, *auth)
	}
	var local struct {
		Auth *Auth `json:"auth"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(existing)), &local) == nil && local.Auth != nil {
		return MergeAuth(out, *local.Auth)
	}
	return out
}

func insertPortableFoldersWithIDs(tx *sql.Tx, collectionID, parentID string, folders []PortableFolder, now int64) error {
	for i, f := range folders {
		folderID := f.ID
		if folderID == "" || tableHasID(tx, "folders", folderID) {
			folderID = newID()
		}
		sortOrder := f.SortOrder
		if sortOrder == 0 {
			sortOrder = i
		}
		if _, err := tx.Exec(`INSERT INTO folders(id, collection_id, parent_folder_id, name,
			sort_order, created_at, updated_at, meta_json)
			VALUES(?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)`,
			folderID, collectionID, parentID, f.Name, sortOrder, now, now,
			portableMeta(f.MetaJSON, "", f.Auth)); err != nil {
			return err
		}
		if err := insertPortableFoldersWithIDs(tx, collectionID, folderID, f.Folders, now); err != nil {
			return err
		}
		if err := insertPortableRequestsWithIDs(tx, collectionID, folderID, f.Requests, now); err != nil {
			return err
		}
	}
	return nil
}

func insertPortableRequestsWithIDs(tx *sql.Tx, collectionID, folderID string, requests []PortableRequest, now int64) error {
	for i, r := range requests {
		reqID := r.ID
		if reqID == "" || tableHasID(tx, "requests", reqID) {
			reqID = newID()
		}
		sortOrder := r.SortOrder
		if sortOrder == 0 {
			sortOrder = i
		}
		if _, err := tx.Exec(`INSERT INTO requests(id, collection_id, folder_id, name, method, url,
			headers_text, params_text, body_text, sort_order, created_at, updated_at, meta_json)
			VALUES(?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			reqID, collectionID, folderID, r.Name, r.Method, r.URL,
			r.HeadersText, r.ParamsText, r.BodyText, sortOrder, now, now,
			portableMeta(r.MetaJSON, "", r.Auth)); err != nil {
			return err
		}
	}
	return nil
}

func mergePortableFolders(tx *sql.Tx, collectionID, parentID string, folders []PortableFolder, now int64) error {
	for i, f := range folders {
		folderID := ""
		if f.ID != "" {
			var owner string
			err := tx.QueryRow(`SELECT collection_id FROM folders WHERE id = ?`, f.ID).Scan(&owner)
			switch {
			case err == nil && owner == collectionID:
				folderID = f.ID
				var existingMeta string
				_ = tx.QueryRow(`SELECT meta_json FROM folders WHERE id = ?`, f.ID).Scan(&existingMeta)
				sortOrder := f.SortOrder
				if sortOrder == 0 {
					sortOrder = i
				}
				if _, err := tx.Exec(`UPDATE folders SET name = ?, parent_folder_id = NULLIF(?, ''),
					sort_order = ?, updated_at = ?, meta_json = ? WHERE id = ?`,
					f.Name, parentID, sortOrder, now, portableMeta(f.MetaJSON, existingMeta, f.Auth), f.ID); err != nil {
					return err
				}
			case err == nil:
				// Owned by another collection: insert a fresh row instead of
				// moving it out from under its owner.
			case errors.Is(err, sql.ErrNoRows):
				folderID = f.ID
				sortOrder := f.SortOrder
				if sortOrder == 0 {
					sortOrder = i
				}
				if _, err := tx.Exec(`INSERT INTO folders(id, collection_id, parent_folder_id, name,
					sort_order, created_at, updated_at, meta_json)
					VALUES(?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)`,
					folderID, collectionID, parentID, f.Name, sortOrder, now, now,
					portableMeta(f.MetaJSON, "", f.Auth)); err != nil {
					return err
				}
			default:
				return err
			}
		}
		if folderID == "" {
			folderID = newID()
			sortOrder := f.SortOrder
			if sortOrder == 0 {
				sortOrder = i
			}
			if _, err := tx.Exec(`INSERT INTO folders(id, collection_id, parent_folder_id, name,
				sort_order, created_at, updated_at, meta_json)
				VALUES(?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)`,
				folderID, collectionID, parentID, f.Name, sortOrder, now, now,
				portableMeta(f.MetaJSON, "", f.Auth)); err != nil {
				return err
			}
		}
		if err := mergePortableFolders(tx, collectionID, folderID, f.Folders, now); err != nil {
			return err
		}
		if err := mergePortableRequests(tx, collectionID, folderID, f.Requests, now); err != nil {
			return err
		}
	}
	return nil
}

func mergePortableRequests(tx *sql.Tx, collectionID, folderID string, requests []PortableRequest, now int64) error {
	for i, r := range requests {
		reqID := ""
		if r.ID != "" {
			var owner string
			err := tx.QueryRow(`SELECT collection_id FROM requests WHERE id = ?`, r.ID).Scan(&owner)
			if err == nil && owner == collectionID {
				reqID = r.ID
			} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		sortOrder := r.SortOrder
		if sortOrder == 0 {
			sortOrder = i
		}
		if reqID != "" {
			var existingMeta string
			_ = tx.QueryRow(`SELECT meta_json FROM requests WHERE id = ?`, reqID).Scan(&existingMeta)
			if _, err := tx.Exec(`UPDATE requests SET folder_id = NULLIF(?, ''), name = ?, method = ?, url = ?,
				headers_text = ?, params_text = ?, body_text = ?, sort_order = ?, updated_at = ?, meta_json = ?
				WHERE id = ?`,
				folderID, r.Name, r.Method, r.URL, r.HeadersText, r.ParamsText, r.BodyText,
				sortOrder, now, portableMeta(r.MetaJSON, existingMeta, r.Auth), reqID); err != nil {
				return err
			}
			continue
		}
		if r.ID != "" && !tableHasID(tx, "requests", r.ID) {
			reqID = r.ID
		} else {
			reqID = newID()
		}
		if _, err := tx.Exec(`INSERT INTO requests(id, collection_id, folder_id, name, method, url,
			headers_text, params_text, body_text, sort_order, created_at, updated_at, meta_json)
			VALUES(?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			reqID, collectionID, folderID, r.Name, r.Method, r.URL,
			r.HeadersText, r.ParamsText, r.BodyText, sortOrder, now, now,
			portableMeta(r.MetaJSON, "", r.Auth)); err != nil {
			return err
		}
	}
	return nil
}

func tableHasID(tx *sql.Tx, table, id string) bool {
	var one int
	return tx.QueryRow(`SELECT 1 FROM `+table+` WHERE id = ?`, id).Scan(&one) == nil
}
