package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	barWidth       = 10
	barFilled      = "█"
	barEmpty       = "░"
	maxQuestionLen = 300
	maxOptionLen   = 120
	minOptions     = 2
)

// escapeMarkdown приводит пользовательский текст к виду, в котором он не ломает
// разметку сообщения и не превращается в упоминание на весь канал.
func escapeMarkdown(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	replacer := strings.NewReplacer(
		`\`, `\\`,
		"`", "'",
		"*", "\\*",
		"_", "\\_",
		"~", "\\~",
		"|", "\\|",
		"<", "&lt;",
		">", "&gt;",
		"[", "\\[",
		"]", "\\]",
	)
	s = replacer.Replace(s)
	return neutralizeMentions(s)
}

// neutralizeMentions вставляет невидимый пробел между @ и ником, чтобы текст
// варианта не рассылал уведомления каналу.
func neutralizeMentions(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		b.WriteRune(r)
		if r != '@' {
			continue
		}
		if i+1 < len(runes) && (unicode.IsLetter(runes[i+1]) || unicode.IsDigit(runes[i+1])) {
			b.WriteRune('\u200b')
		}
	}
	return b.String()
}

// shorten режет строку по границе слова: обрезанное посреди слова «возвращаем
// Нарезк…» на кнопке читается как опечатка, поэтому отступаем до последнего
// пробела, если он не слишком далеко от предела.
func shorten(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	cut := n - 1
	for i := cut - 1; i > cut/2; i-- {
		if runes[i] == ' ' {
			cut = i
			break
		}
	}
	return strings.TrimSpace(string(runes[:cut])) + "…"
}

func pluralRu(n int, one, few, many string) string {
	mod100 := n % 100
	if mod100 >= 11 && mod100 <= 14 {
		return fmt.Sprintf("%d %s", n, many)
	}
	switch n % 10 {
	case 1:
		return fmt.Sprintf("%d %s", n, one)
	case 2, 3, 4:
		return fmt.Sprintf("%d %s", n, few)
	default:
		return fmt.Sprintf("%d %s", n, many)
	}
}

// progressBar рисует полоску шириной barWidth. Вариант с голосами получает хотя
// бы один закрашенный блок, чтобы «1 голос из 200» не выглядел как ноль.
func progressBar(percent int, hasVotes bool) string {
	filled := percent * barWidth / 100
	if hasVotes && filled == 0 {
		filled = 1
	}
	if filled > barWidth {
		filled = barWidth
	}
	return strings.Repeat(barFilled, filled) + strings.Repeat(barEmpty, barWidth-filled)
}

func percentOf(part, total int) int {
	if total <= 0 {
		return 0
	}
	return int(float64(part)*100/float64(total) + 0.5)
}

func atoiDefault(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}
