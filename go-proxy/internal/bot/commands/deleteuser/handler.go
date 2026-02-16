package deleteuser

import (
	"context"

	tele "gopkg.in/telebot.v3"

	"github.com/nskondratev/socks5-proxy-server/internal/bot"
	"github.com/nskondratev/socks5-proxy-server/internal/bot/store"
)

const Command = "/delete_user"

type stateStore interface {
	SetUserState(ctx context.Context, username string, state store.UserState) error
}

type Handler struct{ store stateStore }

func New(store stateStore) *Handler { return &Handler{store: store} }

func (h *Handler) Handle(c tele.Context) error {
	sender := c.Sender()
	if sender == nil || sender.Username == "" {
		return nil
	}

	if err := h.store.SetUserState(bot.GetContext(c), sender.Username, store.UserState{State: store.StateDeleteUserEnterUsername, Data: map[string]string{}}); err != nil {
		return err
	}

	return c.Send("Enter username to delete.", &tele.SendOptions{ReplyMarkup: &tele.ReplyMarkup{RemoveKeyboard: true}})
}
