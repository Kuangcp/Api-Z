package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type OpenAPIMergeStats struct {
	Added     int
	Updated   int
	Folders   int
	Unchanged int
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

type openAPIMergeState struct {
	collectionID string
	now          int64
	stats        *OpenAPIMergeStats

	requests       map[string]openAPIExistingRequest
	requestKeys    map[string][]string
	matchedRequest map[string]bool
	usedRequestIDs map[string]bool

	folderKeys    map[string][]string
	matchedFolder map[string]bool
	usedFolderIDs map[string]bool
}

// MergeOpenAPICollection upserts an OpenAPI tree into an existing collection.
// Existing requests keep their local URL/headers/params/body/auth; names,
// methods and OpenAPI metadata are refreshed. Removed endpoints are not
// deleted, matching api-x.
func (s *Store) MergeOpenAPICollection(collectionID string, p PortableCollection) (OpenAPIMergeStats, error) {
	var stats OpenAPIMergeStats
	s.mu.Lock()
	defer s.mu.Unlock()

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

	st, err := newOpenAPIMergeState(tx, collectionID)
	if err != nil {
		return stats, err
	}
	st.stats = &stats

	for _, folder := range p.Folders {
		folderID, err := mergeOpenAPIFolder(tx, "", folder, st)
		if err != nil {
			return stats, err
		}
		stats.Folders++
		if err := mergeOpenAPISubtree(tx, folderID, folder.Folders, folder.Requests, st); err != nil {
			return stats, err
		}
	}
	if err := tx.Commit(); err != nil {
		return stats, err
	}
	return stats, nil
}

func newOpenAPIMergeState(tx *sql.Tx, collectionID string) (*openAPIMergeState, error) {
	st := &openAPIMergeState{
		collectionID:   collectionID,
		now:            time.Now().UnixMilli(),
		requests:       map[string]openAPIExistingRequest{},
		requestKeys:    map[string][]string{},
		matchedRequest: map[string]bool{},
		usedRequestIDs: map[string]bool{},
		folderKeys:     map[string][]string{},
		matchedFolder:  map[string]bool{},
		usedFolderIDs:  map[string]bool{},
	}

	rows, err := tx.Query(`SELECT id, folder_id, url, headers_text, params_text, body_text, meta_json
		FROM requests WHERE collection_id = ?`, collectionID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r openAPIExistingRequest
		var folder sql.NullString
		if err := rows.Scan(&r.ID, &folder, &r.URL, &r.Headers, &r.Params, &r.Body, &r.Meta); err != nil {
			rows.Close()
			return nil, err
		}
		r.FolderID = folder.String
		st.requests[r.ID] = r
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(st.requests))
	for id := range st.requests {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		seen := map[string]bool{}
		for _, key := range openAPIRequestKeys(st.requests[id].Meta) {
			if seen[key] {
				continue
			}
			seen[key] = true
			st.requestKeys[key] = append(st.requestKeys[key], id)
		}
	}

	rows, err = tx.Query(`SELECT id FROM requests`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		st.usedRequestIDs[id] = true
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	folderRows, err := tx.Query(`SELECT id, meta_json FROM folders WHERE collection_id = ?`, collectionID)
	if err != nil {
		return nil, err
	}
	var folderIDs []string
	folderMeta := map[string]string{}
	for folderRows.Next() {
		var id, meta string
		if err := folderRows.Scan(&id, &meta); err != nil {
			folderRows.Close()
			return nil, err
		}
		folderIDs = append(folderIDs, id)
		folderMeta[id] = meta
	}
	if err := folderRows.Close(); err != nil {
		return nil, err
	}
	if err := folderRows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(folderIDs)
	for _, id := range folderIDs {
		seen := map[string]bool{}
		for _, key := range openAPIFolderKeys(folderMeta[id]) {
			if seen[key] {
				continue
			}
			seen[key] = true
			st.folderKeys[key] = append(st.folderKeys[key], id)
		}
	}

	rows, err = tx.Query(`SELECT id FROM folders`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		st.usedFolderIDs[id] = true
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return st, nil
}

func mergeOpenAPISubtree(tx *sql.Tx, parentID string, folders []PortableFolder,
	requests []PortableRequest, st *openAPIMergeState) error {
	for _, folder := range folders {
		folderID, err := mergeOpenAPIFolder(tx, parentID, folder, st)
		if err != nil {
			return err
		}
		st.stats.Folders++
		if err := mergeOpenAPISubtree(tx, folderID, folder.Folders, folder.Requests, st); err != nil {
			return err
		}
	}
	for _, req := range requests {
		if err := mergeOpenAPIRequest(tx, parentID, req, st); err != nil {
			return err
		}
	}
	return nil
}

func mergeOpenAPIRequest(tx *sql.Tx, parentID string, req PortableRequest, st *openAPIMergeState) error {
	matchedID := st.matchRequest(req)
	if matchedID == "" {
		id := strings.TrimSpace(req.ID)
		if id == "" || st.usedRequestIDs[id] || st.matchedRequest[id] {
			id = st.newRequestID()
		}
		st.usedRequestIDs[id] = true
		meta := req.MetaJSON
		if req.Auth != nil {
			meta = MergeAuth(meta, *req.Auth)
		}
		if _, err := tx.Exec(`INSERT INTO requests(id, collection_id, folder_id, name, method, url,
			headers_text, params_text, body_text, sort_order, created_at, updated_at, meta_json)
			VALUES(?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, st.collectionID, parentID, req.Name, req.Method, req.URL,
			req.HeadersText, req.ParamsText, req.BodyText, req.SortOrder, st.now, st.now, meta); err != nil {
			return err
		}
		st.stats.Added++
		return nil
	}

	old := st.requests[matchedID]
	meta := MergeOpenAPIRequestMeta(old.Meta, req.MetaJSON)
	if _, err := tx.Exec(`UPDATE requests SET collection_id = ?, folder_id = NULLIF(?, ''),
		name = ?, method = ?, sort_order = ?, updated_at = ?, meta_json = ?
		WHERE id = ?`,
		st.collectionID, parentID, req.Name, req.Method, req.SortOrder, st.now, meta, matchedID); err != nil {
		return err
	}
	st.matchedRequest[matchedID] = true
	st.stats.Updated++
	return nil
}

func (st *openAPIMergeState) matchRequest(req PortableRequest) string {
	if id := strings.TrimSpace(req.ID); id != "" {
		if _, ok := st.requests[id]; ok && !st.matchedRequest[id] {
			return id
		}
	}
	for _, key := range openAPIRequestKeys(req.MetaJSON) {
		for _, id := range st.requestKeys[key] {
			if !st.matchedRequest[id] {
				return id
			}
		}
	}
	return ""
}

func (st *openAPIMergeState) newRequestID() string {
	for {
		id := newID()
		if !st.usedRequestIDs[id] {
			return id
		}
	}
}

func mergeOpenAPIFolder(tx *sql.Tx, parentID string, folder PortableFolder, st *openAPIMergeState) (string, error) {
	incomingID := strings.TrimSpace(folder.ID)
	if incomingID != "" {
		var owner, oldMeta string
		err := tx.QueryRow(`SELECT collection_id, meta_json FROM folders WHERE id = ?`, incomingID).
			Scan(&owner, &oldMeta)
		switch {
		case err == nil && owner == st.collectionID:
			return st.updateOpenAPIFolder(tx, incomingID, parentID, folder, oldMeta)
		case err == nil:
			// The deterministic ID now belongs to another collection. Reattach
			// by the folder's OpenAPI key below.
		case !errors.Is(err, sql.ErrNoRows):
			return "", err
		}
	}

	if matchedID := st.matchFolder(folder); matchedID != "" {
		var oldMeta string
		if err := tx.QueryRow(`SELECT meta_json FROM folders WHERE id = ?`, matchedID).Scan(&oldMeta); err != nil {
			return "", err
		}
		return st.updateOpenAPIFolder(tx, matchedID, parentID, folder, oldMeta)
	}

	folderID := incomingID
	if folderID == "" || st.usedFolderIDs[folderID] {
		folderID = newID()
		for st.usedFolderIDs[folderID] {
			folderID = newID()
		}
	}
	meta := folder.MetaJSON
	if folder.Auth != nil {
		meta = MergeAuth(meta, *folder.Auth)
	}
	if _, err := tx.Exec(`INSERT INTO folders(id, collection_id, parent_folder_id, name,
		sort_order, created_at, updated_at, meta_json) VALUES(?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)`,
		folderID, st.collectionID, parentID, folder.Name, folder.SortOrder, st.now, st.now, meta); err != nil {
		return "", err
	}
	st.usedFolderIDs[folderID] = true
	return folderID, nil
}

func (st *openAPIMergeState) updateOpenAPIFolder(tx *sql.Tx, folderID, parentID string,
	folder PortableFolder, oldMeta string) (string, error) {
	meta := MergeOpenAPIRequestMeta(oldMeta, folder.MetaJSON)
	if _, err := tx.Exec(`UPDATE folders SET name = ?, parent_folder_id = NULLIF(?, ''),
		sort_order = ?, updated_at = ?, meta_json = ? WHERE id = ?`,
		folder.Name, parentID, folder.SortOrder, st.now, meta, folderID); err != nil {
		return "", err
	}
	st.matchedFolder[folderID] = true
	return folderID, nil
}

func (st *openAPIMergeState) matchFolder(folder PortableFolder) string {
	for _, key := range openAPIFolderKeys(folder.MetaJSON) {
		for _, id := range st.folderKeys[key] {
			if !st.matchedFolder[id] {
				return id
			}
		}
	}
	return ""
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
	seen := map[string]bool{}
	add := func(key string) {
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		keys = append(keys, key)
	}
	if meta.OpenAPI.SyncKey != "" {
		add("sync:" + meta.OpenAPI.SyncKey)
	}
	if meta.OpenAPI.Method != "" && meta.OpenAPI.Path != "" {
		add("sync:" + openAPISyncKey(meta.OpenAPI.Method, meta.OpenAPI.Path))
		add("raw:" + strings.ToUpper(meta.OpenAPI.Method) + " " + meta.OpenAPI.Path)
	}
	if meta.OpenAPI.OperationID != "" {
		add("operation:" + meta.OpenAPI.OperationID)
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
