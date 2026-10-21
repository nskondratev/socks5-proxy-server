package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strconv"

	"golang.org/x/sync/singleflight"
)

type passwordHashGetter interface {
	GetPasswordHash(ctx context.Context, userName string) (string, error)
}

type passwordComparator interface {
	Valid(input, toCompare string) (bool, error)
}

type authValidator interface {
	Valid(user, password, userAddr string) bool
}

type Auth struct {
	passwordHashGetter passwordHashGetter
	passwordComparator passwordComparator
}

func NewAuth(
	passwordHashGetter passwordHashGetter,
	passwordComparator passwordComparator,
) *Auth {
	return &Auth{
		passwordHashGetter: passwordHashGetter,
		passwordComparator: passwordComparator,
	}
}

func (a *Auth) Valid(user, password, _ string) bool {
	ctx := context.TODO()

	passwordHash, err := a.passwordHashGetter.GetPasswordHash(ctx, user)
	if err != nil {
		log.Printf("failed to get password hash for user %s: %s", user, err.Error())

		return false
	}

	valid, err := a.passwordComparator.Valid(password, passwordHash)
	if err != nil {
		log.Printf("failed to compare password hash for user %s: %s", user, err.Error())

		return false
	}

	return valid
}

type AuthCacheKey struct {
	user     string
	password string
}

type cache interface {
	Add(key AuthCacheKey, value bool)
	Get(key AuthCacheKey) (value, exists bool)
}

type AuthWithCache struct {
	cache cache
	authValidator
	onSuccessfulAuth func(user string)

	// inflight deduplicates concurrent validations of the same credentials,
	// so a burst of new connections triggers a single expensive password check.
	inflight singleflight.Group
}

func NewAuthWithCache(cache cache, authValidator authValidator, onSuccessfulAuth func(user string)) *AuthWithCache {
	return &AuthWithCache{
		cache:            cache,
		authValidator:    authValidator,
		onSuccessfulAuth: onSuccessfulAuth,
	}
}

func (a *AuthWithCache) Valid(user, password, _ string) bool {
	entryCacheKey := AuthCacheKey{user, password}

	cachedValue, ok := a.cache.Get(entryCacheKey)
	if ok {
		if cachedValue && a.onSuccessfulAuth != nil {
			a.onSuccessfulAuth(user)
		}

		return cachedValue
	}

	result, _, _ := a.inflight.Do(inflightKey(user, password), func() (interface{}, error) {
		// Another validation of the same credentials may have finished right before this one started.
		if value, ok := a.cache.Get(entryCacheKey); ok {
			return value, nil
		}

		value := a.authValidator.Valid(user, password, "")
		a.cache.Add(entryCacheKey, value)

		return value, nil
	})

	calculatedValue, _ := result.(bool)

	if calculatedValue && a.onSuccessfulAuth != nil {
		a.onSuccessfulAuth(user)
	}

	return calculatedValue
}

// inflightKey builds an unambiguous fixed-size key that does not keep the plain password.
func inflightKey(user, password string) string {
	hash := sha256.New()
	// Length prefix keeps (user, password) pairs from colliding after concatenation.
	hash.Write([]byte(strconv.Itoa(len(user)) + ":" + user))
	hash.Write([]byte(password))

	return hex.EncodeToString(hash.Sum(nil))
}
