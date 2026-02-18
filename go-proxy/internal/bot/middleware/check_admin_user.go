package middleware

import (
	"context"

	tele "gopkg.in/telebot.v3"

	"github.com/nskondratev/socks5-proxy-server/internal/bot"
)

type adminService interface {
	IsAdmin(ctx context.Context, username string) (bool, error)
}

func RestrictByAdminUserID(adminService adminService) func(tele.HandlerFunc) tele.HandlerFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			sender := c.Sender()
			if sender == nil || sender.Username == "" {
				return c.Send("Sorry, this functionality is available only for admin users.")
			}

			isAdmin, err := adminService.IsAdmin(bot.GetContext(c), sender.Username)
			if err != nil {
				return err
			}

			if !isAdmin {
				return c.Send("Sorry, this functionality is available only for admin users.")
			}

			return next(c)
		}
	}
}
