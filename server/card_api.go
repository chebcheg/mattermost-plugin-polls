package main

import (
	"encoding/json"
	"net/http"

	"github.com/mattermost/mattermost/server/public/model"
)

// JSON-API для карточки опроса: её рисует веб-часть (кастомный тип поста), и ей
// нужны данные отдельными запросами, а не разбор текста сообщения.

type cardOption struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Count   int    `json:"count"`
	Percent int    `json:"percent"`
	Mine    bool   `json:"mine"`
}

type cardState struct {
	PostID      string       `json:"post_id"`
	Question    string       `json:"question"`
	Options     []cardOption `json:"options"`
	Multiple    bool         `json:"multiple"`
	Anonymous   bool         `json:"anonymous"`
	HideResults bool         `json:"hide_results"`
	Closed      bool         `json:"closed"`
	ShowResults bool         `json:"show_results"`
	TotalVotes  int          `json:"total_votes"`
	VotersCount int          `json:"voters_count"`
	Voted       bool         `json:"voted"`
	CanEnd      bool         `json:"can_end"`
	CanVote     bool         `json:"can_vote"`
	Error       string       `json:"error,omitempty"`
}

type cardVoters struct {
	Options []cardVotersOption `json:"options"`
	Error   string             `json:"error,omitempty"`
}

type cardVotersOption struct {
	Text  string   `json:"text"`
	Names []string `json:"names"`
	More  int      `json:"more"`
}

// votersShowLimit — сколько ников показываем в одном варианте: у популярного
// варианта список иначе растягивается на экран.
const votersShowLimit = 30

func (p *Plugin) cardFor(pl *poll, viewerID string) cardState {
	counts := pl.counts()
	total := pl.totalVotes()
	myVotes := pl.Votes[viewerID]
	voted := len(myVotes) > 0

	state := cardState{
		PostID:      pl.PostID,
		Question:    pl.Question,
		Multiple:    pl.Multiple,
		Anonymous:   pl.Anonymous,
		HideResults: pl.HideResults,
		Closed:      pl.Closed,
		ShowResults: !pl.HideResults || pl.Closed || voted,
		TotalVotes:  total,
		VotersCount: pl.votersCount(),
		Voted:       voted,
		CanVote:     !pl.Closed,
	}

	state.CanEnd = !pl.Closed && p.canEndPoll(pl, viewerID)

	mine := map[string]bool{}
	for _, id := range myVotes {
		mine[id] = true
	}

	for _, opt := range pl.Options {
		state.Options = append(state.Options, cardOption{
			ID:      opt.ID,
			Text:    opt.Text,
			Count:   counts[opt.ID],
			Percent: percentOf(counts[opt.ID], total),
			Mine:    mine[opt.ID],
		})
	}

	return state
}

// pollByPost достаёт опрос по id сообщения — через метку в props.
func (p *Plugin) pollByPost(postID string) (*poll, *model.AppError) {
	post, appErr := p.API.GetPost(postID)
	if appErr != nil {
		return nil, appErr
	}
	pollID, _ := post.GetProp(pollPropName).(string)
	if pollID == "" {
		return nil, model.NewAppError("pollByPost", "polls.not_a_poll", nil, "в сообщении нет метки опроса", http.StatusNotFound)
	}
	pl, err := p.loadPoll(pollID)
	if err != nil {
		return nil, model.NewAppError("pollByPost", "polls.not_found", nil, err.Error(), http.StatusNotFound)
	}
	return pl, nil
}

func writeCardJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (p *Plugin) handleCardState(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-Id")
	if userID == "" {
		writeCardJSON(w, http.StatusUnauthorized, cardState{Error: "Нет сессии"})
		return
	}

	pl, appErr := p.pollByPost(r.URL.Query().Get("post_id"))
	if appErr != nil {
		writeCardJSON(w, http.StatusNotFound, cardState{Error: "Опрос не найден"})
		return
	}
	if !p.member(pl.ChannelID, userID) {
		writeCardJSON(w, http.StatusForbidden, cardState{Error: "Нет доступа к каналу"})
		return
	}

	writeCardJSON(w, http.StatusOK, p.cardFor(pl, userID))
}

type cardActionRequest struct {
	PostID    string   `json:"post_id"`
	OptionID  string   `json:"option_id"`
	OptionIDs []string `json:"option_ids"`
}

func readCardAction(w http.ResponseWriter, r *http.Request) (string, cardActionRequest, bool) {
	userID := r.Header.Get("Mattermost-User-Id")
	if userID == "" {
		writeCardJSON(w, http.StatusUnauthorized, cardState{Error: "Нет сессии"})
		return "", cardActionRequest{}, false
	}
	var req cardActionRequest
	body := http.MaxBytesReader(w, r.Body, maxRequestBody)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeCardJSON(w, http.StatusBadRequest, cardState{Error: "Некорректный запрос"})
		return "", req, false
	}
	return userID, req, true
}

// respondWithState перечитывает опрос и отдаёт карточке свежее состояние.
func (p *Plugin) respondWithState(w http.ResponseWriter, postID, userID string, appErr error) {
	if appErr != nil {
		pl, _ := p.pollByPost(postID)
		state := cardState{Error: appErr.Error()}
		if pl != nil {
			state = p.cardFor(pl, userID)
			state.Error = appErr.Error()
		}
		writeCardJSON(w, http.StatusOK, state)
		return
	}

	pl, loadErr := p.pollByPost(postID)
	if loadErr != nil {
		writeCardJSON(w, http.StatusNotFound, cardState{Error: "Опрос не найден"})
		return
	}
	writeCardJSON(w, http.StatusOK, p.cardFor(pl, userID))
}

func (p *Plugin) handleCardVote(w http.ResponseWriter, r *http.Request) {
	userID, req, ok := readCardAction(w, r)
	if !ok {
		return
	}

	pl, appErr := p.pollByPost(req.PostID)
	if appErr != nil {
		writeCardJSON(w, http.StatusNotFound, cardState{Error: "Опрос не найден"})
		return
	}

	optionIDs := req.OptionIDs
	if len(optionIDs) == 0 && req.OptionID != "" {
		optionIDs = []string{req.OptionID}
	}
	if len(optionIDs) == 0 {
		writeCardJSON(w, http.StatusBadRequest, cardState{Error: "Не выбран вариант"})
		return
	}

	var err error
	for _, optionID := range optionIDs {
		if _, err = p.applyVote(userID, pl.ID, optionID); err != nil {
			break
		}
	}
	p.respondWithState(w, req.PostID, userID, err)
}

func (p *Plugin) handleCardCancel(w http.ResponseWriter, r *http.Request) {
	userID, req, ok := readCardAction(w, r)
	if !ok {
		return
	}

	pl, appErr := p.pollByPost(req.PostID)
	if appErr != nil {
		writeCardJSON(w, http.StatusNotFound, cardState{Error: "Опрос не найден"})
		return
	}

	_, err := p.cancelVote(userID, pl.ID)
	p.respondWithState(w, req.PostID, userID, err)
}

func (p *Plugin) handleCardEnd(w http.ResponseWriter, r *http.Request) {
	userID, req, ok := readCardAction(w, r)
	if !ok {
		return
	}

	pl, appErr := p.pollByPost(req.PostID)
	if appErr != nil {
		writeCardJSON(w, http.StatusNotFound, cardState{Error: "Опрос не найден"})
		return
	}

	_, err := p.closePoll(userID, pl.ID)
	p.respondWithState(w, req.PostID, userID, err)
}

func (p *Plugin) handleCardVoters(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-Id")
	if userID == "" {
		writeCardJSON(w, http.StatusUnauthorized, cardVoters{Error: "Нет сессии"})
		return
	}

	pl, appErr := p.pollByPost(r.URL.Query().Get("post_id"))
	if appErr != nil {
		writeCardJSON(w, http.StatusNotFound, cardVoters{Error: "Опрос не найден"})
		return
	}
	if !p.member(pl.ChannelID, userID) {
		writeCardJSON(w, http.StatusForbidden, cardVoters{Error: "Нет доступа к каналу"})
		return
	}
	if pl.Anonymous {
		writeCardJSON(w, http.StatusForbidden, cardVoters{Error: "Опрос анонимный"})
		return
	}

	result := cardVoters{}
	for _, opt := range pl.Options {
		voters := pl.votersOf(opt.ID)
		names := make([]string, 0, len(voters))
		if len(voters) > 0 {
			users, apiErr := p.API.GetUsersByIds(voters)
			byID := map[string]string{}
			if apiErr == nil {
				format := normalizeNameFormat(p.getConfiguration().VotersNameFormat)
				for _, u := range users {
					byID[u.Id] = displayName(u, format)
				}
			}
			for _, id := range voters {
				if name, found := byID[id]; found {
					names = append(names, name)
				} else {
					names = append(names, "удалённый пользователь")
				}
			}
		}
		more := 0
		if len(names) > votersShowLimit {
			more = len(names) - votersShowLimit
			names = names[:votersShowLimit]
		}
		result.Options = append(result.Options, cardVotersOption{Text: opt.Text, Names: names, More: more})
	}

	writeCardJSON(w, http.StatusOK, result)
}
