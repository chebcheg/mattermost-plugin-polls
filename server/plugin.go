package main

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const (
	manifestID  = "com.chebcheg.plugin-polls"
	botUsername = "polls"

	routeVote   = "/api/v1/vote"
	routeCancel = "/api/v1/cancel"
	routeVoters = "/api/v1/voters"
	routeEnd    = "/api/v1/end"
	routeDialog = "/api/v1/dialog"

	// Карточка опроса: JSON-API для веб-части и тип поста, который она рисует.
	routeCard       = "/api/v1/card"
	routeCardVote   = "/api/v1/card/vote"
	routeCardCancel = "/api/v1/card/cancel"
	routeCardEnd    = "/api/v1/card/end"
	routeCardVoters = "/api/v1/card/voters"
	pollPostType    = "custom_polls"
)

// Plugin реализует опросы в стиле Телеграма: одно нажатие на вариант — голос
// учтён, результаты с полосками перерисовываются в том же сообщении.
type Plugin struct {
	plugin.MattermostPlugin

	configurationLock sync.RWMutex
	configuration     *configuration

	// mu сериализует чтение-изменение-запись опроса в KV-хранилище плагина.
	// Сервер один, поэтому достаточно мьютекса процесса; для кластера
	// Mattermost понадобился бы распределённый замок.
	mu sync.Mutex

	botID string
}

func main() {
	plugin.ClientMain(&Plugin{})
}

func (p *Plugin) OnActivate() error {
	if err := p.OnConfigurationChange(); err != nil {
		return err
	}

	botID, err := p.API.EnsureBotUser(&model.Bot{
		Username:    botUsername,
		DisplayName: "Опросы",
		Description: "Публикует опросы и обновляет результаты голосования.",
	})
	if err != nil {
		return fmt.Errorf("не удалось создать бота плагина %q: %w", botUsername, err)
	}
	p.botID = botID

	// Аватар бота: синий круг со столбиками диаграммы. Ошибка не мешает работе
	// плагина — опросы создаются и с аватаром по умолчанию.
	if avatar, err := botAvatarPNG(); err != nil {
		p.API.LogWarn("Не удалось нарисовать аватар бота", "error", err.Error())
	} else if appErr := p.API.SetProfileImage(botID, avatar); appErr != nil {
		p.API.LogWarn("Не удалось установить аватар бота", "error", appErr.Error())
	}

	for _, trigger := range []string{"poll", "опрос"} {
		cmd := &model.Command{
			Trigger:          trigger,
			AutoComplete:     true,
			AutoCompleteDesc: "Создать опрос",
			AutoCompleteHint: `[вопрос] ["вариант 1"] ["вариант 2"] …`,
			DisplayName:      "poll",
			Description:      "Опрос с голосованием кнопками",
		}
		if err := p.API.RegisterCommand(cmd); err != nil {
			return fmt.Errorf("не удалось зарегистрировать команду /%s: %w", trigger, err)
		}
	}

	p.API.LogInfo("Плагин опросов запущен", "bot_id", botID)
	return nil
}

// ServeHTTP обслуживает нажатия кнопок в сообщении с опросом и отправку окна
// создания опроса. Пути приходят без префикса /plugins/<id>.
func (p *Plugin) ServeHTTP(_ *plugin.Context, w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case routeVote:
		p.handleVote(w, r)
	case routeCancel:
		p.handleCancel(w, r)
	case routeVoters:
		p.handleVoters(w, r)
	case routeEnd:
		p.handleEnd(w, r)
	case routeDialog:
		p.handleDialogSubmit(w, r)
	case routeCard:
		p.handleCardState(w, r)
	case routeCardVote:
		p.handleCardVote(w, r)
	case routeCardCancel:
		p.handleCardCancel(w, r)
	case routeCardEnd:
		p.handleCardEnd(w, r)
	case routeCardVoters:
		p.handleCardVoters(w, r)
	default:
		http.NotFound(w, r)
	}
}
