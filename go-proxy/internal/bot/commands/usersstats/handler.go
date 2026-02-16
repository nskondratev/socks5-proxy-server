package usersstats

import (
	"context"
	"fmt"
	"strings"

	tele "gopkg.in/telebot.v3"

	"github.com/nskondratev/socks5-proxy-server/internal/bot"
	"github.com/nskondratev/socks5-proxy-server/internal/bot/store"
)

const Command = "/users_stats"

type stateStatsStore interface {
	SetUserState(ctx context.Context, username string, state store.UserState) error
	GetUsersStats(ctx context.Context) ([]store.UserStat, error)
}

type Handler struct{ store stateStatsStore }

func New(store stateStatsStore) *Handler { return &Handler{store: store} }

func (h *Handler) Handle(c tele.Context) error {
	sender := c.Sender()
	if sender == nil || sender.Username == "" {
		return nil
	}

	ctx := bot.GetContext(c)
	stats, err := h.store.GetUsersStats(ctx)
	if err != nil {
		return err
	}

	msg := "<b>Data usage by users:</b>\n\n"
	if len(stats) == 0 {
		msg += "No usage stats."
	} else {
		parts := make([]string, 0, len(stats))
		for _, s := range stats {
			parts = append(parts, fmt.Sprintf("<b>%d.</b> %s (%s): %s", s.Num, s.Username, s.LastAuth, s.Usage))
		}
		msg += strings.Join(parts, "\n")
	}

	if err = h.store.SetUserState(ctx, sender.Username, store.UserState{State: store.StateIdle, Data: map[string]string{}}); err != nil {
		return err
	}

	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeHTML, ReplyMarkup: &tele.ReplyMarkup{RemoveKeyboard: true}})
}
