//go:generate minimock -i .passwordHashGetter,.passwordComparator,.authValidator,.cache -g

package proxy

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	proxycache "github.com/nskondratev/socks5-proxy-server/proxy/internal/cache"
)

func TestAuth_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		user     string
		password string
		setup    func(t *testing.T, getter *PasswordHashGetterMock, comparator *PasswordComparatorMock)
		want     bool
	}{
		{
			name:     "hash getter error",
			user:     "alice",
			password: "secret",
			setup: func(t *testing.T, getter *PasswordHashGetterMock, _ *PasswordComparatorMock) {
				t.Helper()

				getter.GetPasswordHashMock.
					Expect(context.TODO(), "alice").
					Return("", errors.New("lookup failed"))
			},
			want: false,
		},
		{
			name:     "comparator error",
			user:     "bob",
			password: "guess",
			setup: func(t *testing.T, getter *PasswordHashGetterMock, comparator *PasswordComparatorMock) {
				t.Helper()

				getter.GetPasswordHashMock.
					Expect(context.TODO(), "bob").
					Return("hash", nil)
				comparator.ValidMock.
					Expect("guess", "hash").
					Return(false, errors.New("compare failed"))
			},
			want: false,
		},
		{
			name:     "invalid password",
			user:     "carol",
			password: "bad",
			setup: func(t *testing.T, getter *PasswordHashGetterMock, comparator *PasswordComparatorMock) {
				t.Helper()

				getter.GetPasswordHashMock.
					Expect(context.TODO(), "carol").
					Return("hash", nil)
				comparator.ValidMock.
					Expect("bad", "hash").
					Return(false, nil)
			},
			want: false,
		},
		{
			name:     "valid password",
			user:     "dave",
			password: "good",
			setup: func(t *testing.T, getter *PasswordHashGetterMock, comparator *PasswordComparatorMock) {
				t.Helper()

				getter.GetPasswordHashMock.
					Expect(context.TODO(), "dave").
					Return("hash", nil)
				comparator.ValidMock.
					Expect("good", "hash").
					Return(true, nil)
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			getter := NewPasswordHashGetterMock(t)
			comparator := NewPasswordComparatorMock(t)
			tt.setup(t, getter, comparator)

			auth := NewAuth(getter, comparator)
			got := auth.Valid(tt.user, tt.password, "ignored")
			if got != tt.want {
				t.Fatalf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthWithCache_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		user            string
		password        string
		setup           func(t *testing.T, cache *CacheMock, validator *AuthValidatorMock)
		want            bool
		wantUpdates     int
		wantUpdatedUser string
	}{
		{
			name:     "cache hit invalid credentials",
			user:     "alice",
			password: "cached",
			setup: func(t *testing.T, cache *CacheMock, _ *AuthValidatorMock) {
				t.Helper()

				cache.GetMock.
					Expect(AuthCacheKey{user: "alice", password: "cached"}).
					Return(false, true)
			},
			want:        false,
			wantUpdates: 0,
		},
		{
			name:     "cache miss valid credentials",
			user:     "bob",
			password: "fresh",
			setup: func(t *testing.T, cache *CacheMock, validator *AuthValidatorMock) {
				t.Helper()

				key := AuthCacheKey{user: "bob", password: "fresh"}
				cache.GetMock.
					Expect(key).
					Return(false, false)
				validator.ValidMock.
					Expect("bob", "fresh", "").
					Return(true)
				cache.AddMock.
					Expect(key, true)
			},
			want:            true,
			wantUpdates:     1,
			wantUpdatedUser: "bob",
		},
		{
			name:     "cache hit valid credentials",
			user:     "eve",
			password: "cached-ok",
			setup: func(t *testing.T, cache *CacheMock, _ *AuthValidatorMock) {
				t.Helper()

				cache.GetMock.
					Expect(AuthCacheKey{user: "eve", password: "cached-ok"}).
					Return(true, true)
			},
			want:            true,
			wantUpdates:     1,
			wantUpdatedUser: "eve",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cache := NewCacheMock(t)
			validator := NewAuthValidatorMock(t)
			tt.setup(t, cache, validator)

			updatedCount := 0
			updatedUser := ""
			auth := NewAuthWithCache(cache, validator, func(user string) {
				updatedCount++
				updatedUser = user
			})

			got := auth.Valid(tt.user, tt.password, "ignored")
			if got != tt.want {
				t.Fatalf("Valid() = %v, want %v", got, tt.want)
			}

			if updatedCount != tt.wantUpdates {
				t.Fatalf("onSuccessfulAuth calls = %d, want %d", updatedCount, tt.wantUpdates)
			}

			if tt.wantUpdatedUser != "" && updatedUser != tt.wantUpdatedUser {
				t.Fatalf("updated user = %q, want %q", updatedUser, tt.wantUpdatedUser)
			}
		})
	}
}

// countingCache wraps a real cache and counts Get calls.
type countingCache struct {
	cache
	gets atomic.Int64
}

func (c *countingCache) Get(key AuthCacheKey) (bool, bool) {
	c.gets.Add(1)

	return c.cache.Get(key)
}

func TestAuthWithCache_ValidDeduplicatesConcurrentCacheMisses(t *testing.T) {
	t.Parallel()

	const callers = 50

	authCache := &countingCache{
		cache: proxycache.NewExpirableLRU[AuthCacheKey, bool](10, time.Minute),
	}

	validator := NewAuthValidatorMock(t)
	validator.ValidMock.Set(func(_, _, _ string) bool {
		// Block until every caller has missed the cache, so without deduplication
		// each of them would run its own (expensive) validation.
		deadline := time.Now().Add(2 * time.Second)
		for authCache.gets.Load() < callers && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}

		return true
	})

	var successfulAuths atomic.Int64
	auth := NewAuthWithCache(authCache, validator, func(string) {
		successfulAuths.Add(1)
	})

	var (
		wg      sync.WaitGroup
		invalid atomic.Int64
	)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !auth.Valid("alice", "secret", "ignored") {
				invalid.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := validator.ValidAfterCounter(); got != 1 {
		t.Fatalf("validator calls = %d, want 1", got)
	}
	if got := invalid.Load(); got != 0 {
		t.Fatalf("invalid results = %d, want 0", got)
	}
	if got := successfulAuths.Load(); got != callers {
		t.Fatalf("onSuccessfulAuth calls = %d, want %d", got, callers)
	}
}

func TestAuthWithCache_ValidDoesNotShareResultBetweenDifferentPasswords(t *testing.T) {
	t.Parallel()

	validator := NewAuthValidatorMock(t)
	validator.ValidMock.Set(func(_, password, _ string) bool {
		return password == "good"
	})

	auth := NewAuthWithCache(proxycache.NewExpirableLRU[AuthCacheKey, bool](10, time.Minute), validator, nil)

	var wg sync.WaitGroup
	results := make([]bool, 20)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			password := "bad"
			if i%2 == 0 {
				password = "good"
			}
			results[i] = auth.Valid("alice", password, "ignored")
		}()
	}
	wg.Wait()

	for i, got := range results {
		if want := i%2 == 0; got != want {
			t.Fatalf("result[%d] = %v, want %v", i, got, want)
		}
	}
}
