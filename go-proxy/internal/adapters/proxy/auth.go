package proxy

import (
	"context"
	"log"
	"time"
)

type passwordHashGetter interface {
	GetPasswordHash(ctx context.Context, userName string) (string, error)
}

type passwordComparator interface {
	Valid(input, toCompare string) (bool, error)
}

type lastAuthDateEnqueuer interface {
	EnqueueLastAuthDate(userName string, t time.Time)
}

type Auth struct {
	passwordHashGetter   passwordHashGetter
	passwordComparator   passwordComparator
	lastAuthDateEnqueuer lastAuthDateEnqueuer
}

func NewAuth(
	passwordHashGetter passwordHashGetter,
	passwordComparator passwordComparator,
	lastAuthDateEnqueuer lastAuthDateEnqueuer,
) *Auth {
	return &Auth{
		passwordHashGetter:   passwordHashGetter,
		passwordComparator:   passwordComparator,
		lastAuthDateEnqueuer: lastAuthDateEnqueuer,
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

	if valid && a.lastAuthDateEnqueuer != nil {
		a.lastAuthDateEnqueuer.EnqueueLastAuthDate(user, time.Now())
	}

	return valid
}
