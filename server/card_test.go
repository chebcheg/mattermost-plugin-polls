package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Карточка отдаёт разное состояние разным людям: проголосовавший видит итоги,
// остальные — бюллетень без цифр.
func TestCardStatePerUser(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	pl := mustCreatePoll(t, p)
	pl.PostID = "post-1"
	pl.Votes["voter"] = []string{pl.Options[0].ID}

	voter := p.cardFor(pl, "voter")
	assert.True(t, voter.Voted)
	assert.True(t, voter.ShowResults)
	assert.True(t, voter.Options[0].Mine)
	assert.Equal(t, 100, voter.Options[0].Percent)
	assert.Equal(t, 1, voter.TotalVotes)

	other := p.cardFor(pl, "другой")
	assert.False(t, other.Voted)
	assert.False(t, other.Options[0].Mine)
	assert.False(t, other.CanEnd, "завершать может только автор или админ")
}

func TestCardStateAuthorCanEnd(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	pl := mustCreatePoll(t, p) // создатель — creator
	pl.PostID = "post-1"

	author := p.cardFor(pl, "creator")
	assert.True(t, author.CanEnd)

	admin := p.cardFor(pl, "админ")
	assert.False(t, admin.CanEnd, "по умолчанию администратор чужой опрос не завершает")

	p.configuration.AdminsCanEnd = true
	admin = p.cardFor(pl, "админ")
	assert.True(t, admin.CanEnd, "с включённой настройкой — может")
}

func TestCardStateClosedShowsResultsToEveryone(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	pl := mustCreatePoll(t, p)
	pl.PostID = "post-1"
	pl.Votes["voter"] = []string{pl.Options[1].ID}
	pl.Closed = true

	other := p.cardFor(pl, "другой")
	assert.True(t, other.ShowResults, "после завершения итоги видны всем")
	assert.False(t, other.CanVote)
	assert.False(t, other.CanEnd)
}

var _ = require.New

// Подписи кнопок-вложений не должны начинаться с markdown-разметки: мобильный
// клиент рендерит их как markdown, и «1. текст» даёт пустую плашку.
func TestAttachmentLabelsAreNotMarkdownLists(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	pl := mustCreatePoll(t, p, "Первый вариант", "Второй вариант", "Третий вариант")
	listPrefix := regexp.MustCompile(`^\s*\d+[.)]\s`)
	sets := [][]*model.MessageAttachment{p.voteAttachment(pl), p.mobileCard(pl)}
	for _, attachments := range sets {
		require.NotEmpty(t, attachments)
		for _, attachment := range attachments {
			for _, action := range attachment.Actions {
				assert.Empty(t, listPrefix.FindString(action.Name), "подпись %q похожа на markdown-список", action.Name)
				assert.NotEmpty(t, action.Name)
			}
		}
	}
}

// В текстовом списке (мобильный клиент) ники идут отдельной строкой под
// вариантом, а не в одну строку с ним.
func TestRenderVotersNamesOnNewLine(t *testing.T) {
	p, api, _ := newTestPlugin(t)
	pl := mustCreatePoll(t, p, "Очень длинный вариант ответа, который не влезает в строку")
	pl.PostID = "post-1"
	pl.Votes["u1"] = []string{pl.Options[0].ID}
	pl.Votes["u2"] = []string{pl.Options[0].ID}

	api.On("GetUsersByIds", mock.AnythingOfType("[]string")).Return(
		[]*model.User{{Id: "u1", Username: "ivanov"}, {Id: "u2", Username: "petrov"}}, (*model.AppError)(nil),
	)

	text := p.renderVoters(pl)
	optionLine := strings.SplitN(text, "\n", 4)
	require.GreaterOrEqual(t, len(optionLine), 4, "ожидаем отдельную строку с никами")
	assert.Contains(t, optionLine[3], "ivanov")
	assert.NotContains(t, optionLine[2], "ivanov", "ники не должны стоять в строке с вариантом")
}
