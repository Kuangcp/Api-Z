package sync

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AppDir is this app's namespace in a shared sync repository.
const AppDir = "api-z"

const KCVFile = AppDir + "/key.kcv"

// ErrKeyMismatch is returned when the local key does not match the repository.
var ErrKeyMismatch = errors.New("同步密钥与仓库不匹配")

// ErrRepoKeyMissing is returned when encrypted files exist without a KCV.
var ErrRepoKeyMissing = errors.New("仓库缺少 api-z/key.kcv，无法验证同步密钥")

// Engine moves files between the work tree and git; it does not know app data.
type Engine struct {
	WorkTree string
	Remote   string
	Branch   string
	SSHKey   string
	Git      *Git
}

func NewEngine(workTree, remote, branch, sshKey string) *Engine {
	if branch == "" {
		branch = "main"
	}
	return &Engine{WorkTree: workTree, Remote: remote, Branch: branch, SSHKey: sshKey,
		Git: NewGit(workTree, sshKey)}
}

func (e *Engine) EnsureRepo(ctx context.Context) (cloned bool, err error) {
	if _, statErr := os.Stat(filepath.Join(e.WorkTree, ".git")); statErr != nil {
		if err := e.Git.Clone(ctx, e.Remote); err != nil {
			return false, err
		}
		cloned = true
	}
	e.Git.SetIdentity(ctx)
	if err := e.Git.SetRemote(ctx, e.Remote); err != nil {
		return cloned, err
	}
	if err := e.Git.Fetch(ctx, e.Branch); err != nil && !errors.Is(err, ErrNoRemoteRef) {
		return cloned, err
	}
	if err := e.Git.CheckoutBranch(ctx, e.Branch); err != nil {
		return cloned, err
	}
	e.BindBranch(ctx)
	return cloned, nil
}

func (e *Engine) Head(ctx context.Context) string          { return e.Git.Head(ctx) }
func (e *Engine) Dirty(ctx context.Context) bool           { return e.Git.Dirty(ctx) }
func (e *Engine) HasLocalCommits(ctx context.Context) bool { return e.Git.HasCommits(ctx) }

func (e *Engine) BindBranch(ctx context.Context) {
	if cur, err := e.Git.CurrentBranch(ctx); err == nil && cur != "" && cur != e.Branch && !e.Git.HasCommits(ctx) {
		_ = e.Git.SymbolicRef(ctx, "refs/heads/"+e.Branch)
	}
	e.Git.SetBranchTracking(ctx, e.Branch, "origin")
}

func (e *Engine) RemoteRef(ctx context.Context) (string, bool) {
	return e.Git.RemoteRef(ctx, e.Branch)
}

func (e *Engine) RemoteHasData(ctx context.Context) bool {
	ref, ok := e.Git.RemoteRef(ctx, e.Branch)
	if !ok {
		return false
	}
	return e.Git.RemoteHasEntity(ctx, ref, AppDir)
}

func (e *Engine) RepoKCV(ctx context.Context) (string, bool) {
	if ref, ok := e.Git.RemoteRef(ctx, e.Branch); ok {
		if data, ok := e.Git.ShowFile(ctx, ref, KCVFile); ok {
			if kcv := strings.TrimSpace(string(data)); kcv != "" {
				return strings.ToUpper(kcv), true
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(e.WorkTree, filepath.FromSlash(KCVFile))); err == nil {
		if kcv := strings.TrimSpace(string(data)); kcv != "" {
			return strings.ToUpper(kcv), true
		}
	}
	return "", false
}

func (e *Engine) CheckKey(ctx context.Context, key []byte) error {
	repoKCV, has := e.RepoKCV(ctx)
	if has {
		if !KCVEqual(KCV(key), repoKCV) {
			return ErrKeyMismatch
		}
		return nil
	}
	hasEnc := false
	root := filepath.Join(e.WorkTree, filepath.FromSlash(AppDir))
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".enc") {
			hasEnc = true
		}
		return nil
	})
	if e.RemoteHasData(ctx) {
		hasEnc = true
	}
	if hasEnc {
		return ErrRepoKeyMissing
	}
	return nil
}

func (e *Engine) MergeRemote(ctx context.Context) ([]string, error) {
	ref, ok := e.Git.RemoteRef(ctx, e.Branch)
	if !ok {
		return nil, ErrNoRemoteRef
	}
	allowUnrelated := !e.Git.HasMergeBase(ctx, "HEAD", ref)
	if err := e.Git.Merge(ctx, ref, allowUnrelated); err != nil {
		paths := e.Git.Conflicted(ctx)
		e.Git.MergeAbort(ctx)
		if len(paths) > 0 {
			return paths, &ConflictError{Paths: paths, Err: err}
		}
		return nil, err
	}
	return nil, nil
}

// WriteFiles atomically writes files under api-z/ and deletes stale .enc/.json
// entity files, leaving unknown files and files of other apps untouched.
func (e *Engine) WriteFiles(files map[string][]byte) (WriteStats, error) {
	var st WriteStats
	for rel, data := range files {
		target := filepath.Join(e.WorkTree, filepath.FromSlash(rel))
		old, err := os.ReadFile(target)
		if err == nil && string(old) == string(data) {
			st.Unchanged++
			continue
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return st, err
		}
		if err := writeFileAtomic(target, data, 0o644); err != nil {
			return st, err
		}
		st.Written++
	}
	root := filepath.Join(e.WorkTree, filepath.FromSlash(AppDir))
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(e.WorkTree, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if _, ok := files[rel]; ok {
			return nil
		}
		if strings.HasSuffix(rel, ".enc") || strings.HasSuffix(rel, ".json") {
			if os.Remove(path) == nil {
				st.Deleted++
			}
		}
		return nil
	})
	return st, nil
}

// ReadFiles reads every file under api-z/ in the work tree.
func (e *Engine) ReadFiles() (map[string][]byte, error) {
	out := map[string][]byte{}
	root := filepath.Join(e.WorkTree, filepath.FromSlash(AppDir))
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil || d.IsDir() {
			return err
		}
		rel, rerr := filepath.Rel(e.WorkTree, path)
		if rerr != nil {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		out[filepath.ToSlash(rel)] = data
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	return out, err
}

func (e *Engine) RepoKCVFromFiles(files map[string][]byte) (string, bool) {
	data, ok := files[KCVFile]
	if !ok {
		return "", false
	}
	kcv := strings.TrimSpace(string(data))
	return strings.ToUpper(kcv), kcv != ""
}

func (e *Engine) Commit(ctx context.Context, msg string) (bool, error) {
	existing, _ := os.ReadFile(filepath.Join(e.WorkTree, ".gitattributes"))
	_ = writeFileAtomic(filepath.Join(e.WorkTree, ".gitattributes"), mergeGitAttributes(existing), 0o644)
	return e.Git.Commit(ctx, msg)
}

func mergeGitAttributes(existing []byte) []byte {
	need := []string{"*.enc -text"}
	lines := strings.Split(strings.ReplaceAll(string(existing), "\r\n", "\n"), "\n")
	seen := map[string]bool{}
	for _, line := range lines {
		seen[strings.TrimSpace(line)] = true
	}
	out := []string{}
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	for _, line := range need {
		if !seen[line] {
			out = append(out, line)
		}
	}
	sort.Strings(out)
	return []byte(strings.Join(out, "\n") + "\n")
}

type WriteStats struct {
	Written   int
	Unchanged int
	Deleted   int
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".api-z-sync-*.tmp")
	if err != nil {
		return os.WriteFile(path, data, mode)
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return os.WriteFile(path, data, mode)
	}
	return nil
}
