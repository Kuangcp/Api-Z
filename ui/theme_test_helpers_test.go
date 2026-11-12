package ui

import "github.com/egoist/mygo/ui"

func uiLight() *ui.Theme { return ui.LightTheme() }

func hexColor(s string) ui.Color { return ui.Hex(s) }
