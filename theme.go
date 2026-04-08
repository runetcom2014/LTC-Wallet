package main

import (
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// runicTheme — стандартная тема с поддержкой рунических символов
type runicTheme struct {
	fyne.Theme
	runicFont fyne.Resource
}

func newRunicTheme() fyne.Theme {
	base := theme.DefaultTheme()

	// пробуем загрузить шрифт с поддержкой рун рядом с exe
	fontNames := []string{
		"NotoSansRunic-Regular.ttf",
		"NotoSans-Regular.ttf",
		// Segoe UI Symbol — системный шрифт Windows с рунами
		`C:\Windows\Fonts\seguisym.ttf`,
		`C:\Windows\Fonts\segoeui.ttf`,
	}

	for _, name := range fontNames {
		data, err := os.ReadFile(name)
		if err == nil && len(data) > 0 {
			return &runicTheme{
				Theme:     base,
				runicFont: fyne.NewStaticResource(name, data),
			}
		}
	}

	// шрифт не найден — возвращаем стандартную тему
	return base
}

func (t *runicTheme) Font(style fyne.TextStyle) fyne.Resource {
	if t.runicFont != nil {
		return t.runicFont
	}
	return t.Theme.Font(style)
}