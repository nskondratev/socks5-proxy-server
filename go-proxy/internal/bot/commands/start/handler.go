package start

import (
	"context"
	"fmt"

	tele "gopkg.in/telebot.v3"

	"github.com/nskondratev/socks5-proxy-server/internal/bot"
	"github.com/nskondratev/socks5-proxy-server/internal/bot/store"
)

const (
	Command = "/start"
)

type userStateSetter interface {
	SetUserState(ctx context.Context, username string, state store.UserState) error
}

type Handler struct {
	store userStateSetter
}

func New(store userStateSetter) *Handler {
	return &Handler{store: store}
}

func (h *Handler) Handle(c tele.Context) error {
	sender := c.Sender()
	if sender == nil || sender.Username == "" {
		return nil
	}

	ctx := bot.GetContext(c)
	if err := h.store.SetUserState(ctx, sender.Username, store.UserState{State: store.StateIdle, Data: map[string]string{}}); err != nil {
		return fmt.Errorf("failed to save user state: %w", err)
	}

	if err := c.Send("Hello! You can manage proxy server."); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}
