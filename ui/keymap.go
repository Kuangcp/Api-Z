package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

type keyAction struct {
	ID      string
	Label   string
	Default string
}

var keyActions = []keyAction{
	{"send", "发送请求", "Ctrl+Enter"},
	{"save", "保存请求", "Ctrl+S"},
	{"search", "全局搜索", "Ctrl+K"},
	{"recent", "最近请求切换", "Ctrl+Tab"},
	{"new_request", "新建请求", "Ctrl+N"},
	{"close_tab", "关闭当前标签", "Ctrl+W"},
	{"settings", "打开设置", "Ctrl+,"},
}

func defaultKeymap() map[string]string {
	m := make(map[string]string, len(keyActions))
	for _, a := range keyActions {
		m[a.ID] = a.Default
	}
	return m
}

func mergedKeymap(saved map[string]string) map[string]string {
	m := defaultKeymap()
	for id, chord := range saved {
		if _, ok := m[id]; !ok {
			continue
		}
		if _, _, ok := parseChord(chord); ok {
			m[id] = chord
		}
	}
	return m
}

var keyByName = map[string]ui.Key{}
var nameByKey = map[ui.Key]string{}

func init() {
	add := func(name string, k ui.Key) {
		keyByName[strings.ToLower(name)] = k
		nameByKey[k] = name
	}
	letters := []struct {
		name string
		k    ui.Key
	}{
		{"A", ui.KeyA}, {"B", ui.KeyB}, {"C", ui.KeyC}, {"D", ui.KeyD},
		{"E", ui.KeyE}, {"F", ui.KeyF}, {"G", ui.KeyG}, {"H", ui.KeyH},
		{"I", ui.KeyI}, {"J", ui.KeyJ}, {"K", ui.KeyK}, {"L", ui.KeyL},
		{"M", ui.KeyM}, {"N", ui.KeyN}, {"O", ui.KeyO}, {"P", ui.KeyP},
		{"Q", ui.KeyQ}, {"R", ui.KeyR}, {"S", ui.KeyS}, {"T", ui.KeyT},
		{"U", ui.KeyU}, {"V", ui.KeyV}, {"W", ui.KeyW}, {"X", ui.KeyX},
		{"Y", ui.KeyY}, {"Z", ui.KeyZ},
		{"0", ui.Key0}, {"1", ui.Key1}, {"2", ui.Key2}, {"3", ui.Key3},
		{"4", ui.Key4}, {"5", ui.Key5}, {"6", ui.Key6}, {"7", ui.Key7},
		{"8", ui.Key8}, {"9", ui.Key9},
		{"Enter", ui.KeyEnter}, {"Tab", ui.KeyTab}, {"Space", ui.KeySpace},
		{"Escape", ui.KeyEscape}, {"Backspace", ui.KeyBackspace}, {"Delete", ui.KeyDelete},
		{"Home", ui.KeyHome}, {"End", ui.KeyEnd}, {"PageUp", ui.KeyPageUp},
		{"PageDown", ui.KeyPageDown}, {"Up", ui.KeyUp}, {"Down", ui.KeyDown},
		{"Left", ui.KeyLeft}, {"Right", ui.KeyRight},
		{"F1", ui.KeyF1}, {"F2", ui.KeyF2}, {"F3", ui.KeyF3}, {"F4", ui.KeyF4},
		{"F5", ui.KeyF5}, {"F6", ui.KeyF6}, {"F7", ui.KeyF7}, {"F8", ui.KeyF8},
		{"F9", ui.KeyF9}, {"F10", ui.KeyF10}, {"F11", ui.KeyF11}, {"F12", ui.KeyF12},
		{"Comma", ui.KeyComma}, {"Period", ui.KeyPeriod}, {"Slash", ui.KeySlash},
		{"Semicolon", ui.KeySemicolon}, {"Minus", ui.KeyMinus}, {"Equal", ui.KeyEqual},
	}
	for _, l := range letters {
		add(l.name, l.k)
	}
	add(",", ui.KeyComma)
	add(".", ui.KeyPeriod)
	add("/", ui.KeySlash)
	add("-", ui.KeyMinus)
	add("=", ui.KeyEqual)
	recordableKeys = make([]ui.Key, 0, len(nameByKey))
	for k := range nameByKey {
		recordableKeys = append(recordableKeys, k)
	}
}

var recordableKeys []ui.Key

func parseChord(s string) (ui.Modifiers, ui.Key, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	parts := strings.Split(s, "+")
	key, ok := keyByName[strings.ToLower(strings.TrimSpace(parts[len(parts)-1]))]
	if !ok {
		return 0, 0, false
	}
	var mods ui.Modifiers
	for _, p := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "ctrl", "control":
			mods |= ui.Ctrl
		case "shift":
			mods |= ui.Shift
		case "alt", "option":
			mods |= ui.Alt
		case "super", "cmd", "command", "meta":
			mods |= ui.Super
		default:
			return 0, 0, false
		}
	}
	return mods, key, true
}

func formatChord(mods ui.Modifiers, key ui.Key) string {
	name, ok := nameByKey[key]
	if !ok {
		return ""
	}
	var parts []string
	if mods&ui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	if mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&ui.Super != 0 {
		parts = append(parts, "Super")
	}
	parts = append(parts, name)
	return strings.Join(parts, "+")
}

func detectChord(c *ui.Context) (string, bool) {
	mods := c.Modifiers()
	for _, k := range recordableKeys {
		if c.KeyDown(k) {
			chord := formatChord(mods, k)
			if chord != "" {
				return chord, true
			}
		}
	}
	return "", false
}

func (r *Root) handleShortcuts(c *ui.Context) {
	if r.settingsOpen || r.dialogOpen || r.searchOpen || r.moveOpen || r.envOpen || r.recording != "" {
		return
	}
	for _, a := range keyActions {
		mods, key, ok := parseChord(r.keymap[a.ID])
		if !ok {
			continue
		}
		if c.Shortcut(mods, key) {
			r.runAction(a.ID)
			return
		}
	}
}

func (r *Root) runAction(id string) {
	switch id {
	case "send":
		if ed := r.activeEditor(); ed != nil {
			ed.send()
		}
	case "save":
		if ed := r.activeEditor(); ed != nil {
			r.saveEditor(ed)
			r.status = "已保存"
		}
	case "search":
		r.searchOpen = true
		r.globalQuery = ""
	case "recent":
		r.switchRecent()
	case "new_request":
		r.newRequestFromSelection()
	case "close_tab":
		r.closeTabAt(r.active)
	case "settings":
		r.settingsOpen = true
	}
}

func (r *Root) activeEditor() *Editor {
	if r.active < 0 || r.active >= len(r.tabs) {
		return nil
	}
	return r.tabs[r.active]
}

func (r *Root) saveKeymap() {
	if err := app.SaveKeymap(r.keymapPath, app.KeymapPrefs(r.keymap)); err != nil {
		r.settingsErr = "保存快捷键失败: " + err.Error()
	} else {
		r.settingsErr = ""
	}
}

func (r *Root) handleRecording(c *ui.Context) {
	if r.recording == "" {
		return
	}
	if c.KeyDown(ui.KeyEscape) && c.Modifiers() == 0 {
		r.recording = ""
		r.recordMsg = "已取消"
		return
	}
	chord, ok := detectChord(c)
	if !ok {
		return
	}
	id := r.recording
	for other, ch := range r.keymap {
		if other != id && strings.EqualFold(ch, chord) {
			delete(r.keymap, other)
		}
	}
	r.keymap[id] = chord
	r.saveKeymap()
	r.recording = ""
	r.recordMsg = "已设置为 " + chord
}
