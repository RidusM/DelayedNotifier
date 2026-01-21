// swagger:meta
package httpt

import (
	"time"
)

// swagger:model CreateNotificationRequest
type CreateNotificationRequest struct {
	UserID      string    `json:"user_id" example:"550e8400-e29b-41d4-a716-446655440000"` // Обязательно UUID строкой
	Channel     string    `json:"channel" example:"email"`                                // telegram, email, sms, push
	Payload     string    `json:"payload" example:"Your order #123 is ready!"`            // Содержимое уведомления
	ScheduledAt time.Time `json:"scheduled_at" example:"2023-10-27T10:00:00Z"`            // Когда отправить
}

// swagger:model CreateNotificationResponse
type CreateNotificationResponse struct {
	ID          string    `json:"id" example:"550e8400-e29b-41d4-a716-446655440001"`
	UserID      string    `json:"user_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Channel     string    `json:"channel" example:"email"`
	Status      string    `json:"status" example:"waiting"`
	ScheduledAt time.Time `json:"scheduled_at" example:"2023-10-27T10:00:00Z"`
	CreatedAt   time.Time `json:"created_at" example:"2023-10-26T10:00:00Z"`
	Message     string    `json:"message" example:"Notification created successfully"`
}

// swagger:model NotificationStatusResponse
type NotificationStatusResponse struct {
	ID          string     `json:"id" example:"550e8400-e29b-41d4-a716-446655440001"`
	UserID      string     `json:"user_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Channel     string     `json:"channel" example:"email"`
	Status      string     `json:"status" example:"sent"`
	Payload     string     `json:"payload" example:"Your order #123 is ready!"`
	ScheduledAt time.Time  `json:"scheduled_at" example:"2023-10-27T10:00:00Z"`
	SentAt      *time.Time `json:"sent_at,omitempty" example:"2023-10-27T10:00:05Z"`
	RetryCount  int        `json:"retry_count" example:"1"`
	LastError   string     `json:"last_error,omitempty" example:"connection timeout"`
	CreatedAt   time.Time  `json:"created_at" example:"2023-10-26T10:00:00Z"`
}

// swagger:model ErrorResponse
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// swagger:model SuccessResponse
type SuccessResponse struct {
	Message string `json:"message"`
}
