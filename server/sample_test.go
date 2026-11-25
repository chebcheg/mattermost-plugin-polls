package main

import (
	"fmt"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

// TestSampleRender печатает сообщение целиком — удобно посмотреть, как выглядят
// полоски и подписи, не поднимая сервер: go test -run Sample -v
func TestSampleRender(t *testing.T) {
	p, _, _ := newTestPlugin(t)
	pl := newPoll("chan", "creator", "Куда идём на обед?", []string{"Столовая", "Кафе", "Пицца", "Своё"}, p.getConfiguration())
	pl.Votes["u1"] = []string{pl.Options[0].ID}
	pl.Votes["u2"] = []string{pl.Options[0].ID}
	pl.Votes["u3"] = []string{pl.Options[1].ID}
	pl.Votes["u4"] = []string{pl.Options[0].ID, pl.Options[3].ID}
	pl.Multiple = true

	fmt.Println("=== сообщение в канале ===")
	fmt.Println(p.renderPoll(pl, ""))
	fmt.Println("=== персональный ответ на свой голос ===")
	fmt.Println(p.renderPoll(pl, "u3"))

	pl.Closed = true
	fmt.Println("=== после завершения ===")
	fmt.Println(p.renderPoll(pl, ""))

	att := p.voteAttachment(pl)
	for _, a := range att[0].Actions {
		fmt.Printf("кнопка: %q -> %s\n", a.Name, a.Integration.URL)
	}

	var _ = model.Post{}
}
