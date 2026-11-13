package ui

import (
	"fmt"
	"strings"

	"api-z/app"
)

func (r *Root) pushDataDir() {
	if r.win == nil {
		r.finishPush(r.store.PushDataDir(r.paths))
		return
	}
	win := r.win
	app.SafeGo("ui", func() {
		res, err := r.store.PushDataDir(r.paths)
		win.Update(func() { r.finishPush(res, err) })
	})
}

func (r *Root) finishPush(res app.DataPushResult, err error) {
	if err != nil {
		r.err = "Push 失败: " + err.Error()
		return
	}
	r.err = ""
	r.status = fmt.Sprintf("已 Push %d 个集合到 %s", res.Collections, app.DataRoot(r.paths))
}

func (r *Root) pullDataDir() {
	if r.win == nil {
		r.finishPull(r.store.PullDataDir(r.paths))
		return
	}
	win := r.win
	app.SafeGo("ui", func() {
		res, err := r.store.PullDataDir(r.paths)
		win.Update(func() { r.finishPull(res, err) })
	})
}

func (r *Root) finishPull(res app.DataPullResult, err error) {
	if err != nil {
		r.err = "Pull 失败: " + err.Error()
		return
	}
	r.reloadTree()
	if res.EnvMerged {
		r.reloadEnv()
	}
	r.status = fmt.Sprintf("已从 data/ 拉取：新增 %d / 合并 %d", res.Created, res.Merged)
	if len(res.FileErrs) > 0 {
		r.err = fmt.Sprintf("%d 个文件失败: %s", len(res.FileErrs),
			strings.Join(res.FileErrs[:min(3, len(res.FileErrs))], "；"))
	} else {
		r.err = ""
	}
}

func (r *Root) reloadEnv() {
	env, err := r.store.LoadEnvironments()
	if err != nil {
		r.err = "读取环境失败: " + err.Error()
		return
	}
	r.env = env
	r.envNames = envNames(env)
	if active := env.Active(); active != nil {
		r.envName = active.Name
	} else {
		r.envName = ""
	}
}
