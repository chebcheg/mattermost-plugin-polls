package main

import (
	"encoding/json"
	"errors"
	"sort"

	"github.com/mattermost/mattermost/server/public/model"
)

var (
	errPollNotFound   = errors.New("опрос не найден")
	errPollClosed     = errors.New("опрос уже завершён")
	errOptionNotFound = errors.New("такой вариант в опросе не найден")
	errNoAccess       = errors.New("нет доступа к каналу с этим опросом")
)

// pollOption — один вариант ответа. ID короткий и не меняется, чтобы кнопка
// оставалась рабочей после правки текста опроса.
type pollOption struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// poll — состояние голосования. Хранится в KV плагина под ключом poll_<ID>,
// сообщение в канале — только его отрисовка.
type poll struct {
	ID          string              `json:"id"`
	PostID      string              `json:"post_id"`
	ChannelID   string              `json:"channel_id"`
	CreatorID   string              `json:"creator_id"`
	Question    string              `json:"question"`
	Options     []pollOption        `json:"options"`
	Multiple    bool                `json:"multiple"`
	Anonymous   bool                `json:"anonymous"`
	HideResults bool                `json:"hide_results"`
	Closed      bool                `json:"closed"`
	ClosedAt    int64               `json:"closed_at"`
	CreateAt    int64               `json:"create_at"`
	Votes       map[string][]string `json:"votes"`
}

func newPoll(channelID, creatorID, question string, options []string, cfg *configuration) *poll {
	pl := &poll{
		ID:          model.NewId(),
		ChannelID:   channelID,
		CreatorID:   creatorID,
		Question:    question,
		Multiple:    false,
		Anonymous:   cfg.DefaultAnonymous,
		HideResults: cfg.HideResultsByDefault,
		CreateAt:    model.GetMillis(),
		Votes:       map[string][]string{},
	}
	for _, text := range options {
		pl.Options = append(pl.Options, pollOption{ID: model.NewId()[:8], Text: text})
	}
	return pl
}

func (pl *poll) optionByID(id string) *pollOption {
	for i := range pl.Options {
		if pl.Options[i].ID == id {
			return &pl.Options[i]
		}
	}
	return nil
}

func (pl *poll) hasVoted(userID string) bool {
	return len(pl.Votes[userID]) > 0
}

func (pl *poll) hasOption(userID, optionID string) bool {
	for _, id := range pl.Votes[userID] {
		if id == optionID {
			return true
		}
	}
	return false
}

func (pl *poll) totalVotes() int {
	total := 0
	for _, ids := range pl.Votes {
		total += len(ids)
	}
	return total
}

func (pl *poll) votersCount() int {
	n := 0
	for _, ids := range pl.Votes {
		if len(ids) > 0 {
			n++
		}
	}
	return n
}

func (pl *poll) counts() map[string]int {
	counts := make(map[string]int, len(pl.Options))
	for _, ids := range pl.Votes {
		for _, id := range ids {
			counts[id]++
		}
	}
	return counts
}

// leaders возвращает варианты с максимальным числом голосов (для победителя
// после завершения опроса). Пустой список, если голосов нет.
func (pl *poll) leaders() []string {
	counts := pl.counts()
	max := 0
	for _, n := range counts {
		if n > max {
			max = n
		}
	}
	if max == 0 {
		return nil
	}
	var out []string
	for _, opt := range pl.Options {
		if counts[opt.ID] == max {
			out = append(out, opt.ID)
		}
	}
	return out
}

// votersOf — ники проголосовавших за вариант, по алфавиту.
func (pl *poll) votersOf(optionID string) []string {
	var out []string
	for userID, ids := range pl.Votes {
		for _, id := range ids {
			if id == optionID {
				out = append(out, userID)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// --- хранилище ---------------------------------------------------------------

func pollKey(pollID string) string     { return "poll_" + pollID }
func indexKey(channelID string) string { return "polls_" + channelID }

const indexLimit = 100

// pollPropName — ключ в props сообщения, по которому веб-часть узнаёт опрос.
const pollPropName = "polls_poll_id"

func (p *Plugin) loadPoll(pollID string) (*poll, error) {
	if pollID == "" {
		return nil, errPollNotFound
	}
	data, appErr := p.API.KVGet(pollKey(pollID))
	if appErr != nil {
		return nil, appErr
	}
	if len(data) == 0 {
		return nil, errPollNotFound
	}
	var pl poll
	if err := json.Unmarshal(data, &pl); err != nil {
		return nil, err
	}
	if pl.Votes == nil {
		pl.Votes = map[string][]string{}
	}
	return &pl, nil
}

func (p *Plugin) savePoll(pl *poll) error {
	data, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	if appErr := p.API.KVSet(pollKey(pl.ID), data); appErr != nil {
		return appErr
	}
	return nil
}

func (p *Plugin) indexPoll(channelID, pollID string) {
	ids := p.channelPolls(channelID)
	for _, id := range ids {
		if id == pollID {
			return
		}
	}
	ids = append([]string{pollID}, ids...)
	if len(ids) > indexLimit {
		ids = ids[:indexLimit]
	}
	if data, err := json.Marshal(ids); err == nil {
		p.API.KVSet(indexKey(channelID), data)
	}
}

func (p *Plugin) channelPolls(channelID string) []string {
	data, appErr := p.API.KVGet(indexKey(channelID))
	if appErr != nil || len(data) == 0 {
		return nil
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil
	}
	return ids
}

// member проверяет, что пользователь состоит в канале опроса: кнопки видят только
// участники, но эндпоинт вызывается и напрямую.
func (p *Plugin) member(pollChannelID, userID string) bool {
	_, appErr := p.API.GetChannelMember(pollChannelID, userID)
	return appErr == nil
}

// --- голосование -------------------------------------------------------------

type voteResult struct {
	Poll *poll
	Note string // что произошло: «учтён», «убрано» — для подтверждения
}

func (p *Plugin) applyVote(userID, pollID, optionID string) (*voteResult, error) {
	p.mu.Lock()

	pl, err := p.loadPoll(pollID)
	if err != nil {
		p.mu.Unlock()
		return nil, err
	}
	if !p.member(pl.ChannelID, userID) {
		p.mu.Unlock()
		return nil, errNoAccess
	}
	if pl.Closed {
		p.mu.Unlock()
		return nil, errPollClosed
	}
	opt := pl.optionByID(optionID)
	if opt == nil {
		p.mu.Unlock()
		return nil, errOptionNotFound
	}

	note := "выбран"
	if pl.Multiple {
		if pl.hasOption(userID, optionID) {
			p.removeVote(pl, userID, optionID)
			note = "снят"
		} else {
			pl.Votes[userID] = append(pl.Votes[userID], optionID)
		}
	} else {
		pl.Votes[userID] = []string{optionID}
	}

	if err := p.savePoll(pl); err != nil {
		p.mu.Unlock()
		return nil, err
	}
	p.mu.Unlock()

	p.refreshPost(pl)
	return &voteResult{Poll: pl, Note: note}, nil
}

func (p *Plugin) removeVote(pl *poll, userID, optionID string) {
	ids := pl.Votes[userID]
	kept := ids[:0]
	for _, id := range ids {
		if id != optionID {
			kept = append(kept, id)
		}
	}
	if len(kept) == 0 {
		delete(pl.Votes, userID)
		return
	}
	pl.Votes[userID] = kept
}

func (p *Plugin) cancelVote(userID, pollID string) (*voteResult, error) {
	p.mu.Lock()

	pl, err := p.loadPoll(pollID)
	if err != nil {
		p.mu.Unlock()
		return nil, err
	}
	if !p.member(pl.ChannelID, userID) {
		p.mu.Unlock()
		return nil, errNoAccess
	}
	if pl.Closed {
		p.mu.Unlock()
		return nil, errPollClosed
	}
	if !pl.hasVoted(userID) {
		p.mu.Unlock()
		return nil, errors.New("вы ещё не голосовали в этом опросе")
	}

	delete(pl.Votes, userID)
	if err := p.savePoll(pl); err != nil {
		p.mu.Unlock()
		return nil, err
	}
	p.mu.Unlock()

	p.refreshPost(pl)
	return &voteResult{Poll: pl, Note: "отменён"}, nil
}

// canEndPoll — одно правило для сервера, карточки и команды: автор опроса,
// любой участник при включённой настройке, либо администратор — но только если
// это разрешено отдельной настройкой. Раньше права администратора подразумевались
// всегда, и «Завершить» висело у любого админа в чужом опросе.
func (p *Plugin) canEndPoll(pl *poll, userID string) bool {
	cfg := p.getConfiguration()
	if pl.CreatorID == userID || cfg.AnyoneCanEnd {
		return true
	}
	return cfg.AdminsCanEnd && p.API.HasPermissionTo(userID, model.PermissionManageSystem)
}

func (p *Plugin) closePoll(userID, pollID string) (*poll, error) {
	p.mu.Lock()

	pl, err := p.loadPoll(pollID)
	if err != nil {
		p.mu.Unlock()
		return nil, err
	}
	if !p.member(pl.ChannelID, userID) {
		p.mu.Unlock()
		return nil, errNoAccess
	}
	if pl.Closed {
		p.mu.Unlock()
		return pl, errPollClosed
	}
	if !p.canEndPoll(pl, userID) {
		p.mu.Unlock()
		return pl, errors.New("завершить опрос может только автор")
	}

	pl.Closed = true
	pl.ClosedAt = model.GetMillis()
	if err := p.savePoll(pl); err != nil {
		p.mu.Unlock()
		return nil, err
	}
	p.mu.Unlock()

	p.refreshPost(pl)
	return pl, nil
}

// refreshPost перерисовывает сообщение с опросом. Ошибку не пробрасываем дальше:
// голос уже сохранён, а следующее обновление (или перезагрузка поста) покажет его.
func (p *Plugin) refreshPost(pl *poll) {
	if pl.PostID == "" {
		return
	}
	// Перечитываем состояние: два голоса подряд идут каждый со своей копией,
	// и рисовать надо последнюю, иначе более ранний голос перетрёт более поздний.
	p.mu.Lock()
	if fresh, err := p.loadPoll(pl.ID); err == nil {
		pl = fresh
	}
	p.mu.Unlock()
	post, appErr := p.API.GetPost(pl.PostID)
	if appErr != nil {
		p.API.LogWarn("Не удалось прочитать сообщение опроса", "poll_id", pl.ID, "post_id", pl.PostID, "error", appErr.Error())
		return
	}
	post.Message = p.pollMessage(pl, post.Type)
	p.setAttachments(post, pl)
	if _, appErr := p.API.UpdatePost(post); appErr != nil {
		p.API.LogWarn("Не удалось обновить сообщение опроса", "poll_id", pl.ID, "post_id", pl.PostID, "error", appErr.Error())
	}
}

func (p *Plugin) setAttachments(post *model.Post, pl *poll) {
	post.AddProp(pollPropName, pl.ID)
	if pl.Closed {
		post.AddProp(model.PostPropsAttachments, []*model.MessageAttachment{})
		return
	}
	if post.Type == pollPostType {
		post.AddProp(model.PostPropsAttachments, p.mobileCard(pl))
		return
	}
	post.AddProp(model.PostPropsAttachments, p.voteAttachment(pl))
}
