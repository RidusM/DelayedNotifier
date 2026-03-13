package entity

type Channel string

const (
	Telegram Channel = "telegram"
	Email    Channel = "email"
)

func (c Channel) String() string {
	return string(c)
}

func (c Channel) IsValid() bool {
	switch c {
	case Telegram, Email:
		return true
	default:
		return false
	}
}

func ListChannels() []Channel {
	return []Channel{Telegram, Email}
}
