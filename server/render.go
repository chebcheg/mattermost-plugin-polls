package main

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

// renderPoll собирает текст сообщения. viewerID непустой только для
// персонального ответа: в общем сообщении канала отметки «мой голос» нет,
// потому что оно одно на всех.
func (p *Plugin) renderPoll(pl *poll, viewerID string) string {
	var b strings.Builder
	b.WriteString("**" + escapeMarkdown(pl.Question) + "**")
	b.WriteString("\n\n")
	b.WriteString(p.pollOptionLines(pl, viewerID))

	b.WriteString("\n\n_" + p.pollMeta(pl) + "_")
	return b.String()
}

// pollOptionLines — строки вариантов: номер, текст, при видимых результатах —
// полоска, процент и число голосов. Используется и в тексте сообщения, и в
// карточке-вложении для мобильного клиента.
func (p *Plugin) pollOptionLines(pl *poll, viewerID string) string {
	counts := pl.counts()
	total := pl.totalVotes()
	myChoice := pl.Votes[viewerID]
	showResults := !pl.HideResults || pl.Closed || len(myChoice) > 0

	winners := map[string]bool{}
	if pl.Closed {
		for _, id := range pl.leaders() {
			winners[id] = true
		}
	}

	lines := make([]string, 0, len(pl.Options))
	for i, opt := range pl.Options {
		line := fmt.Sprintf("%d. %s", i+1, escapeMarkdown(opt.Text))
		for _, id := range myChoice {
			if id == opt.ID {
				line += " ✓"
				break
			}
		}

		if showResults {
			count := counts[opt.ID]
			line += fmt.Sprintf(" — %s\u00a0\u00a0**%d%%** · %d", progressBar(percentOf(count, total), count > 0), percentOf(count, total), count)
			if winners[opt.ID] {
				line += " · победитель"
			}
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// pollMeta — подпись под вариантами: состояние опроса, счётчики, флаги.
func (p *Plugin) pollMeta(pl *poll) string {
	total := pl.totalVotes()
	showResults := !pl.HideResults || pl.Closed

	meta := []string{}
	switch {
	case pl.Closed:
		meta = append(meta, "опрос завершён")
	case !showResults:
		// Пока результаты скрыты, не показываем и счётчик: иначе число
		// проголосовавших утекает до голосования.
		meta = append(meta, "результаты скрыты до вашего голоса")
	}
	if showResults {
		if total == 0 {
			meta = append(meta, "голосов пока нет")
		} else {
			meta = append(meta, pluralRu(total, "голос", "голоса", "голосов"))
			meta = append(meta, pluralRu(pl.votersCount(), "человек", "человека", "человек"))
		}
	}
	if pl.Anonymous {
		meta = append(meta, "анонимное")
	}
	if pl.Multiple {
		meta = append(meta, "можно выбрать несколько")
	}

	return strings.Join(meta, " · ")
}

// optionActions — по кнопке на вариант ответа.
func (p *Plugin) optionActions(pl *poll) []*model.PostAction {
	actions := make([]*model.PostAction, 0, len(pl.Options))
	for i, opt := range pl.Options {
		actions = append(actions, &model.PostAction{
			Id: "vote_" + opt.ID,
			// Подпись без «N.»: мобильный клиент рендерит текст кнопки как
			// markdown, и «1. текст» превращается в нумерованный список, для
			// которого у рендерера кнопки нет обработчика — плашка остаётся
			// пустой. Тире в роли разделителя markdown не трогает.
			Name:    fmt.Sprintf("%d — %s", i+1, shorten(opt.Text, 26)),
			Tooltip: opt.Text,
			Type:    model.PostActionTypeButton,
			Integration: &model.PostActionIntegration{
				URL:     "/plugins/" + manifestID + routeVote,
				Context: map[string]any{"poll_id": pl.ID, "option_id": opt.ID},
			},
		})
	}

	return actions
}

// manageActions — отмена голоса, список проголосовавших, завершение.
func (p *Plugin) manageActions(pl *poll) []*model.PostAction {
	cfg := p.getConfiguration()

	manage := make([]*model.PostAction, 0, 3)
	manage = append(manage, &model.PostAction{
		Id:   "cancel_vote",
		Name: "Отменить голос",
		Type: model.PostActionTypeButton,
		Integration: &model.PostActionIntegration{
			URL:     "/plugins/" + manifestID + routeCancel,
			Context: map[string]any{"poll_id": pl.ID},
		},
	})

	if cfg.ShowVotersButton && !pl.Anonymous {
		manage = append(manage, &model.PostAction{
			Id:   "who_voted",
			Name: "Кто голосовал",
			Type: model.PostActionTypeButton,
			Integration: &model.PostActionIntegration{
				URL:     "/plugins/" + manifestID + routeVoters,
				Context: map[string]any{"poll_id": pl.ID},
			},
		})
	}

	manage = append(manage, &model.PostAction{
		Id:   "end_poll",
		Name: "Завершить",
		Type: model.PostActionTypeButton,
		Integration: &model.PostActionIntegration{
			URL:     "/plugins/" + manifestID + routeEnd,
			Context: map[string]any{"poll_id": pl.ID},
		},
	})

	return manage
}

// voteAttachment — кнопки под сообщением: сначала варианты, затем управление.
// Так выглядит текстовый фолбэк в старых опросах и в клиентах без карточки.
func (p *Plugin) voteAttachment(pl *poll) []*model.MessageAttachment {
	return []*model.MessageAttachment{
		{Actions: p.optionActions(pl)},
		{Actions: p.manageActions(pl)},
	}
}

// mobileCard — то же содержимое одной карточкой-вложением: рамка и акцентная
// полоса рисуются клиентом (в мобильном приложении плагинный компонент
// недоступен, а вложение — доступно и выглядит как карточка).
func (p *Plugin) mobileCard(pl *poll) []*model.MessageAttachment {
	return []*model.MessageAttachment{{
		Color:   "#1c58d9",
		Text:    p.pollOptionLines(pl, ""),
		Footer:  p.pollMeta(pl),
		Actions: append(p.optionActions(pl), p.manageActions(pl)...),
	}}
}

// pollMessage — текст сообщения. Для постов-карточек он короткий: подробности
// лежат во вложении, а на десктопе текст вообще заменяет компонент.
func (p *Plugin) pollMessage(pl *poll, postType string) string {
	if postType == pollPostType {
		return "**" + escapeMarkdown(pl.Question) + "**"
	}
	return p.renderPoll(pl, "")
}

// voteReply — персональный ответ на голос. Полную карточку показываем только
// когда результаты в канале скрыты: там это единственный способ увидеть цифры.
// В остальных случаях хватает строки — иначе каждый голос оставлял бы в ленте
// ещё одну копию опроса.
func (p *Plugin) voteReply(pl *poll, viewerID, head string) string {
	if pl.HideResults && !pl.Closed {
		return head + "\n\n" + p.renderPoll(pl, viewerID)
	}
	choice := pl.Votes[viewerID]
	if len(choice) > 1 {
		names := make([]string, 0, len(choice))
		for _, opt := range pl.Options {
			for _, id := range choice {
				if id == opt.ID {
					names = append(names, escapeMarkdown(opt.Text))
				}
			}
		}
		head += "\n\n_Ваш выбор: " + strings.Join(names, ", ") + "_"
	}
	return head
}

// pollDialog — окно создания опроса. Пять полей вместо флагов командной строки.
func (p *Plugin) pollDialog(rootID string) model.Dialog {
	cfg := p.getConfiguration()
	boolText := func(v bool) string {
		if v {
			return "true"
		}
		return "false"
	}

	return model.Dialog{
		CallbackId:       "create_poll",
		Title:            "Новый опрос",
		IntroductionText: "Вопрос и варианты ответа. После создания под сообщением появятся кнопки — голосование в один клик.",
		SubmitLabel:      "Создать опрос",
		State:            rootID,
		Elements: []model.DialogElement{
			{
				DisplayName: "Вопрос",
				Name:        "question",
				Type:        "text",
				Placeholder: "Куда идём на обед?",
				MaxLength:   maxQuestionLen,
				HelpText:    "Это заголовок сообщения с опросом.",
			},
			{
				DisplayName: "Варианты ответа",
				Name:        "options",
				Type:        "textarea",
				Placeholder: "Столовая\nКафе\nПицца",
				MaxLength:   2000,
				HelpText: fmt.Sprintf(
					"По одному варианту в строке, от %d до %d. Пустые строки и повторы отбрасываются.",
					minOptions, cfg.maxOptions()),
			},
			{
				DisplayName: "Несколько ответов",
				Name:        "multiple",
				Type:        "bool",
				Optional:    true,
				Default:     "false",
				HelpText:    "Каждый сможет отметить несколько вариантов; повторное нажатие снимает выбор.",
			},
			{
				DisplayName: "Анонимное голосование",
				Name:        "anonymous",
				Type:        "bool",
				Optional:    true,
				Default:     boolText(cfg.DefaultAnonymous),
				HelpText:    "Не показывать, кто как голосовал.",
			},
			{
				DisplayName: "Скрывать результаты до голосования",
				Name:        "hide_results",
				Type:        "bool",
				Optional:    true,
				Default:     boolText(cfg.HideResultsByDefault),
				HelpText:    "Полоски и проценты увидит только тот, кто уже проголосовал.",
			},
		},
	}
}

const helpText = "**Опросы**\n\n" +
	"- `/poll` — открыть окно создания опроса\n" +
	"- `/poll Вопрос` — быстрый опрос «Да / Нет»\n" +
	"- `/poll \"Вопрос\" \"Вариант 1\" \"Вариант 2\"` — опрос с готовыми вариантами\n" +
	"- `/poll end` — завершить свой последний открытый опрос в канале\n\n" +
	"Флаги (можно дописать в конце): `--multiple` — несколько ответов, " +
	"`--anonymous` — не показывать, кто голосовал, `--hidden` — результаты только после голосования.\n\n" +
	"Голосование в один клик по кнопке под сообщением; повторное нажатие меняет голос, " +
	"полоски и проценты обновляются в том же сообщении."

// renderShortPollText — короткий текст сообщения для постов-карточек.
func (p *Plugin) renderShortPollText(pl *poll) string {
	return p.pollMessage(pl, pollPostType)
}
