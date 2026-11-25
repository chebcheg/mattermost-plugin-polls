package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShortenCutsAtWordBoundary(t *testing.T) {
	assert.Equal(t, "короткий", shorten("короткий", 20))
	// Режем по последнему пробелу, который влезает в предел, — слово не рвётся.
	assert.Equal(t, "возвращаем Нарезку…", shorten("возвращаем Нарезку на каналы", 20))
	assert.Equal(t, "все равно, как…", shorten("все равно, как хотите", 20))
	// Без пробелов деваться некуда — режем по символу.
	assert.Equal(t, "оченьдлинноесловобе…", shorten("оченьдлинноесловобезпробелов", 20))
}
