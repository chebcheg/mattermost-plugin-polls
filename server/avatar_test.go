package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBotAvatarPNG(t *testing.T) {
	data, err := botAvatarPNG()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	img, err := png.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, avatarSize, img.Bounds().Dx())
	assert.Equal(t, avatarSize, img.Bounds().Dy())

	// Угол прозрачный — аватар круглый, а не квадрат.
	_, _, _, alpha := img.At(2, 2).RGBA()
	assert.Zero(t, alpha, "угол должен быть прозрачным")

	// Центр — на среднем столбике, значит белый.
	r, g, b, a := img.At(avatarSize/2, avatarSize/2).RGBA()
	assert.Equal(t, uint32(0xffff), a)
	assert.True(t, r > 0xf000 && g > 0xf000 && b > 0xf000, "центр должен быть белым")

	// Точка слева от столбиков — синий фон.
	cr, cg, cb, _ := img.At(40, avatarSize/2).RGBA()
	assert.Greater(t, cb, cr, "фон должен быть синим")
	assert.Greater(t, cb, cg)
}

func TestBotAvatarIsStable(t *testing.T) {
	first, err := botAvatarPNG()
	require.NoError(t, err)
	second, err := botAvatarPNG()
	require.NoError(t, err)
	assert.Equal(t, first, second, "аватар должен быть одинаковым при каждой сборке")

	var _ color.Color = color.RGBA{}
	var _ image.Image = image.NewRGBA(image.Rect(0, 0, 1, 1))
}
