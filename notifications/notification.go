package notifications

type Notifiable interface {
	Notify(message string)
}
