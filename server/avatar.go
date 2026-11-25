package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// Аватар бота рисуется кодом, а не берётся из файла: go:embed не умеет
// подниматься выше каталога пакета, а тащить копию картинки в server/assets
// ради одного PNG смысла нет. Рисуем в 4× и уменьшаем — так края круга и
// столбиков получаются гладкими без ручного сглаживания.

const (
	avatarSize  = 256
	avatarScale = 4
)

var (
	avatarBackground = color.RGBA{R: 0x1c, G: 0x58, B: 0xd9, A: 0xff}
	avatarForeground = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
)

// botAvatarPNG возвращает PNG 256×256: синий круг и три белых столбика —
// та же метафора, что у иконки плагина и кнопки в панели форматирования.
func botAvatarPNG() ([]byte, error) {
	side := avatarSize * avatarScale
	img := image.NewRGBA(image.Rect(0, 0, side, side))

	// Круг.
	center := float64(side) / 2
	radius := center - float64(avatarScale)
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			dx := float64(x) + 0.5 - center
			dy := float64(y) + 0.5 - center
			if dx*dx+dy*dy <= radius*radius {
				img.Set(x, y, avatarBackground)
			}
		}
	}

	// Три столбика: слева низкий, в центре высокий, справа средний.
	type bar struct {
		left, top, right, bottom float64
	}
	bars := []bar{
		{left: 58, top: 120, right: 90, bottom: 184},
		{left: 112, top: 56, right: 144, bottom: 184},
		{left: 166, top: 88, right: 198, bottom: 184},
	}
	for _, b := range bars {
		fillRoundedRect(img,
			b.left*avatarScale, b.top*avatarScale,
			b.right*avatarScale, b.bottom*avatarScale,
			10*avatarScale, avatarForeground)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, downscale(img, avatarScale)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// fillRoundedRect закрашивает прямоугольник со скруглёнными углами.
func fillRoundedRect(img *image.RGBA, x0, y0, x1, y1, radius float64, c color.RGBA) {
	halfW := (x1 - x0) / 2
	halfH := (y1 - y0) / 2
	cx := x0 + halfW
	cy := y0 + halfH

	for y := int(y0); y < int(y1); y++ {
		for x := int(x0); x < int(x1); x++ {
			px := float64(x) + 0.5 - cx
			py := float64(y) + 0.5 - cy

			dx := abs(px) - (halfW - radius)
			dy := abs(py) - (halfH - radius)
			if dx < 0 {
				dx = 0
			}
			if dy < 0 {
				dy = 0
			}
			if dx*dx+dy*dy <= radius*radius {
				img.Set(x, y, c)
			}
		}
	}
}

// downscale усредняет блок scale×scale в один пиксель — простое сглаживание.
func downscale(src *image.RGBA, scale int) *image.RGBA {
	w := src.Bounds().Dx() / scale
	h := src.Bounds().Dy() / scale
	dst := image.NewRGBA(image.Rect(0, 0, w, h))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b, a uint32
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					sr, sg, sb, sa := src.At(x*scale+dx, y*scale+dy).RGBA()
					r += sr
					g += sg
					b += sb
					a += sa
				}
			}
			n := uint32(scale * scale)
			dst.Set(x, y, color.RGBA{
				R: uint8(r / n >> 8),
				G: uint8(g / n >> 8),
				B: uint8(b / n >> 8),
				A: uint8(a / n >> 8),
			})
		}
	}
	return dst
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
