// swagger:meta
package httpt

import "github.com/ridusm/delayednotifier/internal/entity"

// swagger:model ErrorResponse
type ErrorResponse struct {
	Error string `json:"error"`
}

// swagger:model Notification
type Notification entity.Notification