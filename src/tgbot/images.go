package tgbot

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// bannerTheme — цветовая схема баннера команды.
type bannerTheme struct {
	TopLeft     color.RGBA
	BottomRight color.RGBA
	Accent      color.RGBA
}

// commandThemes задаёт цветовую схему для каждой команды.
var commandThemes = map[string]bannerTheme{
	"start": {
		TopLeft:     color.RGBA{28, 112, 213, 255},
		BottomRight: color.RGBA{10, 60, 160, 255},
		Accent:      color.RGBA{100, 181, 246, 255},
	},
	"status": {
		TopLeft:     color.RGBA{34, 139, 87, 255},
		BottomRight: color.RGBA{15, 90, 55, 255},
		Accent:      color.RGBA{105, 220, 158, 255},
	},
	"deadlines": {
		TopLeft:     color.RGBA{205, 57, 44, 255},
		BottomRight: color.RGBA{140, 25, 15, 255},
		Accent:      color.RGBA{255, 140, 130, 255},
	},
	"docs": {
		TopLeft:     color.RGBA{51, 103, 214, 255},
		BottomRight: color.RGBA{25, 60, 160, 255},
		Accent:      color.RGBA{144, 202, 249, 255},
	},
	"checklists": {
		TopLeft:     color.RGBA{230, 162, 5, 255},
		BottomRight: color.RGBA{160, 105, 0, 255},
		Accent:      color.RGBA{255, 230, 100, 255},
	},
	"ask": {
		TopLeft:     color.RGBA{103, 58, 183, 255},
		BottomRight: color.RGBA{60, 20, 140, 255},
		Accent:      color.RGBA{210, 160, 255, 255},
	},
	"help": {
		TopLeft:     color.RGBA{0, 137, 123, 255},
		BottomRight: color.RGBA{0, 80, 70, 255},
		Accent:      color.RGBA{100, 230, 212, 255},
	},
	"logout": {
		TopLeft:     color.RGBA{95, 99, 104, 255},
		BottomRight: color.RGBA{50, 52, 58, 255},
		Accent:      color.RGBA{180, 185, 190, 255},
	},
}

// defaultTheme используется когда команда не найдена в карте.
var defaultTheme = bannerTheme{
	TopLeft:     color.RGBA{50, 50, 80, 255},
	BottomRight: color.RGBA{20, 20, 50, 255},
	Accent:      color.RGBA{120, 140, 200, 255},
}

// fileIDCache хранит Telegram file_id уже загруженных баннеров.
// После первой отправки Telegram кэширует файл и мы переиспользуем его ID.
type fileIDCache struct {
	mu  sync.RWMutex
	ids map[string]string // command → file_id
}

var bannerCache = &fileIDCache{ids: make(map[string]string)}

func (c *fileIDCache) get(cmd string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	id, ok := c.ids[cmd]
	return id, ok
}

func (c *fileIDCache) set(cmd, fileID string) {
	c.mu.Lock()
	c.ids[cmd] = fileID
	c.mu.Unlock()
}

// sendCommandBanner отправляет тематический баннер команды.
// При первой отправке генерирует PNG и загружает в Telegram; при повторных — переиспользует file_id.
func (b *Bot) sendCommandBanner(chatID int64, command, caption string) {
	if fileID, ok := bannerCache.get(command); ok {
		msg := tgbotapi.NewPhoto(chatID, tgbotapi.FileID(fileID))
		msg.Caption = caption
		msg.ParseMode = "Markdown"
		if _, err := b.api.Send(msg); err == nil {
			return
		}
		// file_id устарел (бывает после долгого простоя) — перегенерируем.
		bannerCache.set(command, "")
	}

	pngBytes := genBanner(command)
	msg := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{
		Name:  "banner_" + command + ".png",
		Bytes: pngBytes,
	})
	msg.Caption = caption
	msg.ParseMode = "Markdown"

	res, err := b.api.Send(msg)
	if err != nil {
		// Не удалось отправить картинку — продолжаем без неё.
		return
	}

	// Кэшируем file_id для повторного использования.
	if len(res.Photo) > 0 {
		bannerCache.set(command, res.Photo[len(res.Photo)-1].FileID)
	}
}

// genBanner генерирует градиентный PNG-баннер 600×140 для команды.
func genBanner(command string) []byte {
	theme, ok := commandThemes[command]
	if !ok {
		theme = defaultTheme
	}

	const W, H = 600, 140
	img := image.NewRGBA(image.Rect(0, 0, W, H))

	// Двухточечный диагональный градиент (topLeft → bottomRight).
	for y := 0; y < H; y++ {
		ty := float64(y) / float64(H)
		for x := 0; x < W; x++ {
			tx := float64(x) / float64(W)
			t := (tx + ty) / 2
			r := uint8(float64(theme.TopLeft.R) + t*float64(int(theme.BottomRight.R)-int(theme.TopLeft.R)))
			g := uint8(float64(theme.TopLeft.G) + t*float64(int(theme.BottomRight.G)-int(theme.TopLeft.G)))
			bv := uint8(float64(theme.TopLeft.B) + t*float64(int(theme.BottomRight.B)-int(theme.TopLeft.B)))
			img.SetRGBA(x, y, color.RGBA{r, g, bv, 255})
		}
	}

	// Декоративные полупрозрачные круги справа.
	drawCircleAlpha(img, W-60, H/2, 90, blendedColor(theme.Accent, 35))
	drawCircleAlpha(img, W-110, 20, 55, blendedColor(theme.Accent, 25))
	drawCircleAlpha(img, W+20, H-10, 70, blendedColor(theme.Accent, 20))

	// Небольшой круг слева.
	drawCircleAlpha(img, 50, H/2+10, 35, color.RGBA{0, 0, 0, 30})

	// Полоска-акцент снизу.
	accentStrip := color.RGBA{theme.Accent.R, theme.Accent.G, theme.Accent.B, 180}
	draw.Draw(img, image.Rect(0, H-5, W, H), &image.Uniform{accentStrip}, image.Point{}, draw.Over)

	// Три горизонтальные линии (как «плитки» в Material Design).
	for i, y := range []int{30, 70, 100} {
		lineW := W/2 - i*30
		if lineW < 60 {
			lineW = 60
		}
		line := color.RGBA{255, 255, 255, 22}
		draw.Draw(img, image.Rect(30, y, 30+lineW, y+3), &image.Uniform{line}, image.Point{}, draw.Over)
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// drawCircleAlpha рисует заливку круга с учётом прозрачности.
func drawCircleAlpha(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	bounds := img.Bounds()
	rf := float64(r)
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if x < bounds.Min.X || x >= bounds.Max.X || y < bounds.Min.Y || y >= bounds.Max.Y {
				continue
			}
			dx, dy := float64(x-cx), float64(y-cy)
			if math.Sqrt(dx*dx+dy*dy) > rf {
				continue
			}
			src := img.RGBAAt(x, y)
			a := float64(c.A) / 255
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(float64(src.R)*(1-a) + float64(c.R)*a),
				G: uint8(float64(src.G)*(1-a) + float64(c.G)*a),
				B: uint8(float64(src.B)*(1-a) + float64(c.B)*a),
				A: 255,
			})
		}
	}
}

// blendedColor возвращает цвет с заданной альфой.
func blendedColor(c color.RGBA, alpha uint8) color.RGBA {
	return color.RGBA{c.R, c.G, c.B, alpha}
}
