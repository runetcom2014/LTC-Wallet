package main

import (
	"fmt"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// ═══════════════════════════════════════════════════════════════
// EmojiButton — кнопка с большим эмодзи
// ═══════════════════════════════════════════════════════════════

type EmojiButton struct {
	widget.BaseWidget
	Emoji   string
	OnTap   func()
	hovered bool
}

func NewEmojiButton(emoji string, onTap func()) *EmojiButton {
	b := &EmojiButton{Emoji: emoji, OnTap: onTap}
	b.ExtendBaseWidget(b)
	return b
}

func (b *EmojiButton) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.NRGBA{R: 50, G: 50, B: 70, A: 255})
	bg.CornerRadius = 6
	text := canvas.NewText(b.Emoji, color.White)
	text.TextSize = 28
	text.Alignment = fyne.TextAlignCenter
	return &emojiBtnRenderer{btn: b, bg: bg, text: text}
}

func (b *EmojiButton) Tapped(_ *fyne.PointEvent)          { if b.OnTap != nil { b.OnTap() } }
func (b *EmojiButton) TappedSecondary(_ *fyne.PointEvent) {}
func (b *EmojiButton) MouseIn(_ *desktop.MouseEvent)      { b.hovered = true; b.Refresh() }
func (b *EmojiButton) MouseOut()                          { b.hovered = false; b.Refresh() }
func (b *EmojiButton) MouseMoved(_ *desktop.MouseEvent)   {}

type emojiBtnRenderer struct {
	btn  *EmojiButton
	bg   *canvas.Rectangle
	text *canvas.Text
}

func (r *emojiBtnRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	r.bg.Move(fyne.NewPos(0, 0))
	r.text.Resize(fyne.NewSize(size.Width, r.text.TextSize*1.4))
	r.text.Move(fyne.NewPos(0, (size.Height-r.text.TextSize*1.4)/2))
}
func (r *emojiBtnRenderer) MinSize() fyne.Size { return fyne.NewSize(52, 52) }
func (r *emojiBtnRenderer) Refresh() {
	if r.btn.hovered {
		r.bg.FillColor = color.NRGBA{R: 80, G: 80, B: 120, A: 255}
	} else {
		r.bg.FillColor = color.NRGBA{R: 50, G: 50, B: 70, A: 255}
	}
	r.bg.Refresh()
	r.text.Text = r.btn.Emoji
	r.text.Refresh()
}
func (r *emojiBtnRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.bg, r.text} }
func (r *emojiBtnRenderer) Destroy()                     {}

// ═══════════════════════════════════════════════════════════════
// ConstellationWidget — звёздное небо
// ═══════════════════════════════════════════════════════════════
type ConstellationWidget struct {
	widget.BaseWidget

	Stars    []StarData
	Rotation int
	Steps    int
	Selected []bool // len == len(Stars)

	OnToggle func(idx int) // вызывается при клике на звезду
}

func NewConstellationWidget(stars []StarData, steps int, onToggle func(int)) *ConstellationWidget {
	w := &ConstellationWidget{
		Stars:    stars,
		Steps:    steps,
		Selected: make([]bool, len(stars)),
		OnToggle: onToggle,
	}
	w.ExtendBaseWidget(w)
	return w
}

func (w *ConstellationWidget) CreateRenderer() fyne.WidgetRenderer {
	r := &constellationRenderer{widget: w}
	r.bg = canvas.NewRectangle(color.NRGBA{R: 8, G: 8, B: 24, A: 255})
	r.rebuild()
	return r
}

// Tappable — обрабатываем клик
func (w *ConstellationWidget) Tapped(ev *fyne.PointEvent) {
	size := w.Size()
	side := size.Width
	if size.Height < side {
		side = size.Height
	}

	for i, star := range w.rotatedStars() {
		cx, cy := star.X*float64(side), star.Y*float64(side)
		r := starRadius(star, float64(side))
		hit := float64(r) + 6
		dx := float64(ev.Position.X) - cx
		dy := float64(ev.Position.Y) - cy
		if math.Sqrt(dx*dx+dy*dy) < hit {
			if w.OnToggle != nil {
				w.OnToggle(i)
			}
			return
		}
	}
}

func (w *ConstellationWidget) TappedSecondary(_ *fyne.PointEvent) {}

// rotatedStars возвращает звёзды с применённым поворотом
func (w *ConstellationWidget) rotatedStars() []StarData {
	steps := w.Steps
	if steps == 0 {
		steps = 18
	}
	angle := float64(w.Rotation) * (2 * math.Pi / float64(steps))
	cos, sin := math.Cos(angle), math.Sin(angle)

	result := make([]StarData, len(w.Stars))
	for i, s := range w.Stars {
		dx, dy := s.X-0.5, s.Y-0.5
		result[i] = StarData{
			X:     0.5 + dx*cos - dy*sin,
			Y:     0.5 + dx*sin + dy*cos,
			Size:  s.Size,
			Color: s.Color,
			Name:  s.Name,
		}
	}
	return result
}

// ─── renderer ────────────────────────────────────────────────

type constellationRenderer struct {
	widget  *ConstellationWidget
	bg      *canvas.Rectangle
	circles []*canvas.Circle
	labels  []*canvas.Text
	objects []fyne.CanvasObject
}

func (r *constellationRenderer) rebuild() {
	n := len(r.widget.Stars)
	r.circles = make([]*canvas.Circle, n)
	r.labels = make([]*canvas.Text, n)

	r.objects = []fyne.CanvasObject{r.bg}
	for i := range r.widget.Stars {
		c := canvas.NewCircle(color.White)
		t := canvas.NewText("", color.White)
		t.TextSize = 9
		t.Alignment = fyne.TextAlignCenter
		r.circles[i] = c
		r.labels[i] = t
		r.objects = append(r.objects, c, t)
	}
}

func (r *constellationRenderer) Layout(size fyne.Size) {
	// квадратное поле — берём меньшую сторону
	side := size.Width
	if size.Height < side {
		side = size.Height
	}
	r.bg.Resize(fyne.NewSize(side, side))
	r.bg.Move(fyne.NewPos(0, 0))

	rotated := r.widget.rotatedStars()
	for i, star := range rotated {
		cx := float32(star.X) * side
		cy := float32(star.Y) * side
		rad := starRadius(star, float64(side))

		sel := r.widget.Selected[i]
		col := starColor(star.Color, sel)

		r.circles[i].FillColor = col
		if sel {
			r.circles[i].StrokeColor = color.NRGBA{R: 255, G: 255, B: 100, A: 255}
			r.circles[i].StrokeWidth = 2
		} else {
			r.circles[i].StrokeWidth = 0
		}
		r.circles[i].Move(fyne.NewPos(cx-rad, cy-rad))
		r.circles[i].Resize(fyne.NewSize(rad*2, rad*2))

		r.labels[i].Text = star.Name
		if sel {
			r.labels[i].Color = color.NRGBA{R: 255, G: 255, B: 100, A: 255}
		} else {
			r.labels[i].Color = color.NRGBA{R: 180, G: 180, B: 220, A: 160}
		}
		r.labels[i].Move(fyne.NewPos(cx-30, cy+rad+1))
		r.labels[i].Resize(fyne.NewSize(60, 14))
		r.labels[i].Refresh()
	}
}

func (r *constellationRenderer) MinSize() fyne.Size {
	return fyne.NewSize(320, 320)
}

func (r *constellationRenderer) Refresh() {
	r.Layout(r.widget.Size())
	canvas.Refresh(r.widget)
}

func (r *constellationRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *constellationRenderer) Destroy() {}

// ─── helpers ─────────────────────────────────────────────────

func starRadius(star StarData, canvasW float64) float32 {
	base := canvasW / 120.0
	switch star.Size {
	case "large":
		return float32(base * 2.2)
	case "medium":
		return float32(base * 1.5)
	default:
		return float32(base * 1.0)
	}
}

func starColor(colorName string, selected bool) color.Color {
	if selected {
		return color.NRGBA{R: 255, G: 255, B: 80, A: 255}
	}
	switch colorName {
	case "blue":
		return color.NRGBA{R: 120, G: 160, B: 255, A: 255}
	case "yellow":
		return color.NRGBA{R: 255, G: 240, B: 120, A: 255}
	case "orange":
		return color.NRGBA{R: 255, G: 180, B: 80, A: 255}
	case "red":
		return color.NRGBA{R: 255, G: 100, B: 80, A: 255}
	default: // white
		return color.NRGBA{R: 230, G: 230, B: 255, A: 255}
	}
}
// ═══════════════════════════════════════════════════════════════
// RuneButton — кнопка с SVG-рендерингом руны
// ═══════════════════════════════════════════════════════════════

type RuneButton struct {
	widget.BaseWidget
	SVGPath  string // d-атрибут SVG path
	Name     string // название руны
	Selected bool
	OnTap    func()
	hovered  bool
}

func NewRuneButton(svgPath, name string, onTap func()) *RuneButton {
	b := &RuneButton{SVGPath: svgPath, Name: name, OnTap: onTap}
	b.ExtendBaseWidget(b)
	return b
}

func (b *RuneButton) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.NRGBA{R: 50, G: 50, B: 70, A: 255})
	bg.CornerRadius = 6
	img := canvas.NewImageFromResource(nil)
	img.FillMode = canvas.ImageFillContain
	text := canvas.NewText(b.Name, color.White)
	text.TextSize = 14
	text.Alignment = fyne.TextAlignCenter
	r := &runeBtnRenderer{btn: b, bg: bg, img: img, text: text}
	r.Refresh()
	return r
}

func (b *RuneButton) buildSVGResource() fyne.Resource {
	if b.SVGPath == "" {
		return nil
	}
	fill := "white"
	if b.Selected {
		fill = "#ffff50"
	}
	svg := fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 112 213"><path d="%s" fill="%s"/></svg>`,
		b.SVGPath, fill,
	)
	return fyne.NewStaticResource(b.Name+".svg", []byte(svg))
}

func (b *RuneButton) Tapped(_ *fyne.PointEvent) {
	if b.OnTap != nil {
		b.OnTap()
	}
}
func (b *RuneButton) TappedSecondary(_ *fyne.PointEvent) {}
func (b *RuneButton) MouseIn(_ *desktop.MouseEvent)      { b.hovered = true; b.Refresh() }
func (b *RuneButton) MouseOut()                          { b.hovered = false; b.Refresh() }
func (b *RuneButton) MouseMoved(_ *desktop.MouseEvent)   {}

type runeBtnRenderer struct {
	btn  *RuneButton
	bg   *canvas.Rectangle
	img  *canvas.Image
	text *canvas.Text
}

func (r *runeBtnRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	r.bg.Move(fyne.NewPos(0, 0))
	pad := float32(6)
	r.img.Resize(fyne.NewSize(size.Width-pad*2, size.Height-pad*2))
	r.img.Move(fyne.NewPos(pad, pad))
	r.text.Resize(fyne.NewSize(size.Width, r.text.TextSize*1.4))
	r.text.Move(fyne.NewPos(0, (size.Height-r.text.TextSize*1.4)/2))
}

func (r *runeBtnRenderer) MinSize() fyne.Size { return fyne.NewSize(52, 52) }

func (r *runeBtnRenderer) Refresh() {
	if r.btn.Selected {
		r.bg.FillColor = color.NRGBA{R: 60, G: 60, B: 30, A: 255}
	} else if r.btn.hovered {
		r.bg.FillColor = color.NRGBA{R: 80, G: 80, B: 120, A: 255}
	} else {
		r.bg.FillColor = color.NRGBA{R: 50, G: 50, B: 70, A: 255}
	}
	r.bg.Refresh()

	res := r.btn.buildSVGResource()
	if res != nil {
		r.img.Resource = res
		r.img.Show()
		r.text.Hide()
	} else {
		r.img.Hide()
		r.text.Text = r.btn.Name
		r.text.Show()
	}
	r.img.Refresh()
	r.text.Refresh()
}

func (r *runeBtnRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.img, r.text}
}
func (r *runeBtnRenderer) Destroy() {}