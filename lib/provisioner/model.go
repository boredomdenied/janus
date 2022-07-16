package provisioner

import (
	"strings"
	"time"
)

// Config stores configuration for the user provisioner.
type Config struct {
	GitlabGroup   string
	GitlabProject string

	Rules []Rule

	MailgunWelcomeTemplate string
}

// Rule encodes an account setup operation based on user's skills.
type Rule struct {
	Name     string   // description, for human consumption.
	Skill    string   // user's skill, used for matching the rule.
	Team     string   // add the user to this team.
	Channels []string // add the user to these channels.
}

type NocoObject struct {
	ID        int
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OnboardingUser struct {
	NocoObject

	TelegramHandle string `json:"telegram_handle"`
	Name           string
	Email          string
	RawSkills      string `json:"skills"` // comma separated values
}

func (u *OnboardingUser) Skills() []string {
	return strings.Split(u.RawSkills, ",")
}
