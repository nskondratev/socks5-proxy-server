package stats

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"text/tabwriter"
)

const (
	command      = "users-stats"
	dataUsageKey = "user_usage_data"
	authDateKey  = "user_auth_date"
)

type redis interface {
	HGetAll(ctx context.Context, key string) (map[string]string, error)
}

type CommandHandler struct {
	redis redis
	out   io.Writer
}

func New(redis redis, out io.Writer) *CommandHandler {
	if out == nil {
		out = os.Stdout
	}

	return &CommandHandler{redis: redis, out: out}
}

func (h *CommandHandler) CanHandle(_ context.Context, commandName string) bool {
	return commandName == command
}

func (h *CommandHandler) Handle(ctx context.Context) error {
	if h.redis == nil {
		return fmt.Errorf("[users-stats] redis dependency is not configured")
	}

	dataUsage, err := h.redis.HGetAll(ctx, dataUsageKey)
	if err != nil {
		return fmt.Errorf("[users-stats] failed to get usage stats: %w", err)
	}

	lastLogin, err := h.redis.HGetAll(ctx, authDateKey)
	if err != nil {
		return fmt.Errorf("[users-stats] failed to get last login stats: %w", err)
	}

	type userStat struct {
		Username string
		Usage    int64
	}

	stats := make([]userStat, 0, len(dataUsage))
	for username, usageStr := range dataUsage {
		usage, parseErr := strconv.ParseInt(usageStr, 10, 64)
		if parseErr != nil {
			return fmt.Errorf("[users-stats] failed to parse usage for user %s: %w", username, parseErr)
		}

		stats = append(stats, userStat{Username: username, Usage: usage})
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Usage > stats[j].Usage
	})

	tw := tabwriter.NewWriter(h.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "#\tUsername\tData usage (in bytes)\tData usage (human readable)\tLast login")

	for i, stat := range stats {
		loginDate := lastLogin[stat.Username]
		if loginDate == "" {
			loginDate = "-"
		}

		fmt.Fprintf(
			tw,
			"%d\t%s\t%d\t%s\t%s\n",
			i+1,
			stat.Username,
			stat.Usage,
			formatBytes(stat.Usage),
			loginDate,
		)
	}

	if err = tw.Flush(); err != nil {
		return fmt.Errorf("[users-stats] failed to print stats: %w", err)
	}

	return nil
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
