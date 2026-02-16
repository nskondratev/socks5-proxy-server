package generatepass

import (
	"crypto/rand"
	"math/big"
	"strconv"
	"strings"

	tele "gopkg.in/telebot.v3"
)

const Command = "/generate_pass"

const (
	defaultLen = 10
	lowers     = "abcdefghijklmnopqrstuvwxyz"
	uppers     = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digits     = "0123456789"
)

type Handler struct{}

func New() *Handler { return &Handler{} }

func (h *Handler) Handle(c tele.Context) error {
	ln := defaultLen
	if payload := strings.TrimSpace(c.Message().Payload); payload != "" {
		if n, err := strconv.Atoi(payload); err == nil && n > 0 {
			ln = n
		}
	}

	pass, err := Generate(ln)
	if err != nil {
		return err
	}

	return c.Send(pass)
}

func Generate(length int) (string, error) {
	if length < 3 {
		length = 3
	}

	all := lowers + uppers + digits
	result := []byte{lowers[0], uppers[0], digits[0]}
	for len(result) < length {
		i, err := rand.Int(rand.Reader, big.NewInt(int64(len(all))))
		if err != nil {
			return "", err
		}
		result = append(result, all[i.Int64()])
	}

	for i := len(result) - 1; i > 0; i-- {
		jBig, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", err
		}
		j := int(jBig.Int64())
		result[i], result[j] = result[j], result[i]
	}

	return string(result), nil
}
