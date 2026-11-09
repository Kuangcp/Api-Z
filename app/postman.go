package app

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

const postmanSchemaV21 = "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"

// PortableCollection is the neutral shape between Postman v2.1 and the
// api-z tables. IDs are only advisory on import; api-z generates its own.
type PortableCollection struct {
	Name     string
	MetaJSON string
	Auth     *Auth
	Folders  []PortableFolder
	Requests []PortableRequest
	ID       string
}

type PortableFolder struct {
	Name      string
	MetaJSON  string
	Auth      *Auth
	Folders   []PortableFolder
	Requests  []PortableRequest
	ID        string
	SortOrder int
}

type PortableRequest struct {
	Name        string
	Method      string
	URL         string
	HeadersText string
	ParamsText  string
	BodyText    string
	MetaJSON    string
	Auth        *Auth
	ID          string
	SortOrder   int
}

// ParsePostmanCollection reads v2.0/v2.1 JSON loosely, as api-x does.
func ParsePostmanCollection(data []byte) (PortableCollection, error) {
	var root map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return PortableCollection{}, fmt.Errorf("JSON 解析失败: %w", err)
	}
	info, _ := root["info"].(map[string]any)
	if info == nil {
		return PortableCollection{}, errors.New("缺少 Postman 集合字段 info")
	}
	name := strings.TrimSpace(strAny(info["name"]))
	if name == "" {
		return PortableCollection{}, errors.New("缺少 info.name")
	}
	schema := strings.ToLower(strAny(info["schema"]))
	if strings.Contains(schema, "openapi") || strings.Contains(schema, "swagger") {
		return PortableCollection{}, errors.New("请选择 Postman Collection JSON（当前文件像是 OpenAPI）")
	}
	meta := map[string]any{}
	if d := descriptionOf(info["description"]); d != "" {
		meta["description"] = d
	}
	if id := strings.TrimSpace(strAny(info["_postman_id"])); id != "" {
		meta["_postman_id"] = id
	}
	if v, ok := root["variable"]; ok {
		meta["variable"] = v
	}
	metaJSON := marshalMeta(meta)

	folders, requests, err := parsePostmanItems(root["item"], 0)
	if err != nil {
		return PortableCollection{}, err
	}
	return PortableCollection{
		Name:     name,
		MetaJSON: metaJSON,
		Auth:     parsePortableAuth(root["auth"]),
		Folders:  folders,
		Requests: requests,
		ID:       strings.TrimSpace(strAny(info["_api_x_id"])),
	}, nil
}

func parsePostmanItems(v any, depth int) ([]PortableFolder, []PortableRequest, error) {
	if depth > 64 {
		return nil, nil, errors.New("文件夹嵌套超过 64 层")
	}
	list, _ := v.([]any)
	var folders []PortableFolder
	var requests []PortableRequest
	for i, el := range list {
		obj, _ := el.(map[string]any)
		if obj == nil {
			continue
		}
		if _, ok := obj["request"]; ok {
			req, err := parsePostmanRequest(obj, i)
			if err != nil {
				return nil, nil, err
			}
			requests = append(requests, req)
			continue
		}
		folder, err := parsePostmanFolder(obj, i, depth)
		if err != nil {
			return nil, nil, err
		}
		folders = append(folders, folder)
	}
	return folders, requests, nil
}

func parsePostmanFolder(obj map[string]any, order, depth int) (PortableFolder, error) {
	name := strings.TrimSpace(strAny(obj["name"]))
	if name == "" {
		name = "(文件夹)"
	}
	meta := map[string]any{}
	if d := descriptionOf(obj["description"]); d != "" {
		meta["description"] = d
	}
	subFolders, subRequests, err := parsePostmanItems(obj["item"], depth+1)
	if err != nil {
		return PortableFolder{}, err
	}
	return PortableFolder{
		Name:      name,
		MetaJSON:  marshalMeta(meta),
		Auth:      parsePortableAuth(obj["auth"]),
		Folders:   subFolders,
		Requests:  subRequests,
		ID:        strings.TrimSpace(strAny(obj["id"])),
		SortOrder: order,
	}, nil
}

func parsePostmanRequest(obj map[string]any, order int) (PortableRequest, error) {
	name := strings.TrimSpace(strAny(obj["name"]))
	if name == "" {
		name = "(请求)"
	}
	req, _ := obj["request"].(map[string]any)
	if req == nil {
		return PortableRequest{}, fmt.Errorf("请求「%s」缺少 request 对象", name)
	}
	method := strings.ToUpper(strings.TrimSpace(strAny(req["method"])))
	if method == "" {
		method = "GET"
	}
	rawURL, paramsText := parsePostmanURL(req["url"])
	meta := map[string]any{}
	if d := descriptionOf(obj["description"]); d != "" {
		meta["description"] = d
	}
	auth := parsePortableAuth(obj["auth"])
	if auth == nil {
		auth = parsePortableAuth(req["auth"])
	}
	return PortableRequest{
		Name:        name,
		Method:      method,
		URL:         rawURL,
		HeadersText: parsePostmanHeaders(req["header"]),
		ParamsText:  paramsText,
		BodyText:    parsePostmanBody(req["body"]),
		MetaJSON:    marshalMeta(meta),
		Auth:        auth,
		ID:          strings.TrimSpace(strAny(obj["id"])),
		SortOrder:   order,
	}, nil
}

func parsePostmanHeaders(v any) string {
	list, _ := v.([]any)
	var lines []string
	for _, el := range list {
		obj, _ := el.(map[string]any)
		if obj == nil {
			continue
		}
		key := strings.TrimSpace(strAny(obj["key"]))
		if key == "" {
			continue
		}
		line := key + ": " + valueString(obj["value"])
		if boolAny(obj["disabled"]) {
			line = "! " + line
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func parsePostmanBody(v any) string {
	obj, _ := v.(map[string]any)
	if obj == nil {
		return ""
	}
	switch strings.ToLower(strAny(obj["mode"])) {
	case "raw":
		if s, ok := obj["raw"].(string); ok {
			return s
		}
		if obj["raw"] != nil {
			b, _ := json.Marshal(obj["raw"])
			return string(b)
		}
	case "urlencoded":
		list, _ := obj["urlencoded"].([]any)
		var pairs []string
		for _, el := range list {
			o, _ := el.(map[string]any)
			if o == nil || boolAny(o["disabled"]) {
				continue
			}
			k := strAny(o["key"])
			if k == "" {
				continue
			}
			pairs = append(pairs, url.QueryEscape(k)+"="+url.QueryEscape(strAny(o["value"])))
		}
		return strings.Join(pairs, "&")
	case "graphql":
		g, _ := obj["graphql"].(map[string]any)
		return strAny(g["query"])
	}
	return ""
}

func parsePostmanURL(v any) (string, string) {
	switch x := v.(type) {
	case string:
		return splitURLQuery(x)
	case map[string]any:
		if raw := strings.TrimSpace(strAny(x["raw"])); raw != "" {
			base, params := splitURLQuery(raw)
			if q := parsePostmanQuery(x["query"]); q != "" {
				params = q
			}
			return base, params
		}
		protocol := strings.ToLower(strings.TrimSpace(strAny(x["protocol"])))
		host := strings.Join(stringList(x["host"]), ".")
		port := strings.TrimSpace(strAny(x["port"]))
		authority := host
		if port != "" {
			authority += ":" + port
		}
		path := strings.Join(stringList(x["path"]), "/")
		base := ""
		switch {
		case authority != "" && path != "":
			base = authority + "/" + path
		case authority != "":
			base = authority
		case path != "":
			base = "/" + path
		}
		if base != "" && protocol != "" {
			base = protocol + "://" + base
		}
		if hash := strings.TrimSpace(strAny(x["hash"])); hash != "" {
			base += "#" + hash
		}
		return base, parsePostmanQuery(x["query"])
	}
	return "", ""
}

func parsePostmanQuery(v any) string {
	list, _ := v.([]any)
	var lines []string
	for _, el := range list {
		obj, _ := el.(map[string]any)
		if obj == nil {
			continue
		}
		key := strAny(obj["key"])
		if key == "" {
			continue
		}
		line := key + ": " + strAny(obj["value"])
		if boolAny(obj["disabled"]) {
			line = "! " + line
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func splitURLQuery(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	fragment := ""
	if i := strings.IndexByte(trimmed, '#'); i >= 0 {
		trimmed, fragment = trimmed[:i], trimmed[i:]
	}
	base, query := trimmed, ""
	if i := strings.IndexByte(trimmed, '?'); i >= 0 {
		base, query = trimmed[:i], trimmed[i+1:]
	}
	var lines []string
	for _, part := range strings.Split(query, "&") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		k, err := url.QueryUnescape(k)
		if err != nil {
			continue
		}
		if k == "" {
			continue
		}
		v, _ = url.QueryUnescape(v)
		lines = append(lines, k+": "+v)
	}
	return base + fragment, strings.Join(lines, "\n")
}

func parsePortableAuth(v any) *Auth {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var a Auth
	if err := json.Unmarshal(b, &a); err != nil || a.Type == "" {
		return nil
	}
	return &a
}

func marshalMeta(v map[string]any) string {
	if len(v) == 0 {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func descriptionOf(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case map[string]any:
		if s := strings.TrimSpace(strAny(x["content"])); s != "" {
			return s
		}
		return strings.TrimSpace(strAny(x["text"]))
	}
	return ""
}

func strAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func boolAny(v any) bool {
	b, ok := v.(bool)
	return ok && b
}

func valueString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprint(x)
		}
		return string(b)
	}
}

func stringList(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		out := make([]string, 0, len(x))
		for _, el := range x {
			if s, ok := el.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// ImportPortableCollection writes a parsed collection as a new collection.
func (s *Store) ImportPortableCollection(p PortableCollection) (string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	collID := newID()
	now := time.Now().UnixMilli()
	meta := p.MetaJSON
	if p.Auth != nil {
		meta = MergeAuth(meta, *p.Auth)
	}
	var sortOrder int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM collections`).Scan(&sortOrder); err != nil {
		return "", err
	}
	if _, err := tx.Exec(`INSERT INTO collections(id, name, sort_order, created_at, updated_at, meta_json)
		VALUES(?, ?, ?, ?, ?, ?)`, collID, p.Name, sortOrder, now, now, meta); err != nil {
		return "", err
	}
	if err := insertPortableFolders(tx, collID, "", p.Folders, now); err != nil {
		return "", err
	}
	if err := insertPortableRequests(tx, collID, "", p.Requests, now); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return collID, nil
}

func insertPortableFolders(tx *sql.Tx, collectionID, parentID string, folders []PortableFolder, now int64) error {
	for i, f := range folders {
		folderID := newID()
		meta := f.MetaJSON
		if f.Auth != nil {
			meta = MergeAuth(meta, *f.Auth)
		}
		if _, err := tx.Exec(`INSERT INTO folders(id, collection_id, parent_folder_id, name,
			sort_order, created_at, updated_at, meta_json)
			VALUES(?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)`,
			folderID, collectionID, parentID, f.Name, i, now, now, meta); err != nil {
			return err
		}
		if err := insertPortableFolders(tx, collectionID, folderID, f.Folders, now); err != nil {
			return err
		}
		if err := insertPortableRequests(tx, collectionID, folderID, f.Requests, now); err != nil {
			return err
		}
	}
	return nil
}

func insertPortableRequests(tx *sql.Tx, collectionID, folderID string, requests []PortableRequest, now int64) error {
	for i, r := range requests {
		meta := r.MetaJSON
		if r.Auth != nil {
			meta = MergeAuth(meta, *r.Auth)
		}
		if _, err := tx.Exec(`INSERT INTO requests(id, collection_id, folder_id, name, method, url,
			headers_text, params_text, body_text, sort_order, created_at, updated_at, meta_json)
			VALUES(?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			newID(), collectionID, folderID, r.Name, r.Method, r.URL,
			r.HeadersText, r.ParamsText, r.BodyText, i, now, now, meta); err != nil {
			return err
		}
	}
	return nil
}

// ExportPortableCollection reads a collection back into the neutral shape.
func (s *Store) ExportPortableCollection(collectionID string) (PortableCollection, error) {
	tree, err := s.LoadTree()
	if err != nil {
		return PortableCollection{}, err
	}
	var col *Collection
	for i := range tree.Collections {
		if tree.Collections[i].ID == collectionID {
			col = &tree.Collections[i]
			break
		}
	}
	if col == nil {
		return PortableCollection{}, errors.New("集合不存在")
	}
	out := PortableCollection{
		Name:     col.Name,
		MetaJSON: col.MetaJSON,
		Auth:     portableAuth(col.MetaJSON),
		ID:       col.ID,
	}
	var build func(parentID string) ([]PortableFolder, []PortableRequest)
	build = func(parentID string) ([]PortableFolder, []PortableRequest) {
		var folders []PortableFolder
		for _, f := range tree.Folders {
			if f.CollectionID != collectionID || f.ParentID != parentID {
				continue
			}
			subFolders, subRequests := build(f.ID)
			folders = append(folders, PortableFolder{
				Name:      f.Name,
				MetaJSON:  f.MetaJSON,
				Auth:      portableAuth(f.MetaJSON),
				Folders:   subFolders,
				Requests:  subRequests,
				ID:        f.ID,
				SortOrder: f.SortOrder,
			})
		}
		sort.SliceStable(folders, func(i, j int) bool { return folders[i].SortOrder < folders[j].SortOrder })
		var requests []PortableRequest
		for _, r := range tree.Requests {
			if r.CollectionID != collectionID || r.FolderID != parentID {
				continue
			}
			requests = append(requests, PortableRequest{
				Name:        r.Name,
				Method:      r.Method,
				URL:         r.URL,
				HeadersText: r.HeadersText,
				ParamsText:  r.ParamsText,
				BodyText:    r.BodyText,
				MetaJSON:    r.MetaJSON,
				Auth:        portableAuth(r.MetaJSON),
				ID:          r.ID,
				SortOrder:   r.SortOrder,
			})
		}
		sort.SliceStable(requests, func(i, j int) bool { return requests[i].SortOrder < requests[j].SortOrder })
		return folders, requests
	}
	out.Folders, out.Requests = build("")
	return out, nil
}

func portableAuth(metaJSON string) *Auth {
	var meta struct {
		Auth *Auth `json:"auth"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(metaJSON)), &meta) != nil || meta.Auth == nil {
		return nil
	}
	if meta.Auth.Type == "" || meta.Auth.Type == "inherit" {
		return nil
	}
	return meta.Auth
}

// PostmanJSON serializes a portable collection as Postman Collection v2.1.
func PostmanJSON(p PortableCollection) ([]byte, error) {
	meta := map[string]any{}
	_ = json.Unmarshal([]byte(strings.TrimSpace(p.MetaJSON)), &meta)
	postmanID := strAny(meta["_postman_id"])
	if postmanID == "" {
		postmanID = newID()
	}
	info := pmInfo{
		Name:        p.Name,
		Schema:      postmanSchemaV21,
		PostmanID:   postmanID,
		Description: strAny(meta["description"]),
		APIXID:      p.ID,
	}
	out := pmCollection{
		Info: info,
		Auth: p.Auth,
		Item: mergePostmanItems(p.Folders, p.Requests),
	}
	if v, ok := meta["variable"]; ok {
		raw, err := json.Marshal(v)
		if err == nil {
			out.Variable = raw
		}
	}
	return json.MarshalIndent(out, "", "  ")
}

type pmCollection struct {
	Info     pmInfo          `json:"info"`
	Auth     *Auth           `json:"auth,omitempty"`
	Variable json.RawMessage `json:"variable,omitempty"`
	Item     []pmItem        `json:"item"`
}

type pmInfo struct {
	Name        string `json:"name"`
	Schema      string `json:"schema"`
	PostmanID   string `json:"_postman_id,omitempty"`
	Description string `json:"description,omitempty"`
	APIXID      string `json:"_api_x_id,omitempty"`
}

type pmItem struct {
	ID          string     `json:"id,omitempty"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Auth        *Auth      `json:"auth,omitempty"`
	Item        []pmItem   `json:"item,omitempty"`
	Request     *pmRequest `json:"request,omitempty"`
	Response    []any      `json:"response,omitempty"`
}

type pmRequest struct {
	Method string     `json:"method"`
	Header []pmHeader `json:"header"`
	Body   pmBody     `json:"body"`
	URL    pmURL      `json:"url"`
}

type pmHeader struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Type     string `json:"type"`
	Disabled bool   `json:"disabled"`
}

type pmBody struct {
	Mode    string     `json:"mode"`
	Raw     string     `json:"raw,omitempty"`
	Options *pmOptions `json:"options,omitempty"`
}

type pmOptions struct {
	Raw pmRawOptions `json:"raw"`
}

type pmRawOptions struct {
	Language string `json:"language"`
}

type pmURL struct {
	Raw      string     `json:"raw"`
	Protocol string     `json:"protocol,omitempty"`
	Host     []string   `json:"host,omitempty"`
	Path     []string   `json:"path,omitempty"`
	Port     string     `json:"port,omitempty"`
	Query    []pmKVPair `json:"query,omitempty"`
	Hash     string     `json:"hash,omitempty"`
}

type pmKVPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func mergePostmanItems(folders []PortableFolder, requests []PortableRequest) []pmItem {
	type tagged struct {
		order int
		name  string
		item  pmItem
	}
	var list []tagged
	for _, f := range folders {
		sub := mergePostmanItems(f.Folders, f.Requests)
		list = append(list, tagged{f.SortOrder, f.Name, pmItem{
			ID:          f.ID,
			Name:        f.Name,
			Description: metaDescription(f.MetaJSON),
			Auth:        f.Auth,
			Item:        sub,
		}})
	}
	for _, r := range requests {
		list = append(list, tagged{r.SortOrder, r.Name, pmRequestItem(r)})
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].order != list[j].order {
			return list[i].order < list[j].order
		}
		return list[i].name < list[j].name
	})
	out := make([]pmItem, 0, len(list))
	for _, v := range list {
		out = append(out, v.item)
	}
	return out
}

func pmRequestItem(r PortableRequest) pmItem {
	return pmItem{
		ID:          r.ID,
		Name:        r.Name,
		Description: metaDescription(r.MetaJSON),
		Auth:        r.Auth,
		Request:     pmRequestFrom(r),
		Response:    []any{},
	}
}

func pmRequestFrom(r PortableRequest) *pmRequest {
	headers := splitHeadersWithState(r.HeadersText)
	headerOut := make([]pmHeader, 0, len(headers))
	for _, h := range headers {
		headerOut = append(headerOut, pmHeader{Key: h.Name, Value: h.Value, Type: "text", Disabled: !h.Enabled})
	}
	body := pmBody{Mode: "none"}
	if strings.TrimSpace(r.BodyText) != "" {
		body = pmBody{
			Mode:    "raw",
			Raw:     r.BodyText,
			Options: &pmOptions{Raw: pmRawOptions{Language: bodyLanguage(r.HeadersText)}},
		}
	}
	return &pmRequest{
		Method: strings.ToUpper(r.Method),
		Header: headerOut,
		Body:   body,
		URL:    pmURLFrom(r.URL, r.ParamsText),
	}
}

func pmURLFrom(rawURL, paramsText string) pmURL {
	merged := MergeURLWithParams(rawURL, ParseParams(paramsText))
	baseWithFrag, queryText := splitURLQuery(merged)
	hash := ""
	if i := strings.IndexByte(baseWithFrag, '#'); i >= 0 {
		baseWithFrag, hash = baseWithFrag[:i], baseWithFrag[i+1:]
	}
	out := pmURL{Raw: merged, Hash: hash}
	for _, line := range strings.Split(queryText, "\n") {
		name, value, ok := strings.Cut(line, ": ")
		if !ok || name == "" || strings.HasPrefix(name, "! ") {
			continue
		}
		out.Query = append(out.Query, pmKVPair{Key: name, Value: value})
	}
	if u, err := url.Parse(baseWithFrag); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		out.Protocol = u.Scheme
		out.Host = strings.Split(u.Hostname(), ".")
		out.Path = splitPath(u.Path)
		port := u.Port()
		if port == "80" && u.Scheme == "http" || port == "443" && u.Scheme == "https" {
			port = ""
		}
		out.Port = port
	}
	return out
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func splitHeadersWithState(text string) []headerRow {
	var out []headerRow
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" {
			continue
		}
		enabled := true
		if strings.HasPrefix(line, "! ") {
			enabled = false
			line = strings.TrimSpace(strings.TrimPrefix(line, "! "))
		}
		name, value, ok := strings.Cut(line, ":")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			continue
		}
		out = append(out, headerRow{Name: name, Value: strings.TrimSpace(value), Enabled: enabled})
	}
	return out
}

type headerRow struct {
	Name    string
	Value   string
	Enabled bool
}

func bodyLanguage(headersText string) string {
	ct := strings.ToLower(HeaderValue(ParseHeaders(headersText), "Content-Type"))
	switch {
	case strings.Contains(ct, "json"):
		return "json"
	case strings.Contains(ct, "xml"):
		return "xml"
	case strings.Contains(ct, "html"):
		return "html"
	default:
		return "text"
	}
}

func metaDescription(metaJSON string) string {
	var meta struct {
		Description string `json:"description"`
	}
	_ = json.Unmarshal([]byte(strings.TrimSpace(metaJSON)), &meta)
	return strings.TrimSpace(meta.Description)
}
