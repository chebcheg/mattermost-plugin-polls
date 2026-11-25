package main

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

func (p *Plugin) ExecuteCommand(_ *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	rest := strings.TrimSpace(stripTrigger(args.Command))
	lower := strings.ToLower(rest)

	switch lower {
	case "":
		return p.commandDialog(args)
	case "help", "справка", "?", "--help", "-h":
		return p.respond(helpText), nil
	case "end", "завершить", "stop", "стоп":
		return p.commandEnd(args)
	}

	return p.commandCreate(args, rest)
}

// stripTrigger убирает из строки команды ведущий «/» и слово-триггер.
func stripTrigger(command string) string {
	command = strings.TrimPrefix(strings.TrimSpace(command), "/")
	if idx := strings.IndexFunc(command, unicode.IsSpace); idx >= 0 {
		return command[idx+1:]
	}
	return ""
}

func (p *Plugin) respond(text string) *model.CommandResponse {
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         text,
	}
}

func (p *Plugin) commandDialog(args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	if args.TriggerId == "" {
		return p.respond("Окно создания опроса не открылось. Опроса можно создать и командой: " +
			"`/poll \"Вопрос\" \"Вариант 1\" \"Вариант 2\"`."), nil
	}

	appErr := p.API.OpenInteractiveDialog(model.OpenDialogRequest{
		TriggerId: args.TriggerId,
		URL:       "/plugins/" + manifestID + routeDialog,
		Dialog:    p.pollDialog(args.RootId),
	})
	if appErr != nil {
		p.API.LogWarn("Не удалось открыть окно создания опроса", "error", appErr.Error())
		return p.respond("Не удалось открыть окно создания опроса: " + appErr.Error()), nil
	}

	// Ответа в канал нет: опрос появится после заполнения окна.
	return &model.CommandResponse{}, nil
}

func (p *Plugin) commandEnd(args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	for _, pollID := range p.channelPolls(args.ChannelId) {
		pl, err := p.loadPoll(pollID)
		if err != nil || pl.Closed {
			continue
		}
		if !p.canEndPoll(pl, args.UserId) {
			continue
		}
		closed, err := p.closePoll(args.UserId, pollID)
		if err != nil {
			return p.respond(errorText(err)), nil
		}
		return p.respond("Опрос завершён.\n\n" + p.renderPoll(closed, args.UserId)), nil
	}

	return p.respond("В этом канале нет открытых опросов, которые вы можете завершить. " +
		"Опрос завершается кнопкой «Завершить» под сообщением."), nil
}

// commandCreate разбирает «/poll Вопрос» и «/poll "Вопрос" "Вариант 1" "Вариант 2"`.
func (p *Plugin) commandCreate(args *model.CommandArgs, rest string) (*model.CommandResponse, *model.AppError) {
	tokens := tokenize(rest)
	if len(tokens) == 0 {
		return p.commandDialog(args)
	}

	var words []string
	quoted := false
	for _, tok := range tokens {
		if strings.HasPrefix(tok.text, "--") {
			continue
		}
		words = append(words, tok.text)
		quoted = quoted || tok.quoted
	}
	if len(words) == 0 {
		return p.commandDialog(args)
	}

	multiple := false
	anonymous := false
	hidden := false
	for _, tok := range tokens {
		switch strings.ToLower(tok.text) {
		case "--multiple", "--multi", "--несколько":
			multiple = true
		case "--anonymous", "--anon", "--анонимное":
			anonymous = true
		case "--hidden", "--hide", "--скрытые":
			hidden = true
		}
	}

	cfg := p.getConfiguration()
	question := words[0]
	var options []string
	if quoted {
		options = words[1:]
		if len(options) < minOptions {
			return p.respond(fmt.Sprintf(
				"quote-форма: нужен вопрос и минимум %d варианта в кавычках, например "+
					"`/poll \"Куда обедаем?\" \"Столовая\" \"Кафе\"`.", minOptions)), nil
		}
	} else {
		// Без кавычек вся строка — вопрос, варианты по умолчанию «Да / Нет».
		question = strings.Join(words, " ")
		options = []string{"Да", "Нет"}
	}

	options, err := normalizeOptions(options, cfg.maxOptions())
	if err != nil {
		return p.respond(err.Error()), nil
	}
	if len([]rune(question)) > maxQuestionLen {
		question = string([]rune(question)[:maxQuestionLen])
	}

	pl, appErr := p.createPoll(args.ChannelId, args.RootId, args.UserId, question, options, multiple, anonymous, hidden)
	if appErr != nil {
		p.API.LogError("Не удалось создать опрос командой", "error", appErr.Error())
		return p.respond("Не удалось создать опрос: " + appErr.Error()), nil
	}
	p.API.LogInfo("Создан опрос", "poll_id", pl.ID, "post_id", pl.PostID, "creator", args.UserId)

	return &model.CommandResponse{}, nil
}

type token struct {
	text   string
	quoted bool
}

// tokenize режет строку по пробелам, уважая двойные кавычки. Признак «токен был
// в кавычках» нужен, чтобы отличить опрос «Да / Нет» от опроса с вариантами.
func tokenize(input string) []token {
	var out []token
	var b strings.Builder
	inQuote := false
	quoted := false
	has := false

	flush := func() {
		if has {
			out = append(out, token{text: b.String(), quoted: quoted})
		}
		b.Reset()
		has = false
		quoted = false
	}

	for _, r := range input {
		switch {
		case r == '"' || r == '«' || r == '»':
			inQuote = !inQuote
			quoted = true
			has = true
		case unicode.IsSpace(r) && !inQuote:
			flush()
		default:
			b.WriteRune(r)
			has = true
		}
	}
	flush()

	return out
}
