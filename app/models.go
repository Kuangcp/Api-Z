package app

import (
	"encoding/json"
	"strings"
)

type Collection struct {
	ID        string
	Name      string
	SortOrder int
	CreatedAt int64
	UpdatedAt int64
	MetaJSON  string
}

type Folder struct {
	ID           string
	CollectionID string
	ParentID     string // "" for the collection root
	Name         string
	SortOrder    int
	CreatedAt    int64
	UpdatedAt    int64
	MetaJSON     string
}

type Request struct {
	ID           string
	CollectionID string
	FolderID     string // "" for the collection root
	Name         string
	Method       string
	URL          string
	HeadersText  string
	ParamsText   string
	BodyText     string
	SortOrder    int
	CreatedAt    int64
	UpdatedAt    int64
	MetaJSON     string
}

type Tree struct {
	Collections []Collection
	Folders     []Folder
	Requests    []Request
}

func (t Tree) Collection(id string) (Collection, bool) {
	for _, c := range t.Collections {
		if c.ID == id {
			return c, true
		}
	}
	return Collection{}, false
}

func (t Tree) Request(id string) (Request, bool) {
	for _, r := range t.Requests {
		if r.ID == id {
			return r, true
		}
	}
	return Request{}, false
}

// EffectiveAuth resolves request -> folder parents -> collection. api-x
// stores "inherit" at any level; the first concrete auth wins.
func (t Tree) EffectiveAuth(requestID string) Auth {
	req, ok := t.Request(requestID)
	if !ok {
		return Auth{Type: "noauth"}
	}
	if a := ParseAuth(req.MetaJSON); a.Type != "inherit" {
		return a
	}
	parent := req.FolderID
	for parent != "" {
		found := false
		for _, f := range t.Folders {
			if f.ID != parent {
				continue
			}
			found = true
			if a := ParseAuth(f.MetaJSON); a.Type != "inherit" {
				return a
			}
			parent = f.ParentID
			break
		}
		if !found {
			break
		}
	}
	for _, c := range t.Collections {
		if c.ID == req.CollectionID {
			if a := ParseAuth(c.MetaJSON); a.Type != "inherit" {
				return a
			}
			break
		}
	}
	return Auth{Type: "noauth"}
}

// Auth is the subset of Postman v2.1 auth api-x stores in requests.meta_json.
type Auth struct {
	Type   string         `json:"type"`
	APIKey []AuthProperty `json:"apikey,omitempty"`
	Basic  []AuthProperty `json:"basic,omitempty"`
	Bearer []AuthProperty `json:"bearer,omitempty"`
}

type AuthProperty struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Type  string `json:"type,omitempty"`
}

func (a Auth) Property(list []AuthProperty, key string) string {
	for _, p := range list {
		if p.Key == key {
			return p.Value
		}
	}
	return ""
}

func ParseAuth(metaJSON string) Auth {
	var meta struct {
		Auth *Auth `json:"auth"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(metaJSON)), &meta); err != nil || meta.Auth == nil {
		return Auth{Type: "inherit"}
	}
	if meta.Auth.Type == "" {
		meta.Auth.Type = "inherit"
	}
	return *meta.Auth
}

func MergeAuth(metaJSON string, auth Auth) string {
	meta := map[string]json.RawMessage{}
	_ = json.Unmarshal([]byte(strings.TrimSpace(metaJSON)), &meta)
	if meta == nil {
		meta = map[string]json.RawMessage{}
	}
	raw, _ := json.Marshal(auth)
	meta["auth"] = raw
	out, err := json.Marshal(meta)
	if err != nil {
		return metaJSON
	}
	return string(out)
}

// Environments mirrors api-x's environments.json.
type EnvState struct {
	Version             int           `json:"version"`
	ActiveEnvironmentID string        `json:"activeEnvironmentId"`
	Environments        []Environment `json:"environments"`
}

type Environment struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Tag       string        `json:"tag,omitempty"`
	Variables []EnvVariable `json:"variables"`
}

type EnvVariable struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (s EnvState) Active() *Environment {
	if s.ActiveEnvironmentID == "" {
		return nil
	}
	for i := range s.Environments {
		if s.Environments[i].ID == s.ActiveEnvironmentID {
			return &s.Environments[i]
		}
	}
	return nil
}

func (s EnvState) Map() map[string]string {
	env := s.Active()
	if env == nil {
		return nil
	}
	m := make(map[string]string, len(env.Variables))
	for _, v := range env.Variables {
		if k := strings.TrimSpace(v.Key); k != "" {
			m[k] = v.Value
		}
	}
	return m
}

// EnsureTags fills a missing tag from the environment name, as api-x does:
// "HCCM - Test" -> "HCCM", "工作流-Local" -> "工作流".
func (s EnvState) EnsureTags() EnvState {
	out := s.Clone()
	for i := range out.Environments {
		e := &out.Environments[i]
		if strings.TrimSpace(e.Tag) != "" {
			continue
		}
		name := strings.TrimSpace(e.Name)
		if idx := strings.IndexAny(name, " -"); idx > 0 {
			e.Tag = strings.TrimSpace(name[:idx])
		}
	}
	return out
}

func (s EnvState) Normalize() EnvState {
	ids := map[string]bool{}
	for _, e := range s.Environments {
		ids[e.ID] = true
	}
	if !ids[s.ActiveEnvironmentID] {
		s.ActiveEnvironmentID = ""
	}
	if s.Version == 0 {
		s.Version = 1
	}
	return s
}

// Exchange is one completed response, stored beside the request artifacts.
type Exchange struct {
	ID         string
	RequestID  string
	SavedAt    int64
	Status     int
	DurationMS int64
	SizeBytes  int64
	IsSSE      bool
	TTFTMS     int64
	EventCount int
	HARPath    string
}

// NewID returns a UUID v4 string for new collections, environments, etc.
func NewID() string { return newID() }

func (s EnvState) Clone() EnvState {
	out := s
	out.Environments = make([]Environment, len(s.Environments))
	for i, e := range s.Environments {
		out.Environments[i] = e
		out.Environments[i].Variables = append([]EnvVariable(nil), e.Variables...)
	}
	return out
}

// Clean normalizes a draft before saving: ids, names, tags, empty variables,
// and the active environment.
func (s EnvState) Clean() EnvState {
	out := s.Clone()
	envs := out.Environments[:0]
	for i := range out.Environments {
		e := out.Environments[i]
		if e.ID == "" {
			e.ID = NewID()
		}
		e.Name = strings.TrimSpace(e.Name)
		if e.Name == "" {
			e.Name = "未命名环境"
		}
		e.Tag = strings.TrimSpace(e.Tag)
		vars := e.Variables[:0]
		for _, v := range e.Variables {
			v.Key = strings.TrimSpace(v.Key)
			if v.Key != "" {
				vars = append(vars, v)
			}
		}
		e.Variables = vars
		envs = append(envs, e)
	}
	out.Environments = envs
	out = out.EnsureTags().Normalize()
	if len(out.Environments) > 0 && out.ActiveEnvironmentID == "" {
		out.ActiveEnvironmentID = out.Environments[0].ID
	}
	return out
}
