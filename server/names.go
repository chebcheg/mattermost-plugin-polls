package main

import (
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

// Форматы отображения имени в списке проголосовавших.
const (
	nameFormatLastFirst   = "last_first"   // Фамилия Имя
	nameFormatLastInitial = "last_initial" // Фамилия И.
	nameFormatUsername    = "username"     // логин
)

// displayName выбирает, как показать человека. По умолчанию — «Фамилия Имя» из
// профиля: в организации имена заполнены в этих полях, а логины вида «aasobolev»
// в списке проголосовавших ничего не говорят.
func displayName(user *model.User, format string) string {
	if user == nil {
		return ""
	}

	last := strings.TrimSpace(user.LastName)
	first := strings.TrimSpace(user.FirstName)
	nick := strings.TrimSpace(user.Nickname)
	username := strings.TrimSpace(user.Username)

	// В части профилей ФИО целиком лежит в одном поле — тогда не склеиваем.
	if last == "" && len(strings.Fields(first)) > 1 {
		last, first = first, ""
	}

	switch format {
	case nameFormatUsername:
		if username != "" {
			return username
		}
	case nameFormatLastInitial:
		if last != "" && first != "" {
			return last + " " + string([]rune(first)[0]) + "."
		}
		if last != "" {
			return last
		}
	default: // nameFormatLastFirst
		if last != "" && first != "" {
			return last + " " + first
		}
		if last != "" {
			return last
		}
	}

	if first != "" {
		return first
	}
	if nick != "" {
		return nick
	}
	return username
}

// normalizeNameFormat приводит значение настройки к известному варианту.
func normalizeNameFormat(format string) string {
	switch format {
	case nameFormatLastFirst, nameFormatLastInitial, nameFormatUsername:
		return format
	default:
		return nameFormatLastFirst
	}
}
