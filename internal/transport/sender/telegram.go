// sender/telegram.go
package sender

import (
	"context"
	"delayednotifier/internal/entity"
	"fmt"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TelegramSender struct {
	bot *tgbotapi.BotAPI
}

func NewTelegramSender(botToken string) (*TelegramSender, error) {
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}
	return &TelegramSender{bot: bot}, nil
}

func (s *TelegramSender) Send(ctx context.Context, notification entity.Notification) error {
	chatID, err := strconv.ParseInt(notification.RecipientIdentifier, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid telegram chat_id '%s': %w", notification.RecipientIdentifier, err)
	}

	msg := tgbotapi.NewMessage(chatID, notification.Payload)
	_, err = s.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("failed to send telegram message: %w", err)
	}
	return nil
}
