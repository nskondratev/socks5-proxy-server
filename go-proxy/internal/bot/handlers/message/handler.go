package message

import (
	"context"
	"fmt"
	"strings"

	tele "gopkg.in/telebot.v3"

	"github.com/nskondratev/socks5-proxy-server/internal/bot"
	"github.com/nskondratev/socks5-proxy-server/internal/bot/commands/generatepass"
	"github.com/nskondratev/socks5-proxy-server/internal/bot/store"
	"github.com/nskondratev/socks5-proxy-server/internal/config"
)

type storeI interface {
	GetUserState(ctx context.Context, username string) (*store.UserState, error)
	SetUserState(ctx context.Context, username string, state store.UserState) error
	IsUsernameFree(ctx context.Context, username string) (bool, error)
	CreateUser(ctx context.Context, username, password string) error
	DeleteUser(ctx context.Context, username string) error
}

type Handler struct{ store storeI }

func New(store storeI) *Handler { return &Handler{store: store} }

func (h *Handler) Handle(c tele.Context) error {
	if strings.HasPrefix(strings.TrimSpace(c.Text()), "/") {
		return nil
	}

	sender := c.Sender()
	if sender == nil || sender.Username == "" {
		return nil
	}

	ctx := bot.GetContext(c)
	userState, err := h.store.GetUserState(ctx, sender.Username)
	if err != nil {
		return err
	}
	if userState == nil {
		return nil
	}

	text := strings.TrimSpace(c.Text())

	switch userState.State {
	case store.StateIdle:
		return c.Send("Enter command")
	case store.StateCreateUserEnterUsername:
		if text == "" {
			return c.Send("Username can not be empty. Enter the new one.")
		}

		isFree, err := h.store.IsUsernameFree(ctx, text)
		if err != nil {
			return err
		}
		if !isFree {
			return c.Send("This username is already taken. Enter another one.")
		}

		userState.State = store.StateCreateUserEnterPassword
		if userState.Data == nil {
			userState.Data = map[string]string{}
		}
		userState.Data["username"] = text

		suggestedPassword, err := generateSuggestedPassword()
		if err != nil {
			return err
		}

		if err = h.store.SetUserState(ctx, sender.Username, *userState); err != nil {
			return err
		}

		rm := &tele.ReplyMarkup{ResizeKeyboard: true}
		rm.Reply(rm.Row(rm.Text(suggestedPassword)))

		return c.Send("Ok. Enter the password or use the suggested one.", &tele.SendOptions{ReplyMarkup: rm})
	case store.StateCreateUserEnterPassword:
		if text == "" {
			return c.Send("Password can not be empty. Enter the new one.")
		}

		proxyUsername := userState.Data["username"]
		if err := h.store.CreateUser(ctx, proxyUsername, text); err != nil {
			return err
		}

		if err := h.store.SetUserState(ctx, sender.Username, store.UserState{State: store.StateIdle, Data: map[string]string{}}); err != nil {
			return err
		}

		message := fmt.Sprintf(
			"User created. Send this settings to him:\n\n<b>host:</b> %s\n<b>port:</b> %d\n<b>username:</b> %s\n<b>password:</b> %s",
			store.CleanPublicHost(config.PublicURL()),
			config.AppPort(),
			proxyUsername,
			text,
		)

		return c.Send(message, &tele.SendOptions{ParseMode: tele.ModeHTML, ReplyMarkup: &tele.ReplyMarkup{RemoveKeyboard: true}})
	case store.StateDeleteUserEnterUsername:
		isFree, err := h.store.IsUsernameFree(ctx, text)
		if err != nil {
			return err
		}
		if isFree {
			return c.Send("User with provided username does not exists. Enter another one.")
		}

		if err = h.store.DeleteUser(ctx, text); err != nil {
			return err
		}

		if err = h.store.SetUserState(ctx, sender.Username, store.UserState{State: store.StateIdle, Data: map[string]string{}}); err != nil {
			return err
		}

		return c.Send("User deleted.")
	}

	return nil
}

func generateSuggestedPassword() (string, error) {
	return generatepass.Generate(10)
}
