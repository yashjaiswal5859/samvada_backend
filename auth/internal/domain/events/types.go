package events

type EventType string

const (
	OTPRequested             EventType = "OTPRequested"
	UserRegistered           EventType = "UserRegistered"
	UserLoggedIn             EventType = "UserLoggedIn"
	UserLoggedOut            EventType = "UserLoggedOut"
	TenantProvisionRequested EventType = "TenantProvisionRequested"
	UserOnboarded            EventType = "UserOnboarded"
)
