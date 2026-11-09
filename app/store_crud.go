package app

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Node kinds used by the tree operations.
const (
	NodeCollection = "collection"
	NodeFolder     = "folder"
	NodeRequest    = "request"
)

func (s *Store) CreateFolder(collectionID, parentID, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "新文件夹"
	}
	id := newID()
	now := time.Now().UnixMilli()
	var sort int
	var err error
	if parentID == "" {
		err = s.db.QueryRow(`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM folders
			WHERE collection_id = ? AND parent_folder_id IS NULL`, collectionID).Scan(&sort)
	} else {
		err = s.db.QueryRow(`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM folders
			WHERE collection_id = ? AND parent_folder_id = ?`, collectionID, parentID).Scan(&sort)
	}
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(`INSERT INTO folders(id, collection_id, parent_folder_id, name,
		sort_order, created_at, updated_at, meta_json)
		VALUES(?, ?, NULLIF(?, ''), ?, ?, ?, ?, '{}')`,
		id, collectionID, parentID, name, sort, now, now)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) RenameNode(kind, id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("名称不能为空")
	}
	table := ""
	switch kind {
	case NodeCollection:
		table = "collections"
	case NodeFolder:
		table = "folders"
	case NodeRequest:
		table = "requests"
	default:
		return fmt.Errorf("unknown node kind %q", kind)
	}
	_, err := s.db.Exec(`UPDATE `+table+` SET name = ?, updated_at = ? WHERE id = ?`,
		name, time.Now().UnixMilli(), id)
	return err
}

func (s *Store) DeleteCollection(id string) error {
	requestIDs := s.queryIDs(`SELECT id FROM requests WHERE collection_id = ?`, id)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM exchanges WHERE request_id IN
		(SELECT id FROM requests WHERE collection_id = ?)`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM collections WHERE id = ?`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.removeArtifacts(requestIDs)
	return nil
}

func (s *Store) DeleteFolder(id string) error {
	requestIDs := s.queryIDs(`WITH RECURSIVE sub(id) AS (
		SELECT ?
		UNION ALL
		SELECT f.id FROM folders f JOIN sub ON f.parent_folder_id = sub.id
	)
	SELECT id FROM requests WHERE folder_id IN (SELECT id FROM sub)`, id)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`WITH RECURSIVE sub(id) AS (
			SELECT ?
			UNION ALL
			SELECT f.id FROM folders f JOIN sub ON f.parent_folder_id = sub.id
		)
		DELETE FROM exchanges WHERE request_id IN
		(SELECT id FROM requests WHERE folder_id IN (SELECT id FROM sub))`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM folders WHERE id = ?`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.removeArtifacts(requestIDs)
	return nil
}

func (s *Store) DeleteRequest(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM exchanges WHERE request_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM requests WHERE id = ?`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Artifacts are api-z's own files; remove them only after the row is gone.
	s.removeArtifacts([]string{id})
	return nil
}

func (s *Store) DuplicateRequest(id string) (string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var collectionID, name, method, url, headers, params, body, meta string
	var folder sql.NullString
	var sortOrder int
	err = tx.QueryRow(`SELECT collection_id, folder_id, name, method, url,
		headers_text, params_text, body_text, sort_order, meta_json
		FROM requests WHERE id = ?`, id).Scan(&collectionID, &folder, &name, &method, &url,
		&headers, &params, &body, &sortOrder, &meta)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("请求不存在")
	}
	if err != nil {
		return "", err
	}
	newRequestID := newID()
	now := time.Now().UnixMilli()
	insertAt := sortOrder + 1
	if folder.Valid && folder.String != "" {
		if _, err := tx.Exec(`UPDATE requests SET sort_order = sort_order + 1, updated_at = ?
			WHERE collection_id = ? AND folder_id = ? AND sort_order >= ?`,
			now, collectionID, folder.String, insertAt); err != nil {
			return "", err
		}
	} else {
		if _, err := tx.Exec(`UPDATE requests SET sort_order = sort_order + 1, updated_at = ?
			WHERE collection_id = ? AND folder_id IS NULL AND sort_order >= ?`,
			now, collectionID, insertAt); err != nil {
			return "", err
		}
	}
	var folderArg any
	if folder.Valid && folder.String != "" {
		folderArg = folder.String
	}
	if _, err := tx.Exec(`INSERT INTO requests(id, collection_id, folder_id, name, method, url,
		headers_text, params_text, body_text, sort_order, created_at, updated_at, meta_json)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		newRequestID, collectionID, folderArg, name+" Copy", method, url,
		headers, params, body, insertAt, now, now, meta); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return newRequestID, nil
}

// MoveSibling swaps a node with its previous or next sibling in the same
// parent, then renumbers the whole sibling list. delta is -1 or +1.
func (s *Store) MoveSibling(kind, id string, delta int) error {
	if delta != -1 && delta != 1 {
		return errors.New("delta must be -1 or 1")
	}
	var table, parentWhere string
	var args []any
	switch kind {
	case NodeCollection:
		table = "collections"
	case NodeFolder:
		var collectionID string
		var parent sql.NullString
		if err := s.db.QueryRow(`SELECT collection_id, parent_folder_id FROM folders WHERE id = ?`, id).
			Scan(&collectionID, &parent); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}
		table = "folders"
		if parent.Valid && parent.String != "" {
			parentWhere = "collection_id = ? AND parent_folder_id = ?"
			args = []any{collectionID, parent.String}
		} else {
			parentWhere = "collection_id = ? AND parent_folder_id IS NULL"
			args = []any{collectionID}
		}
	case NodeRequest:
		var collectionID string
		var folder sql.NullString
		if err := s.db.QueryRow(`SELECT collection_id, folder_id FROM requests WHERE id = ?`, id).
			Scan(&collectionID, &folder); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}
		table = "requests"
		if folder.Valid && folder.String != "" {
			parentWhere = "collection_id = ? AND folder_id = ?"
			args = []any{collectionID, folder.String}
		} else {
			parentWhere = "collection_id = ? AND folder_id IS NULL"
			args = []any{collectionID}
		}
	default:
		return fmt.Errorf("unknown node kind %q", kind)
	}

	query := `SELECT id FROM ` + table
	if parentWhere != "" {
		query += ` WHERE ` + parentWhere
	}
	query += ` ORDER BY sort_order, id`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	idx := -1
	for i, v := range ids {
		if v == id {
			idx = i
			break
		}
	}
	if idx < 0 || idx+delta < 0 || idx+delta >= len(ids) {
		return nil
	}
	ids[idx], ids[idx+delta] = ids[idx+delta], ids[idx]

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i, v := range ids {
		if _, err := tx.Exec(`UPDATE `+table+` SET sort_order = ? WHERE id = ?`, i, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) queryIDs(query string, args ...any) []string {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out
}

func (s *Store) removeArtifacts(ids []string) {
	for _, id := range ids {
		if id == "" {
			continue
		}
		_ = os.RemoveAll(filepath.Join(filepath.Dir(s.path), "request", id))
	}
}

// NodeName returns the display name of a tree node.
func (s *Store) NodeName(kind, id string) string {
	var table string
	switch kind {
	case NodeCollection:
		table = "collections"
	case NodeFolder:
		table = "folders"
	case NodeRequest:
		table = "requests"
	default:
		return ""
	}
	var name string
	_ = s.db.QueryRow(`SELECT name FROM `+table+` WHERE id = ?`, id).Scan(&name)
	return name
}

// MoveNodeTo reparents a request or folder under a collection root or folder.
// Folders cannot be moved into themselves or their descendants.
func (s *Store) MoveNodeTo(kind, id, collectionID, parentFolderID string) error {
	if strings.TrimSpace(collectionID) == "" {
		return errors.New("目标集合为空")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM collections WHERE id = ?`, collectionID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return errors.New("目标集合不存在")
	}

	switch kind {
	case NodeRequest:
		var oldCollection string
		var oldFolder sql.NullString
		if err := tx.QueryRow(`SELECT collection_id, folder_id FROM requests WHERE id = ?`, id).
			Scan(&oldCollection, &oldFolder); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("请求不存在")
			}
			return err
		}
		if oldCollection == collectionID && oldFolder.String == parentFolderID {
			return nil
		}
		if err := validateTargetFolder(tx, collectionID, parentFolderID); err != nil {
			return err
		}
		sortOrder, err := nextSortOrder(tx, "requests", collectionID, parentFolderID, "folder_id")
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE requests SET collection_id = ?, folder_id = NULLIF(?, ''),
			sort_order = ?, updated_at = ? WHERE id = ?`,
			collectionID, parentFolderID, sortOrder, time.Now().UnixMilli(), id); err != nil {
			return err
		}
	case NodeFolder:
		var oldCollection string
		var oldParent sql.NullString
		if err := tx.QueryRow(`SELECT collection_id, parent_folder_id FROM folders WHERE id = ?`, id).
			Scan(&oldCollection, &oldParent); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("文件夹不存在")
			}
			return err
		}
		if oldCollection == collectionID && oldParent.String == parentFolderID {
			return nil
		}
		if parentFolderID != "" {
			if parentFolderID == id {
				return errors.New("不能把文件夹移动到自身")
			}
			var targetCollection string
			if err := tx.QueryRow(`SELECT collection_id FROM folders WHERE id = ?`, parentFolderID).
				Scan(&targetCollection); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return errors.New("目标文件夹不存在")
				}
				return err
			}
			if targetCollection != collectionID {
				return errors.New("目标文件夹不属于目标集合")
			}
			var descendants int
			if err := tx.QueryRow(`WITH RECURSIVE sub(id) AS (
					SELECT id FROM folders WHERE id = ?
					UNION ALL
					SELECT f.id FROM folders f JOIN sub ON f.parent_folder_id = sub.id
				) SELECT COUNT(*) FROM sub WHERE id = ?`, id, parentFolderID).Scan(&descendants); err != nil {
				return err
			}
			if descendants > 0 {
				return errors.New("不能把文件夹移动到自己的子文件夹")
			}
		}
		sortOrder, err := nextSortOrder(tx, "folders", collectionID, parentFolderID, "parent_folder_id")
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE folders SET collection_id = ?, parent_folder_id = NULLIF(?, ''),
			sort_order = ?, updated_at = ? WHERE id = ?`,
			collectionID, parentFolderID, sortOrder, time.Now().UnixMilli(), id); err != nil {
			return err
		}
		if oldCollection != collectionID {
			// The subtree follows its folder into the new collection.
			sub := `WITH RECURSIVE sub(id) AS (
				SELECT id FROM folders WHERE id = ?
				UNION ALL
				SELECT f.id FROM folders f JOIN sub ON f.parent_folder_id = sub.id
			) `
			if _, err := tx.Exec(sub+`UPDATE folders SET collection_id = ? WHERE id IN (SELECT id FROM sub)`,
				id, collectionID); err != nil {
				return err
			}
			if _, err := tx.Exec(sub+`UPDATE requests SET collection_id = ? WHERE folder_id IN (SELECT id FROM sub)`,
				id, collectionID); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unknown node kind %q", kind)
	}
	return tx.Commit()
}

func validateTargetFolder(tx *sql.Tx, collectionID, folderID string) error {
	if folderID == "" {
		return nil
	}
	var targetCollection string
	if err := tx.QueryRow(`SELECT collection_id FROM folders WHERE id = ?`, folderID).Scan(&targetCollection); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("目标文件夹不存在")
		}
		return err
	}
	if targetCollection != collectionID {
		return errors.New("目标文件夹不属于目标集合")
	}
	return nil
}

func nextSortOrder(tx *sql.Tx, table, collectionID, parentID, parentColumn string) (int, error) {
	query := `SELECT COALESCE(MAX(sort_order), -1) + 1 FROM ` + table + ` WHERE collection_id = ? AND `
	if parentID == "" {
		query += parentColumn + ` IS NULL`
	} else {
		query += parentColumn + ` = ?`
	}
	var sort int
	var err error
	if parentID == "" {
		err = tx.QueryRow(query, collectionID).Scan(&sort)
	} else {
		err = tx.QueryRow(query, collectionID, parentID).Scan(&sort)
	}
	return sort, err
}

// MoveNodeBefore reparents a node and inserts it before beforeID among its
// new siblings (empty beforeID appends). It keeps the visible grouping: a
// folder only orders among folders, a request among requests.
func (s *Store) MoveNodeBefore(kind, id, collectionID, parentFolderID, beforeID string) error {
	if beforeID == id {
		return nil
	}
	if err := s.MoveNodeTo(kind, id, collectionID, parentFolderID); err != nil {
		return err
	}
	return s.reorderSiblings(kind, collectionID, parentFolderID, id, beforeID)
}

func (s *Store) reorderSiblings(kind, collectionID, parentFolderID, id, beforeID string) error {
	var table, parentColumn string
	switch kind {
	case NodeFolder:
		table, parentColumn = "folders", "parent_folder_id"
	case NodeRequest:
		table, parentColumn = "requests", "folder_id"
	default:
		return fmt.Errorf("unknown node kind %q", kind)
	}
	query := `SELECT id FROM ` + table + ` WHERE collection_id = ? AND ` + parentColumn
	args := []any{collectionID}
	if parentFolderID == "" {
		query += ` IS NULL`
	} else {
		query += ` = ?`
		args = append(args, parentFolderID)
	}
	query += ` ORDER BY sort_order, id`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	// Remove the moved node, then insert it at the requested place.
	filtered := ids[:0]
	for _, v := range ids {
		if v != id {
			filtered = append(filtered, v)
		}
	}
	insertAt := len(filtered)
	if beforeID != "" {
		for i, v := range filtered {
			if v == beforeID {
				insertAt = i
				break
			}
		}
	}
	filtered = append(filtered, "")
	copy(filtered[insertAt+1:], filtered[insertAt:])
	filtered[insertAt] = id

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i, v := range filtered {
		if _, err := tx.Exec(`UPDATE `+table+` SET sort_order = ? WHERE id = ?`, i, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}
