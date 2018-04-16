# Socks5 proxy server
## Prerequisites
* Docker ([Install Docker](https://docs.docker.com/install/))
* Docker Compose ([Install Docker Compose](https://docs.docker.com/compose/install/))

## How to run
- Copy *.env.example* file: `cp .env.example .env`
- Fill in configuration: `nano .env`. Fields:
  - **APP_PORT** - proxy server port (default: 54321),
  - **LOG_LEVEL** - log level (default: INFO),
  - **REQUIRE_AUTH** - if set to 1, anonymous users are not allowed.
- Start application: `docker-compose up -d`

## CLI commands
In all commands you need to call js-script in app docker container.  
So you need to find out container name with proxy application by running the following command:
```bash
docker-compose ps
```

For example, it will be `socks5proxy_proxy_1`.

In all the following commands you need to replace `socks5proxy_proxy_1` with the yours container name. 

### Create user
```bash
docker exec -it socks5proxy_proxy_1 sh -c 'exec node scripts/create-user.js'
```

### Delete user
```bash
docker exec -it socks5proxy_proxy_1 sh -c 'exec node scripts/delete-user.js'
```

### Show users statistics
```bash
docker exec -it socks5proxy_proxy_1 sh -c 'exec node scripts/users-stats.js'
```
