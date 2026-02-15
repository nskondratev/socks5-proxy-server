package main

import (
	"context"
	"io"
	"log"
	"os"

	"github.com/things-go/go-socks5"

	"github.com/nskondratev/socks5-proxy-server/internal/adapters/proxy"
	"github.com/nskondratev/socks5-proxy-server/internal/config"
	"github.com/nskondratev/socks5-proxy-server/internal/password"
	"github.com/nskondratev/socks5-proxy-server/internal/redis"
	"github.com/nskondratev/socks5-proxy-server/internal/services/users"
)

func main() {
	redisCli := redis.New(config.RedisHost(), config.RedisPort(), config.RedisDB())

	passwords := password.New()

	usersService := users.New(redisCli)
	lastAuthDateQueue := make(chan proxy.LastAuthDate, 100)
	go func() {
		ctx := context.Background()
		for item := range lastAuthDateQueue {
			if err := usersService.SetLastAuthDate(ctx, item.UserName, item.Time); err != nil {
				log.Printf("failed to set last auth date for user %s: %s", item.UserName, err.Error())
			}
		}
	}()

	// Create a SOCKS5 server
	server := socks5.NewServer(
		socks5.WithLogger(socks5.NewLogger(log.New(os.Stdout, "socks5: ", log.LstdFlags))),
		socks5.WithConnectMiddleware(func(ctx context.Context, writer io.Writer, request *socks5.Request) error {
			log.Println("new connection")

			return nil
		}),
		socks5.WithCredential(proxy.NewAuth(usersService, passwords, proxy.NewLastAuthDateQueue(lastAuthDateQueue))),
	)

	// Create SOCKS5 proxy on localhost port 8000
	if err := server.ListenAndServe("tcp", ":8000"); err != nil {
		panic(err)
	}
}
