package proxy

import (
	"log"
	"time"
)

type LastAuthDate struct {
	UserName string
	Time     time.Time
}

type LastAuthDateQueue struct {
	ch chan<- LastAuthDate
}

func NewLastAuthDateQueue(ch chan<- LastAuthDate) *LastAuthDateQueue {
	return &LastAuthDateQueue{ch: ch}
}

func (q *LastAuthDateQueue) EnqueueLastAuthDate(userName string, t time.Time) {
	select {
	case q.ch <- LastAuthDate{UserName: userName, Time: t}:
	default:
		log.Printf("last auth date queue is full, dropping update for user %s", userName)
	}
}
