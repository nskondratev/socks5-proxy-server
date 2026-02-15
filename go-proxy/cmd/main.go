package main

import (
	"context"
	"io"
	"log"
	"os"

	"github.com/things-go/go-socks5"

	"github.com/nskondratev/socks5-proxy-server/internal/adapters/proxy"
	"github.com/nskondratev/socks5-proxy-server/internal/cache"
	"github.com/nskondratev/socks5-proxy-server/internal/cli"
	"github.com/nskondratev/socks5-proxy-server/internal/config"
	"github.com/nskondratev/socks5-proxy-server/internal/password"
	"github.com/nskondratev/socks5-proxy-server/internal/redis"
	"github.com/nskondratev/socks5-proxy-server/internal/services/users"
)

func main() {
	// TODO: implement graceful shutdown
	ctx := context.Background()

	redisCli := redis.New(config.RedisHost(), config.RedisPort(), config.RedisDB())

	passwords := password.New()
	usersService := users.New(redisCli)

	// Handle CLI commands, if passed
	if handled := cli.HandleCLICommand(ctx, &cli.CLICommandsDeps{Redis: redisCli}); handled {
		return
	}

	authCredentialValidator := proxy.NewAuthWithCache(
		cache.NewExpirableLRU[proxy.AuthCacheKey, bool](config.AuthCacheMaxSize(), config.AuthCacheTTL()),
		proxy.NewAuth(usersService, passwords),
	)

	// Create a SOCKS5 server
	server := socks5.NewServer(
		socks5.WithLogger(socks5.NewLogger(log.New(os.Stdout, "socks5: ", log.LstdFlags))),
		socks5.WithConnectMiddleware(func(ctx context.Context, writer io.Writer, request *socks5.Request) error {
			log.Println("new connection")

			return nil
		}),
		socks5.WithCredential(authCredentialValidator),
	)

	// Create SOCKS5 proxy on localhost port 8000
	if err := server.ListenAndServe("tcp", ":8000"); err != nil {
		panic(err)
	}
}
