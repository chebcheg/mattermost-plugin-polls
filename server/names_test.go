package main

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
)

func TestDisplayNameFormats(t *testing.T) {
	user := &model.User{
		Username:  "aasobolev",
		FirstName: "Александр",
		LastName:  "Соболев",
		Nickname:  "Саня",
	}

	assert.Equal(t, "Соболев Александр", displayName(user, nameFormatLastFirst))
	assert.Equal(t, "Соболев А.", displayName(user, nameFormatLastInitial))
	assert.Equal(t, "aasobolev", displayName(user, nameFormatUsername))
}

func TestDisplayNameFallbacks(t *testing.T) {
	onlyFirst := &model.User{Username: "maksim", FirstName: "Максим"}
	assert.Equal(t, "Максим", displayName(onlyFirst, nameFormatLastFirst))

	fullInFirstName := &model.User{Username: "maksim", FirstName: "Максим Евлентьев"}
	assert.Equal(t, "Максим Евлентьев", displayName(fullInFirstName, nameFormatLastFirst))

	nickOnly := &model.User{Username: "solo366", Nickname: "Соло"}
	assert.Equal(t, "Соло", displayName(nickOnly, nameFormatLastInitial))

	loginOnly := &model.User{Username: "cheba"}
	assert.Equal(t, "cheba", displayName(loginOnly, nameFormatLastFirst))

	// Неизвестный формат из конфигурации не должен ломать вывод.
	assert.Equal(t, "Соболев Александр", displayName(&model.User{FirstName: "Александр", LastName: "Соболев"}, "что-то"))
}
