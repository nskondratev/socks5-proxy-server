package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

const (
	userAuthKey  = "user_auth"
	dataUsageKey = "user_usage_data"
	authDateKey  = "user_auth_date"
	adminUserKey = "user_admin"
	userStateKey = "user_state"
	jsTimeLayout = "2006-01-02T15:04:05.000Z"
	defaultBCost = 10
)

const (
	StateIdle                    = "idle"
	StateCreateUserEnterUsername = "create_user_enter_username"
	StateCreateUserEnterPassword = "create_user_enter_password"
	StateDeleteUserEnterUsername = "delete_user_enter_username"
)

type redis interface {
	HGet(ctx context.Context, key, field string) (string, error)
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	HSet(ctx context.Context, key string, values ...interface{}) error
	HDel(ctx context.Context, key string, fields ...string) error
}

type UserState struct {
	State string            `json:"state"`
	Data  map[string]string `json:"data"`
}

type UserStat struct {
	Num      int
	Username string
	UsageRaw int64
	Usage    string
	LastAuth string
}

type Store struct{ redis redis }

func New(redis redis) *Store { return &Store{redis: redis} }

func (s *Store) GetUserState(ctx context.Context, username string) (*UserState, error) {
	stateJSON, err := s.redis.HGet(ctx, userStateKey, username)
	if errors.Is(err, goredis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user state: %w", err)
	}

	var state UserState
	if err = json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user state: %w", err)
	}

	if state.Data == nil {
		state.Data = map[string]string{}
	}

	return &state, nil
}

func (s *Store) SetUserState(ctx context.Context, username string, state UserState) error {
	if state.Data == nil {
		state.Data = map[string]string{}
	}

	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal user state: %w", err)
	}

	if err = s.redis.HSet(ctx, userStateKey, username, stateJSON); err != nil {
		return fmt.Errorf("failed to save user state: %w", err)
	}

	return nil
}

func (s *Store) UpdateAdminChatID(ctx context.Context, username string, chatID int64) error {
	if err := s.redis.HSet(ctx, adminUserKey, username, chatID); err != nil {
		return fmt.Errorf("failed to update admin chat id: %w", err)
	}

	return nil
}

func (s *Store) GetUsersStats(ctx context.Context) ([]UserStat, error) {
	usageData, err := s.redis.HGetAll(ctx, dataUsageKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage data: %w", err)
	}

	authDateData, err := s.redis.HGetAll(ctx, authDateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get auth date data: %w", err)
	}

	type rawStat struct {
		Username string
		Usage    int64
	}

	rawStats := make([]rawStat, 0, len(usageData))
	for username, rawUsage := range usageData {
		usage, parseErr := strconv.ParseInt(rawUsage, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("failed to parse usage for %s: %w", username, parseErr)
		}

		rawStats = append(rawStats, rawStat{Username: username, Usage: usage})
	}

	sort.Slice(rawStats, func(i, j int) bool {
		return rawStats[i].Usage > rawStats[j].Usage
	})

	stats := make([]UserStat, 0, len(rawStats))
	for i, raw := range rawStats {
		stats = append(stats, UserStat{
			Num:      i + 1,
			Username: raw.Username,
			UsageRaw: raw.Usage,
			Usage:    formatBytes(raw.Usage),
			LastAuth: formatFromNow(authDateData[raw.Username]),
		})
	}

	return stats, nil
}

func (s *Store) CreateUser(ctx context.Context, username, password string) error {
	if _, err := s.redis.HGet(ctx, userAuthKey, username); err == nil {
		return fmt.Errorf("user with provided username already exists")
	} else if !errors.Is(err, goredis.Nil) {
		return fmt.Errorf("failed to check if user exists: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), defaultBCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	if err = s.redis.HSet(ctx, userAuthKey, username, string(hash)); err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

func (s *Store) DeleteUser(ctx context.Context, username string) error {
	if _, err := s.redis.HGet(ctx, userAuthKey, username); errors.Is(err, goredis.Nil) {
		return fmt.Errorf("user with provided username not found")
	} else if err != nil {
		return fmt.Errorf("failed to check if user exists: %w", err)
	}

	if err := s.redis.HDel(ctx, userAuthKey, username); err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	return nil
}

func (s *Store) IsUsernameFree(ctx context.Context, username string) (bool, error) {
	_, err := s.redis.HGet(ctx, userAuthKey, username)
	if errors.Is(err, goredis.Nil) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check if user exists: %w", err)
	}

	return false, nil
}

func (s *Store) GetUsers(ctx context.Context) ([]string, error) {
	usersMap, err := s.redis.HGetAll(ctx, userAuthKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}

	users := make([]string, 0, len(usersMap))
	for u := range usersMap {
		users = append(users, u)
	}

	sort.Strings(users)

	return users, nil
}

func formatBytes(size int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)

	switch {
	case size > gb:
		return fmt.Sprintf("%.2f GB", float64(size)/float64(gb))
	case size > mb:
		return fmt.Sprintf("%.2f MB", float64(size)/float64(mb))
	case size > kb:
		return fmt.Sprintf("%.2f KB", float64(size)/float64(kb))
	default:
		return fmt.Sprintf("%d B", size)
	}
}

func formatFromNow(raw string) string {
	if raw == "" {
		return "-"
	}

	t, err := time.Parse(jsTimeLayout, raw)
	if err != nil {
		return raw
	}

	d := time.Since(t)
	if d < 0 {
		d = -d
	}

	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}

func CleanPublicHost(raw string) string {
	withoutScheme := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	return strings.TrimSuffix(withoutScheme, "/")
}
