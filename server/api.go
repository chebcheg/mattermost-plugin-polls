package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

const maxRequestBody = 1 << 20 // 1 МиБ: запрос кнопки — это несколько сотен байт

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// replyEphemeral отправляет нажавшему сообщение, которое видит только он, и
// закрывает запрос пустым ответом. Поле ephemeral_text в ответе на действие
// Mattermost 12 больше не показывает — серверная отправка работает во всех
// версиях и на всех клиентах, включая мобильный.
func (p *Plugin) replyEphemeral(w http.ResponseWriter, channelID, userID, text string) {
	if channelID != "" {
		p.API.SendEphemeralPost(userID, &model.Post{
			ChannelId: channelID,
			UserId:    p.botID,
			Message:   text,
		})
	}
	writeJSON(w, model.PostActionIntegrationResponse{})
}

// readActionRequest разбирает тело нажатия кнопки. Пользователя берём из
// заголовка Mattermost-User-Id: его выставляет сервер после проверки сессии,
// а user_id в теле присылает клиент и подделать его ничего не стоит.
func readActionRequest(w http.ResponseWriter, r *http.Request) (*model.PostActionIntegrationRequest, bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return nil, false
	}
	userID := r.Header.Get("Mattermost-User-Id")
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	body := http.MaxBytesReader(w, r.Body, maxRequestBody)
	var req model.PostActionIntegrationRequest
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return nil, false
	}
	req.UserId = userID
	return &req, true
}

func contextString(ctx map[string]any, key string) string {
	if v, ok := ctx[key].(string); ok {
		return v
	}
	return ""
}

func errorText(err error) string {
	switch err {
	case errPollNotFound:
		return "Опрос не найден — возможно, сообщение удалено."
	case errPollClosed:
		return "Опрос уже завершён, голосовать больше нельзя."
	case errOptionNotFound:
		return "Такого варианта в опросе уже нет."
	case errNoAccess:
		return "Вы не состоите в канале с этим опросом."
	}
	return err.Error()
}

// --- нажатия кнопок -----------------------------------------------------------

func (p *Plugin) handleVote(w http.ResponseWriter, r *http.Request) {
	req, ok := readActionRequest(w, r)
	if !ok {
		return
	}

	pollID := contextString(req.Context, "poll_id")
	optionID := contextString(req.Context, "option_id")

	res, err := p.applyVote(req.UserId, pollID, optionID)
	if err != nil {
		p.replyEphemeral(w, req.ChannelId, req.UserId, errorText(err))
		return
	}

	opt := res.Poll.optionByID(optionID)
	head := "Голос учтён: **" + escapeMarkdown(opt.Text) + "**"
	if res.Note == "снят" {
		head = "Голос снят: **" + escapeMarkdown(opt.Text) + "**"
	}

	p.replyEphemeral(w, res.Poll.ChannelID, req.UserId, p.voteReply(res.Poll, req.UserId, head))
}

func (p *Plugin) handleCancel(w http.ResponseWriter, r *http.Request) {
	req, ok := readActionRequest(w, r)
	if !ok {
		return
	}

	res, err := p.cancelVote(req.UserId, contextString(req.Context, "poll_id"))
	if err != nil {
		p.replyEphemeral(w, req.ChannelId, req.UserId, errorText(err))
		return
	}

	p.replyEphemeral(w, res.Poll.ChannelID, req.UserId, "Голос отменён.")
}

func (p *Plugin) handleEnd(w http.ResponseWriter, r *http.Request) {
	req, ok := readActionRequest(w, r)
	if !ok {
		return
	}

	pl, err := p.closePoll(req.UserId, contextString(req.Context, "poll_id"))
	if err != nil {
		if err == errPollClosed && pl != nil {
			p.replyEphemeral(w, pl.ChannelID, req.UserId, "Опрос уже завершён.\n\n"+p.renderPoll(pl, req.UserId))
			return
		}
		p.replyEphemeral(w, req.ChannelId, req.UserId, errorText(err))
		return
	}

	p.replyEphemeral(w, pl.ChannelID, req.UserId, "Опрос завершён.\n\n"+p.renderPoll(pl, req.UserId))
}

func (p *Plugin) handleVoters(w http.ResponseWriter, r *http.Request) {
	req, ok := readActionRequest(w, r)
	if !ok {
		return
	}

	pl, err := p.loadPoll(contextString(req.Context, "poll_id"))
	if err != nil {
		p.replyEphemeral(w, req.ChannelId, req.UserId, errorText(err))
		return
	}
	if pl.Anonymous {
		p.replyEphemeral(w, pl.ChannelID, req.UserId, "Опрос анонимный — кто как голосовал, не показывается.")
		return
	}

	p.replyEphemeral(w, pl.ChannelID, req.UserId, p.renderVoters(pl))
}

// renderVoters — персональный список проголосовавших. Ники без @, чтобы клик по
// кнопке не рассылал упоминания.
func (p *Plugin) renderVoters(pl *poll) string {
	ids := make([]string, 0, len(pl.Votes))
	for userID, choices := range pl.Votes {
		if len(choices) > 0 {
			ids = append(ids, userID)
		}
	}
	if len(ids) > 100 {
		ids = ids[:100]
	}
	sort.Strings(ids)

	names := map[string]string{}
	if len(ids) > 0 {
		if users, appErr := p.API.GetUsersByIds(ids); appErr == nil {
			format := normalizeNameFormat(p.getConfiguration().VotersNameFormat)
			for _, u := range users {
				names[u.Id] = displayName(u, format)
			}
		}
	}

	counts := pl.counts()
	var b strings.Builder
	b.WriteString("**Кто голосовал** · " + pluralRu(pl.votersCount(), "человек", "человека", "человек"))

	for i, opt := range pl.Options {
		voters := pl.votersOf(opt.ID)
		b.WriteString(fmt.Sprintf("\n\n**%d. %s** — %d\n", i+1, escapeMarkdown(opt.Text), counts[opt.ID]))
		if len(voters) == 0 {
			b.WriteString("_никто_")
			continue
		}
		shown := make([]string, 0, len(voters))
		for _, userID := range voters {
			if name, ok := names[userID]; ok {
				shown = append(shown, escapeMarkdown(name))
			} else {
				shown = append(shown, "удалённый пользователь")
			}
		}
		more := 0
		if len(shown) > votersShowLimit {
			more = len(shown) - votersShowLimit
			shown = shown[:votersShowLimit]
		}
		// Ники — отдельной строкой под вариантом: длинный вариант с десятком
		// имён в одну строку не читается.
		b.WriteString("\n" + strings.Join(shown, ", "))
		if more > 0 {
			b.WriteString(fmt.Sprintf(" и ещё %d", more))
		}
	}

	b.WriteString("\n\n_Список виден только вам._")
	return b.String()
}

// --- создание опроса ----------------------------------------------------------

// createPoll публикует сообщение с опросом и сохраняет его состояние в KV.
// ID опроса не зависит от ID сообщения: сначала генерируется опрос, потом
// сообщение, и только после успешной публикации состояние попадает в хранилище.
func (p *Plugin) createPoll(channelID, rootID, creatorID, question string, options []string, multiple, anonymous, hideResults bool) (*poll, *model.AppError) {
	cfg := p.getConfiguration()

	pl := newPoll(channelID, creatorID, question, options, cfg)
	pl.Multiple = multiple
	pl.Anonymous = anonymous
	pl.HideResults = hideResults

	post := &model.Post{
		ChannelId: channelID,
		RootId:    rootID,
		UserId:    p.botID,
		Message:   p.renderShortPollText(pl),
		// Кастомный тип: браузер и десктоп рисуют карточку плагина. Текст
		// сообщения и кнопки остаются — их видят мобильный клиент и поиск.
		Type: pollPostType,
	}
	// Кнопки и метка опроса: по метке веб-часть делает кнопки компактными.
	p.setAttachments(post, pl)

	created, appErr := p.API.CreatePost(post)
	if appErr != nil {
		return nil, appErr
	}

	pl.PostID = created.Id
	if err := p.savePoll(pl); err != nil {
		if delErr := p.API.DeletePost(created.Id); delErr != nil {
			p.API.LogWarn("Не удалось откатить сообщение-опрос", "post_id", created.Id, "error", delErr.Error())
		}
		return nil, model.NewAppError("createPoll", "polls.save.app_error", nil, err.Error(), http.StatusInternalServerError)
	}
	p.indexPoll(channelID, pl.ID)

	return pl, nil
}

// normalizeOptions вычищает варианты: обрезает пробелы, убирает пустые строки,
// повторы без учёта регистра и лишние варианты сверх настройки.
func normalizeOptions(raw []string, limit int) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, text := range raw {
		text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
		if text == "" {
			continue
		}
		if len([]rune(text)) > maxOptionLen {
			text = string([]rune(text)[:maxOptionLen])
		}
		key := strings.ToLower(text)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, text)
	}
	if len(out) < minOptions {
		return nil, fmt.Errorf("нужно минимум %d варианта", minOptions)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (p *Plugin) handleDialogSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body := http.MaxBytesReader(w, r.Body, maxRequestBody)
	var req model.SubmitDialogRequest
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Cancelled {
		writeJSON(w, model.SubmitDialogResponse{})
		return
	}

	// Окно заполняет браузер пользователя, поэтому авторитетен только заголовок.
	userID := r.Header.Get("Mattermost-User-Id")
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if _, appErr := p.API.GetChannelMember(req.ChannelId, userID); appErr != nil {
		writeJSON(w, model.SubmitDialogResponse{Error: "Нет доступа к этому каналу."})
		return
	}

	cfg := p.getConfiguration()
	resp := model.SubmitDialogResponse{Errors: map[string]string{}}

	question := strings.TrimSpace(submissionString(req.Submission, "question"))
	if question == "" {
		resp.Errors["question"] = "Вопрос не может быть пустым."
	} else if len([]rune(question)) > maxQuestionLen {
		question = string([]rune(question)[:maxQuestionLen])
	}

	raw := strings.Split(submissionString(req.Submission, "options"), "\n")
	options, err := normalizeOptions(raw, cfg.maxOptions())
	if err != nil {
		resp.Errors["options"] = "Нужно как минимум " + fmt.Sprint(minOptions) + " неповторяющихся варианта."
	}

	if len(resp.Errors) > 0 {
		writeJSON(w, resp)
		return
	}

	pl, appErr := p.createPoll(
		req.ChannelId,
		req.State,
		userID,
		question,
		options,
		submissionBool(req.Submission, "multiple"),
		submissionBool(req.Submission, "anonymous"),
		submissionBool(req.Submission, "hide_results"),
	)
	if appErr != nil {
		p.API.LogError("Не удалось создать опрос", "error", appErr.Error())
		resp.Errors = nil
		resp.Error = "Не удалось создать опрос: " + appErr.Error()
		writeJSON(w, resp)
		return
	}

	p.API.LogInfo("Создан опрос", "poll_id", pl.ID, "post_id", pl.PostID, "creator", userID)
	writeJSON(w, model.SubmitDialogResponse{})
}

func submissionString(submission map[string]any, key string) string {
	if v, ok := submission[key].(string); ok {
		return v
	}
	return ""
}

func submissionBool(submission map[string]any, key string) bool {
	if v, ok := submission[key].(bool); ok {
		return v
	}
	return false
}
