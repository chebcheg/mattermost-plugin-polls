package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newTestPlugin поднимает плагин с поддельным API и KV-хранилищем в памяти.
func newTestPlugin(t *testing.T) (*Plugin, *plugintest.API, map[string][]byte) {
	t.Helper()

	kv := map[string][]byte{}
	api := &plugintest.API{}

	api.On("KVGet", mock.AnythingOfType("string")).Return(
		func(key string) []byte { return kv[key] },
		func(string) *model.AppError { return nil },
	)
	api.On("KVSet", mock.AnythingOfType("string"), mock.Anything).Return(
		func(key string, value []byte) *model.AppError {
			kv[key] = value
			return nil
		},
	)
	api.On("KVDelete", mock.AnythingOfType("string")).Return((*model.AppError)(nil))
	api.On("SendEphemeralPost", mock.AnythingOfType("string"), mock.AnythingOfType("*model.Post")).Return(
		func(_ string, post *model.Post) *model.Post { return post },
	)
	// Членство: пускаем всех, кроме «чужака» — на нём проверяем запрет.
	api.On("GetChannelMember", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(
		func(channelID, userID string) *model.ChannelMember {
			return &model.ChannelMember{ChannelId: channelID, UserId: userID}
		},
		func(_ string, userID string) *model.AppError {
			if userID == "чужак" {
				return model.NewAppError("GetChannelMember", "app.channel.get_member.missing.app_error", nil, "", 404)
			}
			return nil
		},
	)
	// Права: «админ» — системный администратор, все остальные — обычные.
	api.On("HasPermissionTo", mock.Anything, mock.Anything).Return(
		func(userID string, permission *model.Permission) bool {
			return userID == "админ" && permission == model.PermissionManageSystem
		},
	)
	// Логи идут с разным числом key/value-пар — регистрируем все варианты.
	for _, method := range []string{"LogInfo", "LogWarn", "LogError", "LogDebug"} {
		for arity := 1; arity <= 8; arity++ {
			args := make([]any, arity)
			for i := range args {
				args[i] = mock.Anything
			}
			api.On(method, args...).Maybe()
		}
	}

	p := &Plugin{botID: "bot-id"}
	p.SetAPI(api)
	p.configuration = &configuration{MaxOptions: "12", ShowVotersButton: true}

	return p, api, kv
}

func mustCreatePoll(t *testing.T, p *Plugin, opts ...string) *poll {
	t.Helper()
	if len(opts) == 0 {
		opts = []string{"Столовая", "Кафе", "Пицца"}
	}
	pl := newPoll("chan", "creator", "Куда идём на обед?", opts, p.getConfiguration())
	return pl
}

func TestNormalizeOptions(t *testing.T) {
	got, err := normalizeOptions([]string{" Столовая ", "", "кафе", "КАФЕ", "Пицца", "Третье лишнее"}, 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"Столовая", "кафе", "Пицца"}, got)

	_, err = normalizeOptions([]string{"один", " "}, 12)
	assert.Error(t, err, "один вариант — не опрос")
}

func TestRenderPollHidesResultsUntilVote(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	pl := mustCreatePoll(t, p)
	pl.HideResults = true
	pl.Votes["u1"] = []string{pl.Options[0].ID}

	shared := p.renderPoll(pl, "")
	assert.Contains(t, shared, "результаты скрыты до вашего голоса")
	assert.NotContains(t, shared, "%")

	personal := p.renderPoll(pl, "u1")
	assert.Contains(t, personal, "100%")
	assert.Contains(t, personal, "✓")
}

func TestRenderPollPercentagesAndBars(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	pl := mustCreatePoll(t, p)
	pl.Votes["u1"] = []string{pl.Options[0].ID}
	pl.Votes["u2"] = []string{pl.Options[0].ID}
	pl.Votes["u3"] = []string{pl.Options[1].ID}

	out := p.renderPoll(pl, "")
	assert.Contains(t, out, "**Куда идём на обед?**")
	assert.Contains(t, out, "**67%** · 2")
	assert.Contains(t, out, "**33%** · 1")
	assert.Contains(t, out, "3 голоса · 3 человека")
	assert.Contains(t, out, barFilled)
}

func TestApplyVoteSingleReplacesChoice(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	pl := mustCreatePoll(t, p)
	require.NoError(t, p.savePoll(pl))

	res, err := p.applyVote("u1", pl.ID, pl.Options[0].ID)
	require.NoError(t, err)
	assert.Equal(t, []string{pl.Options[0].ID}, res.Poll.Votes["u1"])

	res, err = p.applyVote("u1", pl.ID, pl.Options[1].ID)
	require.NoError(t, err)
	assert.Equal(t, []string{pl.Options[1].ID}, res.Poll.Votes["u1"], "в одиночном опросе голос заменяется")

	// Состояние действительно легло в KV, а не осталось в структуре.
	stored, err := p.loadPoll(pl.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{pl.Options[1].ID}, stored.Votes["u1"])
}

func TestApplyVoteMultipleToggles(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	pl := mustCreatePoll(t, p)
	pl.Multiple = true
	require.NoError(t, p.savePoll(pl))

	_, err := p.applyVote("u1", pl.ID, pl.Options[0].ID)
	require.NoError(t, err)
	res, err := p.applyVote("u1", pl.ID, pl.Options[2].ID)
	require.NoError(t, err)
	assert.Len(t, res.Poll.Votes["u1"], 2)

	res, err = p.applyVote("u1", pl.ID, pl.Options[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "снят", res.Note)
	assert.Equal(t, []string{pl.Options[2].ID}, res.Poll.Votes["u1"])
}

func TestVoteRejectedOnClosedPoll(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	pl := mustCreatePoll(t, p)
	pl.Closed = true
	require.NoError(t, p.savePoll(pl))

	_, err := p.applyVote("u1", pl.ID, pl.Options[0].ID)
	assert.ErrorIs(t, err, errPollClosed)
}

func TestVoteUnknownOption(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	pl := mustCreatePoll(t, p)
	require.NoError(t, p.savePoll(pl))

	_, err := p.applyVote("u1", pl.ID, "нет-такого")
	assert.ErrorIs(t, err, errOptionNotFound)
}

func TestCancelVote(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	pl := mustCreatePoll(t, p)
	pl.Votes["u1"] = []string{pl.Options[0].ID}
	require.NoError(t, p.savePoll(pl))

	res, err := p.cancelVote("u1", pl.ID)
	require.NoError(t, err)
	assert.Empty(t, res.Poll.Votes["u1"])
	assert.NotContains(t, p.renderPoll(res.Poll, ""), "✓")

	_, err = p.cancelVote("u1", pl.ID)
	assert.Error(t, err, "повторная отмена — ошибка, а не тишина")
}

func TestClosePollPermissions(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	pl := mustCreatePoll(t, p) // создатель creator
	require.NoError(t, p.savePoll(pl))

	_, err := p.closePoll("чужой", pl.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "только автор")

	// Администратор по умолчанию завершить чужой опрос не может —
	// привилегия включается отдельной настройкой.
	_, err = p.closePoll("админ", pl.ID)
	require.Error(t, err)

	p.configuration.AdminsCanEnd = true
	closed, err := p.closePoll("админ", pl.ID)
	require.NoError(t, err)
	assert.True(t, closed.Closed)

	_, err = p.closePoll("creator", pl.ID)
	assert.ErrorIs(t, err, errPollClosed)
}

func TestClosePollAnyoneCanEnd(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	p.configuration.AnyoneCanEnd = true
	pl := mustCreatePoll(t, p)
	require.NoError(t, p.savePoll(pl))

	_, err := p.closePoll("любой", pl.ID)
	require.NoError(t, err)
}

func TestRenderClosedPollMarksWinner(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	pl := mustCreatePoll(t, p)
	pl.Votes["u1"] = []string{pl.Options[2].ID}
	pl.Votes["u2"] = []string{pl.Options[2].ID}
	pl.Closed = true

	out := p.renderPoll(pl, "")
	assert.Contains(t, out, "опрос завершён")
	assert.Contains(t, out, "победитель")
	assert.Equal(t, 1, strings.Count(out, "победитель"))
}

func TestCreatePollPostsAndStores(t *testing.T) {
	p, api, kv := newTestPlugin(t)
	api.On("CreatePost", mock.AnythingOfType("*model.Post")).Return(
		func(post *model.Post) *model.Post {
			post.Id = "post-1"
			// Проверяем то, что реально уходит в канал.
			attachments, ok := post.GetProp(model.PostPropsAttachments).([]*model.MessageAttachment)
			require.True(t, ok, "кнопки должны лежать в props.attachments")
			require.Len(t, attachments, 1, "для поста-карточки всё содержимое в одном вложении")
			require.Len(t, attachments[0].Actions, 6, "3 варианта + отмена, кто голосовал, завершить")
			require.Equal(t, "#1c58d9", attachments[0].Color)
			require.Contains(t, attachments[0].Text, "Столовая")
			require.Equal(t, "bot-id", post.UserId)
			// Текст поста-карточки короткий: подробности — во вложении.
			require.Equal(t, "**Куда идём на обед?**", strings.TrimSpace(post.Message))
			require.Equal(t, pollPostType, post.Type)
			require.Contains(t, attachments[0].Actions[0].Integration.URL, "/plugins/"+manifestID+routeVote)
			return post
		},
		(*model.AppError)(nil),
	)

	pl, appErr := p.createPoll("chan", "", "creator", "Куда идём на обед?", []string{"Столовая", "Кафе", "Пицца"}, false, false, false)
	require.Nil(t, appErr)
	assert.Equal(t, "post-1", pl.PostID)
	assert.Contains(t, kv, pollKey(pl.ID))
	assert.Contains(t, kv, indexKey("chan"))
}

func TestTokenize(t *testing.T) {
	got := tokenize(`"Куда идём?" "Столовая" "кафе с собой"`)
	require.Len(t, got, 3)
	assert.Equal(t, "Куда идём?", got[0].text)
	assert.Equal(t, "кафе с собой", got[2].text)
	assert.True(t, got[0].quoted)

	plain := tokenize("Куда идём на обед?")
	require.Len(t, plain, 4)
	assert.False(t, plain[0].quoted)
}

func TestCommandCreateUnquotedMeansYesNo(t *testing.T) {
	p, api, _ := newTestPlugin(t)
	api.On("CreatePost", mock.AnythingOfType("*model.Post")).Return(
		func(post *model.Post) *model.Post { post.Id = "post-1"; return post },
		(*model.AppError)(nil),
	)

	args := &model.CommandArgs{Command: "/poll Куда идём на обед?", ChannelId: "chan", UserId: "creator"}
	_, appErr := p.ExecuteCommand(nil, args)
	require.Nil(t, appErr)

	var stored *poll
	for _, v := range mustKV(t, p) {
		var candidate poll
		if json.Unmarshal(v, &candidate) == nil && candidate.Question != "" {
			stored = &candidate
		}
	}
	require.NotNil(t, stored)
	assert.Equal(t, "Куда идём на обед?", stored.Question)
	assert.Equal(t, []string{"Да", "Нет"}, optionTexts(stored))
}

func TestDialogSubmitRejectsOneOption(t *testing.T) {
	p, _, _ := newTestPlugin(t)

	body, _ := json.Marshal(model.SubmitDialogRequest{
		Type:       "dialog_submission",
		CallbackId: "create_poll",
		UserId:     "creator",
		ChannelId:  "chan",
		Submission: map[string]any{"question": "Обед?", "options": "Столовая"},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", routeDialog, strings.NewReader(string(body)))
	req.Header.Set("Mattermost-User-Id", "creator")
	p.handleDialogSubmit(rec, req)

	assert.Equal(t, 200, rec.Code)
	var resp model.SubmitDialogResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Errors["options"], "одного варианта мало")
}

func optionTexts(pl *poll) []string {
	out := make([]string, 0, len(pl.Options))
	for _, opt := range pl.Options {
		out = append(out, opt.Text)
	}
	return out
}

func mustKV(t *testing.T, p *Plugin) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, key := range kvKeys(t, p) {
		data, _ := p.API.KVGet(key)
		out[key] = data
	}
	return out
}

// kvKeys достаёт ключи из поддельного хранилища, чтобы не тянуть его наружу
// через возвращаемое значение newTestPlugin.
func kvKeys(t *testing.T, p *Plugin) []string {
	t.Helper()
	api, ok := p.API.(*plugintest.API)
	require.True(t, ok)
	var keys []string
	for _, call := range api.Calls {
		if call.Method == "KVSet" {
			keys = append(keys, call.Arguments.String(0))
		}
	}
	return keys
}

// Пользователь берётся из заголовка, а не из тела: подделанный user_id не должен
// записывать голос за другого человека.
func TestVoteUsesHeaderNotBody(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	pl := mustCreatePoll(t, p)
	require.NoError(t, p.savePoll(pl))

	body := `{"user_id":"u-жертва","context":{"poll_id":"` + pl.ID + `","option_id":"` + pl.Options[0].ID + `"}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", routeVote, strings.NewReader(body))
	req.Header.Set("Mattermost-User-Id", "u-настоящий")
	p.handleVote(rec, req)

	require.Equal(t, 200, rec.Code)
	// Подтверждение ушло именно нажавшему, с отметкой его выбора.
	eph := p.API.(*plugintest.API).Calls
	var lastEphemeral *model.Post
	for _, call := range eph {
		if call.Method == "SendEphemeralPost" {
			lastEphemeral = call.Arguments.Get(1).(*model.Post)
		}
	}
	require.NotNil(t, lastEphemeral, "подтверждение голоса должно уходить персонально")
	assert.Contains(t, lastEphemeral.Message, "Голос учтён")
	assert.Contains(t, lastEphemeral.Message, "Столовая")
	assert.NotContains(t, lastEphemeral.Message, "%", "в обычном опросе цифры уже есть в канале")

	stored, err := p.loadPoll(pl.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{pl.Options[0].ID}, stored.Votes["u-настоящий"])
	assert.Empty(t, stored.Votes["u-жертва"])

	// Без заголовка — отказ, а не анонимный голос.
	rec2 := httptest.NewRecorder()
	p.handleVote(rec2, httptest.NewRequest("POST", routeVote, strings.NewReader(body)))
	assert.Equal(t, 401, rec2.Code)
}

// В опросе со скрытыми результатами персональный ответ — единственное место,
// где видны проценты, поэтому там карточка показывается целиком.
func TestVoteReplyShowsResultsWhenHidden(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	pl := mustCreatePoll(t, p)
	pl.HideResults = true
	pl.Votes["u1"] = []string{pl.Options[0].ID}

	reply := p.voteReply(pl, "u1", "Голос учтён: **Столовая**")
	assert.Contains(t, reply, "100%")
	assert.Contains(t, reply, "✓")

	plain := mustCreatePoll(t, p)
	plain.Votes["u1"] = []string{plain.Options[0].ID}
	assert.NotContains(t, p.voteReply(plain, "u1", "Голос учтён"), "%")
}

func TestVoteRejectedForNonMember(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	pl := mustCreatePoll(t, p)
	require.NoError(t, p.savePoll(pl))

	_, err := p.applyVote("чужак", pl.ID, pl.Options[0].ID)
	assert.ErrorIs(t, err, errNoAccess)
}
