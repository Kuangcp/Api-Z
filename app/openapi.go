package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type OpenAPIRoot struct {
	Mode   string `json:"mode"`
	EnvKey string `json:"envKey,omitempty"`
}

const (
	OpenAPIRootBaseURL = "baseUrl"
	OpenAPIRootEnv     = "env"
)

type OpenAPIParseResult struct {
	Portable  PortableCollection
	Requests  int
	Folders   int
	SourceURL string
	BaseURL   string
}

var openAPIMethods = []string{"get", "post", "put", "patch", "delete", "head", "options", "trace"}

// FetchOpenAPI downloads an OpenAPI document. A URL without a scheme gets
// http://, matching api-x.
func FetchOpenAPI(ctx context.Context, rawURL string) ([]byte, error) {
	u := NormalizeOpenAPIURL(rawURL)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	return body, nil
}

func NormalizeOpenAPIURL(raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" {
		return u
	}
	low := strings.ToLower(u)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return u
	}
	return "http://" + u
}

func ParseOpenAPI(text, sourceURL, collectionName, collectionID string, root *OpenAPIRoot) (OpenAPIParseResult, error) {
	var doc map[string]any
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return OpenAPIParseResult{}, fmt.Errorf("OpenAPI 内容不是合法 JSON: %w", err)
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		return OpenAPIParseResult{}, fmt.Errorf("OpenAPI 内容缺少 paths")
	}
	baseURL := inferOpenAPIBaseURL(doc, sourceURL)
	pathKeys := make([]string, 0, len(paths))
	for k := range paths {
		pathKeys = append(pathKeys, k)
	}
	sort.Strings(pathKeys)

	grouped := map[string][]PortableRequest{}
	var groupOrder []string
	total := 0
	for _, path := range pathKeys {
		pathItem, _ := paths[path].(map[string]any)
		if pathItem == nil {
			continue
		}
		for _, method := range openAPIMethods {
			operation, _ := pathItem[method].(map[string]any)
			if operation == nil {
				continue
			}
			tag := firstOpenAPITag(operation)
			if tag == "" {
				tag = "Default"
			}
			if _, ok := grouped[tag]; !ok {
				groupOrder = append(groupOrder, tag)
			}
			grouped[tag] = append(grouped[tag], openAPIOperationRequest(doc, root, collectionID, baseURL,
				sourceURL, tag, strings.ToUpper(method), path, operation, len(grouped[tag])))
			total++
		}
	}

	sort.SliceStable(groupOrder, func(i, j int) bool {
		if groupOrder[i] == "Default" {
			return false
		}
		if groupOrder[j] == "Default" {
			return true
		}
		return groupOrder[i] < groupOrder[j]
	})
	var folders []PortableFolder
	for i, tag := range groupOrder {
		folders = append(folders, PortableFolder{
			ID:        openAPIDeterministicID("openapi:" + collectionID + ":folder:" + tag),
			Name:      tag,
			SortOrder: i,
			MetaJSON:  openAPIFolderMetaJSON(sourceURL, tag),
			Requests:  grouped[tag],
		})
	}
	portable := PortableCollection{
		Name:     collectionName,
		MetaJSON: openAPICollectionMetaJSON(sourceURL, baseURL, root),
		Folders:  folders,
	}
	return OpenAPIParseResult{
		Portable:  portable,
		Requests:  total,
		Folders:   len(folders),
		SourceURL: sourceURL,
		BaseURL:   baseURL,
	}, nil
}

func openAPIOperationRequest(doc map[string]any, root *OpenAPIRoot, collectionID, baseURL,
	sourceURL, tag, method, path string, op map[string]any, sortOrder int) PortableRequest {
	summary := strings.TrimSpace(openAPIString(op["summary"]))
	operationID := strings.TrimSpace(openAPIString(op["operationId"]))
	name := summary
	if name == "" {
		name = operationID
	}
	if name == "" {
		name = method + " " + path
	}
	params := collectOpenAPIParameters(doc, op)
	var queryLines, headerLines []string
	for _, p := range params {
		line := p.Name + ": " + p.Example
		if p.Location == "query" {
			queryLines = append(queryLines, line)
		} else if p.Location == "header" {
			headerLines = append(headerLines, line)
		}
	}
	headers := []string{"Accept: application/json"}
	if ct := openAPIRequestBodyContentType(op); ct != "" && method != "GET" && method != "HEAD" {
		headers = append(headers, "Content-Type: "+ct)
	}
	headers = append(headers, headerLines...)
	url := buildOpenAPIURL(baseURL, path, root)
	meta := openAPIRequestMetaJSON(sourceURL, tag, method, path, operationID)
	return PortableRequest{
		ID:          openAPIDeterministicID("openapi:" + collectionID + ":request:" + openAPISyncKey(method, path)),
		Name:        name,
		Method:      method,
		URL:         url,
		HeadersText: uniqueLines(headers),
		ParamsText:  strings.Join(queryLines, "\n"),
		BodyText:    openAPIRequestBodyExample(doc, op),
		SortOrder:   sortOrder,
		MetaJSON:    meta,
		Auth:        &Auth{Type: "inherit"},
	}
}

type openAPIParameter struct {
	Name     string
	Location string
	Example  string
}

func collectOpenAPIParameters(doc map[string]any, op map[string]any) []openAPIParameter {
	list, _ := op["parameters"].([]any)
	var out []openAPIParameter
	for _, raw := range list {
		p, _ := resolveOpenAPIRef(doc, raw).(map[string]any)
		if p == nil {
			continue
		}
		name := strings.TrimSpace(openAPIString(p["name"]))
		if name == "" {
			continue
		}
		loc := openAPIString(p["in"])
		if loc == "" {
			loc = "query"
		}
		schema, _ := resolveOpenAPIRef(doc, p["schema"]).(map[string]any)
		example := openAPIPrimitiveText(p["example"])
		if example == "" {
			example = openAPIPrimitiveText(sampleOpenAPISchema(doc, schema))
		}
		out = append(out, openAPIParameter{Name: name, Location: loc, Example: example})
	}
	return out
}

func openAPIRequestBodyContentType(op map[string]any) string {
	body, _ := op["requestBody"].(map[string]any)
	content, _ := body["content"].(map[string]any)
	if len(content) == 0 {
		return ""
	}
	for _, k := range []string{"multipart", "json"} {
		for ct := range content {
			if strings.Contains(strings.ToLower(ct), k) {
				return ct
			}
		}
	}
	for ct := range content {
		return ct
	}
	return ""
}

func openAPIRequestBodyExample(doc map[string]any, op map[string]any) string {
	body, _ := op["requestBody"].(map[string]any)
	content, _ := body["content"].(map[string]any)
	if len(content) == 0 {
		return ""
	}
	for ct, raw := range content {
		media, _ := raw.(map[string]any)
		if media == nil {
			continue
		}
		if strings.Contains(strings.ToLower(ct), "multipart") {
			return openAPIMultipartExample(doc, media)
		}
	}
	for ct, raw := range content {
		if !strings.Contains(strings.ToLower(ct), "json") {
			continue
		}
		return openAPIMediaExample(doc, raw)
	}
	for _, raw := range content {
		return openAPIMediaExample(doc, raw)
	}
	return ""
}

func openAPIMediaExample(doc map[string]any, raw any) string {
	media, _ := raw.(map[string]any)
	if media == nil {
		return ""
	}
	var element any = media["example"]
	if element == nil {
		if examples, ok := media["examples"].(map[string]any); ok {
			for _, v := range examples {
				if ex, ok := v.(map[string]any); ok {
					element = ex["value"]
					break
				}
			}
		}
	}
	if element == nil {
		schema, _ := resolveOpenAPIRef(doc, media["schema"]).(map[string]any)
		element = sampleOpenAPISchema(doc, schema)
	}
	if element == nil {
		return ""
	}
	b, err := json.Marshal(element)
	if err != nil {
		return ""
	}
	return string(b)
}

func openAPIMultipartExample(doc map[string]any, media map[string]any) string {
	schema, _ := resolveOpenAPIRef(doc, media["schema"]).(map[string]any)
	props, _ := schema["properties"].(map[string]any)
	var lines []string
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		prop, _ := resolveOpenAPIRef(doc, props[name]).(map[string]any)
		typ := openAPIString(prop["type"])
		format := openAPIString(prop["format"])
		structured := typ == "object" || typ == "array" || prop["properties"] != nil || prop["items"] != nil
		switch {
		case format == "binary":
			lines = append(lines, name+": @")
		case structured:
			sample := sampleOpenAPISchema(doc, prop)
			b, _ := json.Marshal(sample)
			lines = append(lines, name+": "+string(b))
		default:
			lines = append(lines, name+": "+openAPIPrimitiveText(sampleOpenAPISchema(doc, prop)))
		}
	}
	return strings.Join(lines, "\n")
}

func sampleOpenAPISchema(doc map[string]any, schema map[string]any) any {
	if schema == nil {
		return ""
	}
	if v, ok := schema["example"]; ok {
		return v
	}
	if v, ok := schema["default"]; ok {
		return v
	}
	resolved, _ := resolveOpenAPIRef(doc, schema).(map[string]any)
	if resolved == nil {
		resolved = schema
	}
	typ := openAPIString(resolved["type"])
	switch {
	case typ == "object" || resolved["properties"] != nil:
		props, _ := resolved["properties"].(map[string]any)
		out := map[string]any{}
		for name, raw := range props {
			s, _ := resolveOpenAPIRef(doc, raw).(map[string]any)
			out[name] = sampleOpenAPISchema(doc, s)
		}
		return out
	case typ == "array":
		items, _ := resolveOpenAPIRef(doc, resolved["items"]).(map[string]any)
		return []any{sampleOpenAPISchema(doc, items)}
	case typ == "integer":
		return json.Number("0")
	case typ == "number":
		return json.Number("0")
	case typ == "boolean":
		return false
	default:
		return ""
	}
}

func resolveOpenAPIRef(doc map[string]any, raw any) any {
	obj, _ := raw.(map[string]any)
	if obj == nil {
		return raw
	}
	ref := openAPIString(obj["$ref"])
	if !strings.HasPrefix(ref, "#/") {
		return raw
	}
	var cur any = doc
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		m, _ := cur.(map[string]any)
		if m == nil {
			return raw
		}
		cur = m[strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")]
	}
	if cur == nil {
		return raw
	}
	return cur
}

func firstOpenAPITag(op map[string]any) string {
	tags, _ := op["tags"].([]any)
	if len(tags) == 0 {
		return ""
	}
	return strings.TrimSpace(openAPIString(tags[0]))
}

func inferOpenAPIBaseURL(doc map[string]any, sourceURL string) string {
	servers, _ := doc["servers"].([]any)
	if len(servers) > 0 {
		if s, ok := servers[0].(map[string]any); ok {
			if u := strings.TrimSpace(openAPIString(s["url"])); u != "" {
				return u
			}
		}
	}
	if u, err := url.Parse(sourceURL); err == nil && u.Scheme != "" && u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	return ""
}

func buildOpenAPIURL(baseURL, path string, root *OpenAPIRoot) string {
	p := path
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if root != nil && root.Mode == OpenAPIRootEnv && strings.TrimSpace(root.EnvKey) != "" {
		return "{{" + strings.TrimSpace(root.EnvKey) + "}}" + p
	}
	return strings.TrimRight(baseURL, "/") + p
}

func openAPIString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func openAPIPrimitiveText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

func uniqueLines(lines []string) string {
	seen := map[string]bool{}
	var out []string
	for _, line := range lines {
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func openAPISyncKey(method, path string) string {
	return strings.ToUpper(method) + " " + normalizeOpenAPIPath(path)
}

func normalizeOpenAPIPath(path string) string {
	p := strings.TrimSpace(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		if p[i] == '{' {
			j := strings.IndexByte(p[i:], '}')
			if j > 0 {
				b.WriteString("{}")
				i += j
				continue
			}
		}
		b.WriteByte(p[i])
	}
	p = b.String()
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	if p == "" {
		p = "/"
	}
	return p
}

func openAPIDeterministicID(seed string) string {
	h := sha256.Sum256([]byte(seed))
	b := h[:16]
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func openAPICollectionMetaJSON(sourceURL, baseURL string, root *OpenAPIRoot) string {
	openapi := map[string]any{
		"sourceUrl":   sourceURL,
		"refreshedAt": time.Now().UnixMilli(),
	}
	if baseURL != "" {
		openapi["baseUrl"] = baseURL
	}
	if root != nil {
		openapi["root"] = root
	}
	return marshalMeta(map[string]any{"openapi": openapi})
}

func openAPIFolderMetaJSON(sourceURL, tag string) string {
	return marshalMeta(map[string]any{"openapi": map[string]any{
		"sourceUrl": sourceURL,
		"kind":      "tag",
		"name":      tag,
	}})
}

func openAPIRequestMetaJSON(sourceURL, tag, method, path, operationID string) string {
	openapi := map[string]any{
		"sourceUrl":      sourceURL,
		"tag":            tag,
		"method":         method,
		"path":           path,
		"normalizedPath": normalizeOpenAPIPath(path),
		"syncKey":        openAPISyncKey(method, path),
	}
	if operationID != "" {
		openapi["operationId"] = operationID
	}
	return marshalMeta(map[string]any{"openapi": openapi})
}

func CollectionOpenAPIInfo(metaJSON string) (sourceURL string, root *OpenAPIRoot) {
	var meta struct {
		OpenAPI struct {
			SourceURL string       `json:"sourceUrl"`
			Root      *OpenAPIRoot `json:"root"`
		} `json:"openapi"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(metaJSON)), &meta) != nil {
		return "", nil
	}
	return strings.TrimSpace(meta.OpenAPI.SourceURL), meta.OpenAPI.Root
}

func mergeOpenAPICollectionMeta(oldMeta, newMeta string) string {
	old := map[string]any{}
	_ = json.Unmarshal([]byte(strings.TrimSpace(oldMeta)), &old)
	if old == nil {
		old = map[string]any{}
	}
	var incoming map[string]any
	_ = json.Unmarshal([]byte(strings.TrimSpace(newMeta)), &incoming)
	if incoming != nil {
		if v, ok := incoming["openapi"]; ok {
			oldOpenAPI, _ := old["openapi"].(map[string]any)
			newOpenAPI, _ := v.(map[string]any)
			if oldOpenAPI != nil && newOpenAPI != nil {
				if _, ok := newOpenAPI["root"]; !ok {
					if root, ok := oldOpenAPI["root"]; ok {
						newOpenAPI["root"] = root
					}
				}
			}
			old["openapi"] = newOpenAPI
		}
	}
	b, err := json.Marshal(old)
	if err != nil {
		return oldMeta
	}
	return string(b)
}

func mergeOpenAPIRequestMeta(oldMeta, newMeta string) string {
	old := map[string]any{}
	_ = json.Unmarshal([]byte(strings.TrimSpace(oldMeta)), &old)
	if old == nil {
		old = map[string]any{}
	}
	var incoming map[string]any
	_ = json.Unmarshal([]byte(strings.TrimSpace(newMeta)), &incoming)
	if incoming != nil {
		if v, ok := incoming["openapi"]; ok {
			old["openapi"] = v
		}
	}
	b, err := json.Marshal(old)
	if err != nil {
		return oldMeta
	}
	return string(b)
}
