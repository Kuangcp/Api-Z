package ui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func (ed *Editor) binaryView(c *ui.Context, snap app.SessionSnapshot) {
	t := c.Theme()
	name := snap.FileName
	if name == "" {
		name = "response.bin"
	}
	ui.Column(c).Fill().Padding(16).Gap(8).Children(func() {
		ui.Text(c, "二进制响应").FontSize(14).Bold()
		ui.Text(c, name).FontSize(13)
		if snap.ContentType != "" {
			ui.Text(c, snap.ContentType).FontSize(11.5).TextColor(t.TextMuted)
		}
		ui.Text(c, app.FormatBytes(snap.Bytes)).FontSize(11.5).TextColor(t.TextMuted)
		ui.Text(c, snap.SpoolPath).FontSize(11).TextColor(t.TextMuted).Selectable().
			Tooltip(snap.SpoolPath)
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "保存到文件").Clicked() {
				ed.saveBinary(snap, name)
			}
			if ui.Button(c, "打开").Clicked() {
				ed.openBinary(snap, name)
			}
		})
	})
}

func (ed *Editor) saveBinary(snap app.SessionSnapshot, name string) {
	if snap.SpoolPath == "" {
		return
	}
	win := ed.root.win
	if win == nil {
		ed.root.err = "保存二进制响应失败: 没有窗口"
		return
	}
	app.SafeGo("ui", func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      win,
			Title:       "保存二进制响应",
			DefaultPath: name,
		})
		if err != nil || path == "" {
			return
		}
		if err := app.CopyFile(snap.SpoolPath, path); err != nil {
			win.Update(func() { ed.root.err = "保存二进制响应失败: " + err.Error() })
			return
		}
		win.Update(func() {
			ed.root.err = ""
			ed.root.status = "已保存二进制响应: " + path
		})
	})
}

func (ed *Editor) openBinary(snap app.SessionSnapshot, name string) {
	if snap.SpoolPath == "" {
		return
	}
	dir, err := os.MkdirTemp("", "api-z-binary-")
	if err != nil {
		ed.root.err = "打开二进制响应失败: " + err.Error()
		return
	}
	dst := filepath.Join(dir, filepath.Base(name))
	if err := app.CopyFile(snap.SpoolPath, dst); err != nil {
		ed.root.err = "打开二进制响应失败: " + err.Error()
		return
	}
	if err := mygo.Shell.OpenPath(dst); err != nil {
		ed.root.err = fmt.Sprintf("打开二进制响应失败: %v", err)
		return
	}
	ed.root.err = ""
}
