package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type OpenAPIMergeStats struct {
	Added     int
	Updated   int
	Folders   int
	Unchanged int
}

// MergeOpenAPICollection upserts an OpenAPI tree into an existing collection.
// Existing requests keep their local URL/headers/params/body/auth; names,
// methods and OpenAPI metadata are refreshed. Removed endpoints are not
// deleted, matching api-x.
func (s *Store) MergeOpenAPICollection(collectionID string, p PortableCollection) (OpenAPIMergeStats, error) {
	var stats OpenAPIMergeStats
	tx, err := s.db.Begin()
	if err != nil {
		return stats, err
	}
	defer func() { _ = tx.Rollback() }()

	var oldName, oldMeta string
	if err := tx.QueryRow(`SELECT name, meta_json FROM collections WHERE id = ?`, collectionID).
		Scan(&oldName, &oldMeta); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return stats, errors.New("集合不存在")
		}
		return stats, err
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = oldName
	}
	newMeta := mergeOpenAPICollectionMeta(oldMeta, p.MetaJSON)
	if _, err := tx.Exec(`UPDATE collections SET name = ?, meta_json = ?, updated_at = ? WHERE id = ?`,
		name, newMeta, time.Now().UnixMilli(), collectionID); err != nil {
		return stats, err
	}

	existing := map[string]openAPIExistingRequest{}
	rows, err := tx.Query(`SELECT id, folder_id, url, headers_text, params_text, body_text, meta_json
		FROM requests WHERE collection_id = ?`, collectionID)
	if err != nil {
		return stats, err
	}
	for rows.Next() {
		var r openAPIExistingRequest
		var folder sql.NullString
		if err := rows.Scan(&r.ID, &folder, &r.URL, &r.Headers, &r.Params, &r.Body, &r.Meta); err != nil {
			rows.Close()
			return stats, err
		}
		r.FolderID = folder.String
		existing[r.ID] = r
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return stats, err
	}
	byKey := map[string]string{}
	for id, r := range existing {
		for _, key := range openAPIRequestKeys(r.Meta) {
			if _, ok := byKey[key]; !ok {
				byKey[key] = id
			}
		}
	}

	now := time.Now().UnixMilli()
	for _, folder := range p.Folders {
		folderID, err := mergeOpenAPIFolder(tx, collectionID, "", folder, now)
		if err != nil {
			return stats, err
		}
		stats.Folders++
		subFolders, subRequests := folder.Folders, folder.Requests
		if err := mergeOpenAPISubtree(tx, collectionID, folderID, subFolders, subRequests, existing, byKey, now, &stats); err != nil {
			return stats, err
		}
	}
	if err := tx.Commit(); err != nil {
		return stats, err
	}
	return stats, nil
}

type openAPIExistingRequest struct {
	ID       string
	FolderID string
	URL      string
	Headers  string
	Params   string
	Body     string
	Meta     string
}

func mergeOpenAPISubtree(tx *sql.Tx, collectionID, parentID string, folders []PortableFolder,
	requests []PortableRequest, existing map[string]openAPIExistingRequest, byKey map[string]string,
	now int64, stats *OpenAPIMergeStats) error {
	for _, folder := range folders {
		folderID, err := mergeOpenAPIFolder(tx, collectionID, parentID, folder, now)
		if err != nil {
			return err
		}
		stats.Folders++
		if err := mergeOpenAPISubtree(tx, collectionID, folderID, folder.Folders, folder.Requests,
			existing, byKey, now, stats); err != nil {
			return err
		}
	}
	for _, req := range requests {
		matchedID := req.ID
		if matchedID != "" {
			if _, ok := existing[matchedID]; !ok {
				matchedID = ""
			}
		}
		if matchedID == "" {
			for _, key := range openAPIRequestKeys(req.MetaJSON) {
				if id, ok := byKey[key]; ok {
					matchedID = id
					break
				}
			}
		}
		if matchedID == "" {
			id := req.ID
			if id == "" {
				id = newID()
			}
			meta := req.MetaJSON
			if req.Auth != nil {
				meta = MergeAuth(meta, *req.Auth)
			}
			if _, err := tx.Exec(`INSERT INTO requests(id, collection_id, folder_id, name, method, url,
				headers_text, params_text, body_text, sort_order, created_at, updated_at, meta_json)
				VALUES(?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, collectionID, parentID, req.Name, req.Method, req.URL,
				req.HeadersText, req.ParamsText, req.BodyText, req.SortOrder, now, now, meta); err != nil {
				return err
			}
			stats.Added++
			continue
		}
		old := existing[matchedID]
		meta := mergeOpenAPIRequestMeta(old.Meta, req.MetaJSON)
		if _, err := tx.Exec(`UPDATE requests SET collection_id = ?, folder_id = NULLIF(?, ''),
			name = ?, method = ?, sort_order = ?, updated_at = ?, meta_json = ?
			WHERE id = ?`,
			collectionID, parentID, req.Name, req.Method, req.SortOrder, now, meta, matchedID); err != nil {
			return err
		}
		stats.Updated++
	}
	return nil
}

func mergeOpenAPIFolder(tx *sql.Tx, collectionID, parentID string, folder PortableFolder, now int64) (string, error) {
	folderID := strings.TrimSpace(folder.ID)
	if folderID != "" {
		var owner string
		var oldMeta string
		err := tx.QueryRow(`SELECT collection_id, meta_json FROM folders WHERE id = ?`, folderID).
			Scan(&owner, &oldMeta)
		switch {
		case err == nil && owner == collectionID:
			meta := mergeOpenAPIRequestMeta(oldMeta, folder.MetaJSON)
			if _, err := tx.Exec(`UPDATE folders SET name = ?, parent_folder_id = NULLIF(?, ''),
				sort_order = ?, updated_at = ?, meta_json = ? WHERE id = ?`,
				folder.Name, parentID, folder.SortOrder, now, meta, folderID); err != nil {
				return "", err
			}
			return folderID, nil
		case err == nil:
			folderID = "" // id belongs to another collection
		case !errors.Is(err, sql.ErrNoRows):
			return "", err
		}
	}
	if folderID == "" {
		folderID = newID()
	}
	meta := folder.MetaJSON
	if folder.Auth != nil {
		meta = MergeAuth(meta, *folder.Auth)
	}
	if _, err := tx.Exec(`INSERT INTO folders(id, collection_id, parent_folder_id, name,
		sort_order, created_at, updated_at, meta_json) VALUES(?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)`,
		folderID, collectionID, parentID, folder.Name, folder.SortOrder, now, now, meta); err != nil {
		return "", err
	}
	return folderID, nil
}

func openAPIRequestKeys(metaJSON string) []string {
	var meta struct {
		OpenAPI struct {
			SyncKey     string `json:"syncKey"`
			Method      string `json:"method"`
			Path        string `json:"path"`
			OperationID string `json:"operationId"`
		} `json:"openapi"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(metaJSON)), &meta) != nil {
		return nil
	}
	var keys []string
	if meta.OpenAPI.SyncKey != "" {
		keys = append(keys, "sync:"+meta.OpenAPI.SyncKey)
	}
	if meta.OpenAPI.Method != "" && meta.OpenAPI.Path != "" {
		keys = append(keys, "sync:"+openAPISyncKey(meta.OpenAPI.Method, meta.OpenAPI.Path))
		keys = append(keys, "raw:"+strings.ToUpper(meta.OpenAPI.Method)+" "+meta.OpenAPI.Path)
	}
	if meta.OpenAPI.OperationID != "" {
		keys = append(keys, "operation:"+meta.OpenAPI.OperationID)
	}
	return keys
}

// CreateOpenAPICollection creates an empty collection, merges a parsed
// OpenAPI tree, and rolls the collection back if merging fails.
func (s *Store) CreateOpenAPICollection(name string, p PortableCollection) (string, OpenAPIMergeStats, error) {
	id, err := s.CreateCollection(name)
	if err != nil {
		return "", OpenAPIMergeStats{}, err
	}
	stats, err := s.MergeOpenAPICollection(id, p)
	if err != nil {
		_ = s.DeleteCollection(id)
		return "", OpenAPIMergeStats{}, err
	}
	return id, stats, nil
}

// RefreshOpenAPICollection fetches sourceURL and merges the parsed tree,
// preserving local request edits.
func (s *Store) RefreshOpenAPICollection(ctx context.Context, collectionID string) (OpenAPIMergeStats, error) {
	var name, meta string
	if err := s.db.QueryRow(`SELECT name, meta_json FROM collections WHERE id = ?`, collectionID).
		Scan(&name, &meta); err != nil {
		return OpenAPIMergeStats{}, err
	}
	sourceURL, root := CollectionOpenAPIInfo(meta)
	if sourceURL == "" {
		return OpenAPIMergeStats{}, fmt.Errorf("当前集合没有绑定 OpenAPI 地址")
	}
	body, err := FetchOpenAPI(ctx, sourceURL)
	if err != nil {
		return OpenAPIMergeStats{}, err
	}
	parsed, err := ParseOpenAPI(string(body), sourceURL, name, collectionID, root)
	if err != nil {
		return OpenAPIMergeStats{}, err
	}
	return s.MergeOpenAPICollection(collectionID, parsed.Portable)
}
