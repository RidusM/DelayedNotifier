// nolint: revive,staticcheck
// swagger:meta
package httpt

import (
	"time"
)

// swagger:model CreateNotificationRequest
type CreateNotificationRequest struct {
	UserID      string    `json:"user_id"      example:"550e8400-e29b-41d4-a716-446655440000"`
	Channel     string    `json:"channel"      example:"email"`
	Payload     string    `json:"payload"      example:"Your order #123 is ready!"`
	ScheduledAt time.Time `json:"scheduled_at" example:"2023-10-27T10:00:00Z"`
}

// swagger:model CreateNotificationResponse
type CreateNotificationResponse struct {
	UserID      string    `json:"user_id"      example:"550e8400-e29b-41d4-a716-446655440000"`
	Channel     string    `json:"channel"      example:"email"`
	Payload     string    `json:"payload"      example:"Your order #123 is ready!"`
	ScheduledAt time.Time `json:"scheduled_at" example:"2023-10-27T10:00:00Z"`
	Message     string    `json:"message"      example:"Notification created successfully"`
}

// swagger:model NotificationStatusResponse
type NotificationStatusResponse struct {
	ID          string     `json:"id"                   example:"550e8400-e29b-41d4-a716-446655440001"`
	UserID      string     `json:"user_id"              example:"550e8400-e29b-41d4-a716-446655440000"`
	Channel     string     `json:"channel"              example:"EMAIL"`
	Status      string     `json:"status"               example:"waiting"`
	Payload     string     `json:"payload"              example:"Your order #123 is ready!"`
	ScheduledAt time.Time  `json:"scheduled_at"         example:"2023-10-27T10:00:00Z"`
	SentAt      *time.Time `json:"sent_at,omitempty"    example:"2023-10-27T10:00:05Z"`
	RetryCount  int        `json:"retry_count"          example:"1"`
	LastError   string     `json:"last_error,omitempty" example:"connection timeout"`
	CreatedAt   time.Time  `json:"created_at"           example:"2023-10-26T10:00:00Z"`
}

// swagger:model ErrorResponse
type ErrorResponse struct {
	Error   string `json:"error"             example:"notification not found"`
	Code    string `json:"code,omitempty"    example:"not_found"`
	Details string `json:"details,omitempty" example:"notification with id 123 does not exist"`
}

// swagger:model SuccessResponse
type SuccessResponse struct {
	Message string `json:"message" example:"Notification cancelled successfully"`
}
