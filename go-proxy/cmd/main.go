package main

import (
	"context"
	"log"
	"net"
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
		func(user string) {
			if err := usersService.UpdateLastAuthDate(ctx, user); err != nil {
				log.Printf("failed to update auth date for user %s: %s", user, err.Error())
			}
		},
	)

	dialer := &net.Dialer{}

	// Create a SOCKS5 server
	server := socks5.NewServer(
		socks5.WithLogger(socks5.NewLogger(log.New(os.Stdout, "socks5: ", log.LstdFlags))),
		socks5.WithDialAndRequest(func(ctx context.Context, network, addr string, request *socks5.Request) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}

			username, ok := getUsernameFromRequest(request)
			if !ok {
				return conn, nil
			}

			return proxy.NewUsageTrackedConn(conn, func(dataLen int64) {
				if err := usersService.IncreaseDataUsage(ctx, username, dataLen); err != nil {
					log.Printf("failed to increase data usage for user %s: %s", username, err.Error())
				}
			}), nil
		}),
		socks5.WithCredential(authCredentialValidator),
	)

	// Create SOCKS5 proxy on localhost port 8000
	if err := server.ListenAndServe("tcp", ":8000"); err != nil {
		panic(err)
	}
}

func getUsernameFromRequest(request *socks5.Request) (string, bool) {
	if request == nil || request.AuthContext == nil || request.AuthContext.Payload == nil {
		return "", false
	}

	username, ok := request.AuthContext.Payload["Username"]
	if !ok || username == "" {
		username = request.AuthContext.Payload["username"]
	}

	if username == "" {
		return "", false
	}

	return username, true
}
