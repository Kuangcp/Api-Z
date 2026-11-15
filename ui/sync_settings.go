package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func (r *Root) syncSettings(c *ui.Context, t *ui.Theme) {
	_, hasKey := app.LoadSyncKey(r.syncPath)
	ui.Scroll(c).Height(480).Children(func() {
		ui.Text(c, "Git 同步").FontSize(13).Bold()
		ui.Text(c, "与其他设备同步集合与环境；共享仓库中只写 api-z/ 目录，不修改根目录 README。").
			FontSize(11).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "仓库地址").Width(84).TextColor(t.TextMuted)
			if ui.TextInput(c, &r.syncPrefs.RepoURL).Placeholder("git@github.com:me/api-sync.git").Grow(1).Changed() {
				r.saveSyncPrefs()
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "分支").Width(84).TextColor(t.TextMuted)
			if ui.TextInput(c, &r.syncPrefs.Branch).Placeholder("main").Width(160).Changed() {
				r.saveSyncPrefs()
			}
			ui.Text(c, "SSH 私钥").TextColor(t.TextMuted)
			if ui.TextInput(c, &r.syncPrefs.SSHKeyPath).Placeholder("可选，如 ~/.ssh/id_ed25519").Grow(1).Changed() {
				r.saveSyncPrefs()
			}
		})
		ui.Text(c, "同步目录："+app.SyncWorkTree(r.syncPath)).FontSize(10.5).
			TextColor(t.TextMuted).SingleLine().Tooltip(app.SyncWorkTree(r.syncPath))

		ui.Text(c, "同步密钥").FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if hasKey {
				ui.Text(c, "已配置").FontSize(11.5).TextColor(t.Success)
			} else {
				ui.Text(c, "未配置").FontSize(11.5).TextColor(t.Warning)
			}
			ui.TextInput(c, &r.syncKeyInput).Placeholder("粘贴密钥（base64 或 64 位 hex）").
				Password().Grow(1)
			if ui.Button(c, "生成").Clicked() {
				svc := app.NewSyncService(r.store, r.paths, r.syncPath)
				if text, err := svc.GenerateKey(); err != nil {
					r.syncErr = "生成密钥失败: " + err.Error()
				} else {
					r.syncKeyInput = text
					c.WriteClipboard(text)
					r.syncMsg = "已生成同步密钥并复制到剪贴板"
					r.syncErr = ""
				}
			}
			if ui.Button(c, "复制").Disabled(!hasKey).Clicked() {
				svc := app.NewSyncService(r.store, r.paths, r.syncPath)
				if text, err := svc.CopyKey(); err != nil {
					r.syncErr = err.Error()
				} else {
					c.WriteClipboard(text)
					r.syncMsg = "已复制同步密钥"
					r.syncErr = ""
				}
			}
		})
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "导入密钥").Disabled(strings.TrimSpace(r.syncKeyInput) == "").Clicked() {
				svc := app.NewSyncService(r.store, r.paths, r.syncPath)
				if err := svc.ImportKey(r.syncKeyInput); err != nil {
					r.syncErr = "导入密钥失败: " + err.Error()
				} else {
					r.syncMsg = "同步密钥已导入"
					r.syncErr = ""
					r.syncKeyInput = ""
				}
			}
			if ui.Button(c, "清除密钥").Disabled(!hasKey).Clicked() {
				svc := app.NewSyncService(r.store, r.paths, r.syncPath)
				if err := svc.ClearKey(); err != nil {
					r.syncErr = "清除密钥失败: " + err.Error()
				} else {
					r.syncMsg = "已清除本机同步密钥"
					r.syncErr = ""
				}
			}
		})
		ui.Text(c, "同步密钥只保存在本机；仓库中只有 KCV 校验值。").
			FontSize(10.5).TextColor(t.TextMuted)

		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "拉取并导入").Disabled(r.syncBusy).Clicked() {
				r.runSync(c, "pull")
			}
			if ui.PrimaryButton(c, "提交并推送").Disabled(r.syncBusy).Clicked() {
				r.runSync(c, "push")
			}
			if r.syncBusy {
				ui.Text(c, "同步中…").FontSize(11).TextColor(t.TextMuted)
			}
			if last := r.syncPrefs.LastSyncAt; last > 0 {
				ui.Text(c, "上次同步 "+fmt.Sprint(timeUnix(last))).FontSize(10.5).TextColor(t.TextMuted)
			}
		})
		if r.syncErr != "" {
			ui.Text(c, r.syncErr).FontSize(11.5).TextColor(t.Danger)
		} else if r.syncMsg != "" {
			ui.Text(c, r.syncMsg).FontSize(11.5).TextColor(t.TextMuted)
		}
	})
}

func timeUnix(ms int64) string {
	return time.UnixMilli(ms).Format("01-02 15:04")
}

func (r *Root) saveSyncPrefs() {
	prefs, _ := app.LoadSyncPrefs(r.syncPath)
	prefs.RepoURL = strings.TrimSpace(r.syncPrefs.RepoURL)
	prefs.Branch = strings.TrimSpace(r.syncPrefs.Branch)
	prefs.SSHKeyPath = strings.TrimSpace(r.syncPrefs.SSHKeyPath)
	if prefs.Branch == "" {
		prefs.Branch = "main"
	}
	if err := app.SaveSyncPrefs(r.syncPath, prefs); err != nil {
		r.syncErr = "保存同步配置失败: " + err.Error()
		return
	}
	r.syncPrefs = prefs
}

func (r *Root) runSync(c *ui.Context, action string) {
	r.saveSyncPrefs()
	svc := app.NewSyncService(r.store, r.paths, r.syncPath)
	r.syncBusy = true
	r.syncMsg = "同步中…"
	r.syncErr = ""
	if r.win == nil {
		r.applySyncOutcome(svc.Run(context.Background(), action))
		return
	}
	win := r.win
	app.SafeGo("sync", func() {
		out := svc.Run(context.Background(), action)
		win.Update(func() { r.applySyncOutcome(out) })
	})
}

func (r *Root) applySyncOutcome(out app.SyncOutcome) {
	r.syncBusy = false
	if !out.OK {
		r.syncErr = out.Message
		r.syncMsg = ""
		if out.NeedsKey {
			r.syncErr = out.Message + "：请生成或导入同步密钥"
		}
		return
	}
	r.syncErr = ""
	r.syncMsg = out.Message
	if out.Applied {
		r.reloadTree()
		r.reloadEnv()
	}
	r.syncPrefs, _ = app.LoadSyncPrefs(r.syncPath)
}
