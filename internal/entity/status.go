package entity

type Status string

const (
	StatusWaiting   Status = "waiting"
	StatusInProcess Status = "in_process"
	StatusSent      Status = "sent"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

func (s Status) String() string {
	return string(s)
}

func (s Status) IsValid() bool {
	switch s {
	case StatusWaiting, StatusInProcess, StatusSent, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}
