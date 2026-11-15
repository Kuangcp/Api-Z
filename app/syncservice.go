package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"api-z/sync"
)

type SyncOutcome struct {
	OK         bool
	Message    string
	NeedsKey   bool
	RepoHasKCV bool
	RepoKCV    string
	Applied    bool
	Written    int
	Deleted    int
}

// SyncService syncs collections and environments through a Git repository.
// The app owns only api-z/ inside the shared repository.
type SyncService struct {
	Store     *Store
	Paths     Paths
	PrefsPath string
	Hostname  string
}

func NewSyncService(store *Store, paths Paths, prefsPath string) *SyncService {
	host, _ := os.Hostname()
	if strings.TrimSpace(host) == "" {
		host = "local"
	}
	return &SyncService{Store: store, Paths: paths, PrefsPath: prefsPath, Hostname: host}
}

func (s *SyncService) engine(prefs SyncPrefs) *sync.Engine {
	return sync.NewEngine(SyncWorkTree(s.PrefsPath), prefs.RepoURL, prefs.Branch, prefs.SSHKeyPath)
}

func (s *SyncService) ProbeKey(ctx context.Context) (string, bool, error) {
	prefs, warn := LoadSyncPrefs(s.PrefsPath)
	if warn != "" {
		return "", false, errors.New(warn)
	}
	if strings.TrimSpace(prefs.RepoURL) == "" {
		return "", false, errors.New("请先在设置 → 同步中填写仓库地址")
	}
	if !sync.Available(ctx) {
		return "", false, errors.New("未找到系统 git，请先安装 git")
	}
	eng := s.engine(prefs)
	if _, err := eng.EnsureRepo(ctx); err != nil {
		return "", false, err
	}
	kcv, ok := eng.RepoKCV(ctx)
	return kcv, ok, nil
}

func (s *SyncService) GenerateKey() (string, error) {
	key, err := sync.NewKey()
	if err != nil {
		return "", err
	}
	if err := SaveSyncKey(s.PrefsPath, key); err != nil {
		return "", err
	}
	return sync.KeyString(key), nil
}

func (s *SyncService) CopyKey() (string, error) {
	key, ok := LoadSyncKey(s.PrefsPath)
	if !ok {
		return "", errors.New("尚未生成同步密钥")
	}
	return sync.KeyString(key), nil
}

func (s *SyncService) ImportKey(text string) error {
	key, err := sync.ParseKeyString(text)
	if err != nil {
		return err
	}
	return SaveSyncKey(s.PrefsPath, key)
}

func (s *SyncService) ClearKey() error { return ClearSyncKey(s.PrefsPath) }

func (s *SyncService) Run(ctx context.Context, action string) SyncOutcome {
	prefs, warn := LoadSyncPrefs(s.PrefsPath)
	if warn != "" {
		return SyncOutcome{Message: warn}
	}
	if strings.TrimSpace(prefs.RepoURL) == "" {
		return SyncOutcome{Message: "请先在设置 → 同步中填写仓库地址"}
	}
	if !sync.Available(ctx) {
		return SyncOutcome{Message: "未找到系统 git，请先安装 git"}
	}
	eng := s.engine(prefs)
	cloned, err := eng.EnsureRepo(ctx)
	if err != nil {
		return SyncOutcome{Message: "准备同步仓库失败：" + gitMessage(err)}
	}
	key, ok := LoadSyncKey(s.PrefsPath)
	if !ok {
		kcv, has := eng.RepoKCV(ctx)
		return SyncOutcome{Message: "需要同步密钥", NeedsKey: true, RepoHasKCV: has, RepoKCV: kcv}
	}
	if err := eng.CheckKey(ctx, key); err != nil {
		switch {
		case errors.Is(err, sync.ErrKeyMismatch):
			return SyncOutcome{Message: "同步密钥与仓库不匹配，请导入正确的 sync.key"}
		case errors.Is(err, sync.ErrRepoKeyMissing):
			return SyncOutcome{Message: err.Error()}
		default:
			return SyncOutcome{Message: "校验同步密钥失败：" + err.Error()}
		}
	}
	if action == "pull" {
		return s.pull(ctx, eng, prefs, key, cloned)
	}
	return s.push(ctx, eng, prefs, key)
}

func (s *SyncService) push(ctx context.Context, eng *sync.Engine, prefs SyncPrefs, key []byte) SyncOutcome {
	files, count, err := s.buildFiles(eng, key)
	if err != nil {
		return SyncOutcome{Message: "生成同步文件失败：" + err.Error()}
	}
	if count == 0 && eng.RemoteHasData(ctx) {
		return SyncOutcome{Message: "本地集合为空、远端已有数据；已阻止推送，避免清空仓库"}
	}
	if prefs.LastSyncAt == 0 && eng.RemoteHasData(ctx) {
		return SyncOutcome{Message: "远端已有同步数据；请先拉取导入，再提交本地改动"}
	}
	stats, err := eng.WriteFiles(files)
	if err != nil {
		return SyncOutcome{Message: "写入同步目录失败：" + err.Error()}
	}
	if _, err := eng.Commit(ctx, fmt.Sprintf("api-z sync: %s %s", s.Hostname, time.Now().Format(time.RFC3339))); err != nil {
		return SyncOutcome{Message: "git 提交失败：" + gitMessage(err)}
	}
	headBefore := eng.Head(ctx)
	merged := false
	if _, err := eng.MergeRemote(ctx); err != nil {
		var ce *sync.ConflictError
		switch {
		case errors.Is(err, sync.ErrNoRemoteRef):
		case errors.As(err, &ce):
			return SyncOutcome{Message: "合并远端失败（冲突），已放弃本次提交：" + strings.Join(ce.Paths, ", ")}
		default:
			return SyncOutcome{Message: "合并远端失败：" + gitMessage(err)}
		}
	} else {
		merged = eng.Head(ctx) != headBefore
	}
	if merged {
		if _, err := s.importFiles(eng, key); err != nil {
			return SyncOutcome{Message: "导入合并结果失败：" + err.Error()}
		}
	}
	if err := eng.Git.Push(ctx, prefs.Branch); err != nil {
		return SyncOutcome{Message: pushMessage(err)}
	}
	eng.BindBranch(ctx)
	s.markSynced()
	LogInfo("sync", "push ok", "written", stats.Written, "deleted", stats.Deleted, "merged", merged)
	return SyncOutcome{OK: true, Written: stats.Written, Deleted: stats.Deleted,
		Applied: merged, Message: fmt.Sprintf("已提交并推送：写入 %d、删除 %d 个文件", stats.Written, stats.Deleted)}
}

func (s *SyncService) pull(ctx context.Context, eng *sync.Engine, prefs SyncPrefs, key []byte, cloned bool) SyncOutcome {
	if _, ok := eng.RemoteRef(ctx); !ok {
		eng.BindBranch(ctx)
		s.markSynced()
		return SyncOutcome{OK: true, Message: fmt.Sprintf("远端仓库为空，已克隆并绑定分支 %s；可点击「提交并推送」", prefs.Branch)}
	}
	if eng.Dirty(ctx) {
		return SyncOutcome{Message: "同步目录有未提交改动，请先点击「提交并推送」"}
	}
	if _, err := eng.MergeRemote(ctx); err != nil {
		var ce *sync.ConflictError
		if errors.As(err, &ce) {
			return SyncOutcome{Message: "合并冲突，已放弃本次拉取：" + strings.Join(ce.Paths, ", ")}
		}
		return SyncOutcome{Message: "拉取失败：" + gitMessage(err)}
	}
	eng.BindBranch(ctx)
	n, err := s.importFiles(eng, key)
	if err != nil {
		return SyncOutcome{Message: "导入本地失败：" + err.Error()}
	}
	s.markSynced()
	msg := "已拉取并导入"
	if cloned {
		msg = "已克隆仓库、检出分支并导入"
	}
	LogInfo("sync", "pull ok", "applied", n)
	return SyncOutcome{OK: true, Applied: true, Message: fmt.Sprintf("%s：更新 %d 个集合", msg, n)}
}

func (s *SyncService) buildFiles(eng *sync.Engine, key []byte) (map[string][]byte, int, error) {
	files := map[string][]byte{}
	tree, err := s.Store.LoadTree()
	if err != nil {
		return nil, 0, err
	}
	for _, col := range tree.Collections {
		dir := sync.AppDir + "/collections/" + col.ID
		var folders []syncFolderRef
		folderMeta := map[string]string{}
		for _, f := range tree.Folders {
			if f.CollectionID != col.ID {
				continue
			}
			folders = append(folders, syncFolderRef{ID: f.ID, Name: f.Name, ParentID: f.ParentID, SortOrder: f.SortOrder})
			if strings.TrimSpace(f.MetaJSON) != "" && strings.TrimSpace(f.MetaJSON) != "{}" {
				folderMeta[f.ID] = f.MetaJSON
			}
		}
		sort.Slice(folders, func(i, j int) bool { return folders[i].ID < folders[j].ID })
		var requests []syncRequestRef
		for _, r := range tree.Requests {
			if r.CollectionID != col.ID {
				continue
			}
			requests = append(requests, syncRequestRef{ID: r.ID, Name: r.Name, FolderID: r.FolderID, SortOrder: r.SortOrder})
			plain, err := json.MarshalIndent(syncRequestFile{
				ID: r.ID, Name: r.Name, Method: r.Method, URL: r.URL,
				HeadersText: r.HeadersText, ParamsText: r.ParamsText,
				BodyText: r.BodyText, MetaJSON: r.MetaJSON,
			}, "", "  ")
			if err != nil {
				return nil, 0, err
			}
			rel := dir + "/requests/" + r.ID + ".enc"
			enc, err := s.encryptChanged(eng, rel, key, append(plain, '\n'))
			if err != nil {
				return nil, 0, err
			}
			files[rel] = enc
		}
		sort.Slice(requests, func(i, j int) bool { return requests[i].ID < requests[j].ID })
		structData, err := json.MarshalIndent(syncStructure{
			FormatVersion: 2, ID: col.ID, Name: col.Name, Folders: folders, Requests: requests,
		}, "", "  ")
		if err != nil {
			return nil, 0, err
		}
		files[dir+"/collection.json"] = append(structData, '\n')
		settingsData, err := json.MarshalIndent(syncSettings{
			CollectionMeta: col.MetaJSON, FolderMeta: folderMeta,
		}, "", "  ")
		if err != nil {
			return nil, 0, err
		}
		encSettings, err := s.encryptChanged(eng, dir+"/settings.enc", key, append(settingsData, '\n'))
		if err != nil {
			return nil, 0, err
		}
		files[dir+"/settings.enc"] = encSettings
	}
	env, err := s.Store.LoadEnvironments()
	if err != nil {
		return nil, 0, err
	}
	envData, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return nil, 0, err
	}
	envEnc, err := s.encryptChanged(eng, sync.AppDir+"/env/environments.enc", key, append(envData, '\n'))
	if err != nil {
		return nil, 0, err
	}
	files[sync.AppDir+"/env/environments.enc"] = envEnc
	files[sync.KCVFile] = []byte(sync.KCV(key) + "\n")
	return files, len(tree.Collections), nil
}

// encryptChanged reuses the existing ciphertext when the plaintext is
// unchanged, so a no-op push does not rewrite every entity file.
func (s *SyncService) encryptChanged(eng *sync.Engine, rel string, key, plain []byte) ([]byte, error) {
	path := filepath.Join(eng.WorkTree, filepath.FromSlash(rel))
	if old, err := os.ReadFile(path); err == nil {
		if decoded, err := sync.Decrypt(key, strings.TrimSpace(string(old))); err == nil && bytes.Equal(decoded, plain) {
			return old, nil
		}
	}
	enc, err := sync.Encrypt(key, plain)
	if err != nil {
		return nil, err
	}
	return []byte(enc + "\n"), nil
}

func (s *SyncService) importFiles(eng *sync.Engine, key []byte) (int, error) {
	files, err := eng.ReadFiles()
	if err != nil {
		return 0, err
	}
	applied := 0
	for rel, raw := range files {
		if !strings.HasPrefix(rel, sync.AppDir+"/collections/") || !strings.HasSuffix(rel, "/collection.json") {
			continue
		}
		var structure syncStructure
		if err := json.Unmarshal(raw, &structure); err != nil {
			return applied, fmt.Errorf("%s: %w", rel, err)
		}
		dir := strings.TrimSuffix(rel, "/collection.json")
		id := strings.TrimPrefix(dir, sync.AppDir+"/collections/")
		if id == "" {
			continue
		}
		var settings syncSettings
		settingsRaw, ok := files[dir+"/settings.enc"]
		if !ok {
			return applied, fmt.Errorf("%s/settings.enc 缺失", dir)
		}
		plain, err := sync.Decrypt(key, strings.TrimSpace(string(settingsRaw)))
		if err != nil {
			return applied, fmt.Errorf("%s/settings.enc: %w", dir, err)
		}
		if err := json.Unmarshal(plain, &settings); err != nil {
			return applied, fmt.Errorf("%s/settings.enc: %w", dir, err)
		}
		portable := PortableCollection{ID: id, Name: structure.Name, MetaJSON: settings.CollectionMeta}
		folderNodes := map[string]*PortableFolder{}
		for _, f := range structure.Folders {
			folderNodes[f.ID] = &PortableFolder{ID: f.ID, Name: f.Name, MetaJSON: settings.FolderMeta[f.ID], SortOrder: f.SortOrder}
		}
		for _, ref := range structure.Requests {
			rawReq, ok := files[dir+"/requests/"+ref.ID+".enc"]
			if !ok {
				return applied, fmt.Errorf("%s/requests/%s.enc 缺失", dir, ref.ID)
			}
			reqPlain, err := sync.Decrypt(key, strings.TrimSpace(string(rawReq)))
			if err != nil {
				return applied, fmt.Errorf("%s/requests/%s.enc: %w", dir, ref.ID, err)
			}
			var rf syncRequestFile
			if err := json.Unmarshal(reqPlain, &rf); err != nil {
				return applied, fmt.Errorf("%s/requests/%s.enc: %w", dir, ref.ID, err)
			}
			pr := PortableRequest{ID: rf.ID, Name: rf.Name, Method: rf.Method, URL: rf.URL,
				HeadersText: rf.HeadersText, ParamsText: rf.ParamsText, BodyText: rf.BodyText,
				MetaJSON: rf.MetaJSON, SortOrder: ref.SortOrder}
			if ref.FolderID == "" {
				portable.Requests = append(portable.Requests, pr)
			} else if parent := folderNodes[ref.FolderID]; parent != nil {
				parent.Requests = append(parent.Requests, pr)
			}
		}
		var buildFolder func(id string) PortableFolder
		buildFolder = func(id string) PortableFolder {
			node := folderNodes[id]
			out := *node
			for _, f := range structure.Folders {
				if f.ParentID == id {
					out.Folders = append(out.Folders, buildFolder(f.ID))
				}
			}
			return out
		}
		for _, f := range structure.Folders {
			if f.ParentID == "" {
				portable.Folders = append(portable.Folders, buildFolder(f.ID))
			}
		}
		sortPortableFolders(&portable)
		exists, err := s.Store.CollectionExists(id)
		if err != nil {
			return applied, err
		}
		if exists {
			err = s.Store.MergePortableCollection(id, portable)
		} else {
			err = s.Store.ImportPortableCollectionWithID(id, portable)
		}
		if err != nil {
			return applied, err
		}
		applied++
	}
	if raw, ok := files[sync.AppDir+"/env/environments.enc"]; ok {
		plain, err := sync.Decrypt(key, strings.TrimSpace(string(raw)))
		if err == nil {
			var incoming EnvState
			if json.Unmarshal(plain, &incoming) == nil {
				current, _ := s.Store.LoadEnvironments()
				_ = s.Store.SaveEnvironments(mergeEnvStates(current, incoming))
			}
		}
	}
	return applied, nil
}

func sortPortableFolders(p *PortableCollection) {
	sort.SliceStable(p.Folders, func(i, j int) bool { return p.Folders[i].SortOrder < p.Folders[j].SortOrder })
	for i := range p.Folders {
		sortPortableFolder(&p.Folders[i])
	}
	sort.SliceStable(p.Requests, func(i, j int) bool { return p.Requests[i].SortOrder < p.Requests[j].SortOrder })
}

func sortPortableFolder(f *PortableFolder) {
	sort.SliceStable(f.Folders, func(i, j int) bool { return f.Folders[i].SortOrder < f.Folders[j].SortOrder })
	for i := range f.Folders {
		sortPortableFolder(&f.Folders[i])
	}
	sort.SliceStable(f.Requests, func(i, j int) bool { return f.Requests[i].SortOrder < f.Requests[j].SortOrder })
}

func (s *SyncService) markSynced() {
	prefs, _ := LoadSyncPrefs(s.PrefsPath)
	prefs.LastSyncAt = time.Now().UnixMilli()
	_ = SaveSyncPrefs(s.PrefsPath, prefs)
}

func gitMessage(err error) string {
	var ge *sync.GitError
	if errors.As(err, &ge) {
		return ge.Out
	}
	return err.Error()
}

func pushMessage(err error) string {
	msg := gitMessage(err)
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "non-fast-forward") || strings.Contains(lower, "fetch first") ||
		strings.Contains(lower, "rejected") {
		return "推送被远端拒绝：请先拉取合并后再推送"
	}
	if strings.Contains(lower, "authentication") || strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "could not read from remote repository") {
		return "推送失败：认证失败，请检查仓库地址/SSH 私钥"
	}
	return "推送失败：" + msg
}
