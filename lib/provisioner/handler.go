package provisioner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/mailgun/mailgun-go/v4"
	mattermost "github.com/mattermost/mattermost-server/v6/model"
	"github.com/xanzy/go-gitlab"
)

type Handler struct {
	Config        *Config
	UseSSO        bool
	EmailFromAddr string

	// TODO(quad404): convert to interfaces and add test doubles.
	Mattermost   *mattermost.Client4
	Gitlab *gitlab.Client
	Mailgun      mailgun.Mailgun
}

func (h *Handler) Provision(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	ctx := req.Context()

	if req.Body == nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	defer req.Body.Close()

	payload := &OnboardingUser{}
	if err := json.Unmarshal(body, payload); err != nil {
		http.Error(w, "Bad payload", http.StatusBadRequest)
		return
	}

	uid, err := h.provisionGitlab(payload)
	if err != nil {
		log.Printf("[ERROR] Provisioning Gitlab: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.provisionMattermost(payload, uid); err != nil {
		log.Printf("[ERROR] Provisioning Mattermost: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := h.sendWelcomeEmail(ctx, payload, uid); err != nil {
		log.Printf("[ERROR] Sending welcome email: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (h *Handler) provisionGitlab(payload *OnboardingUser) (userID int, _ error) {
	// Create a new user and force them to reset their password.
	user, _, err := h.Gitlab.Users.CreateUser(&gitlab.CreateUserOptions{
		Email:            gitlab.String(payload.Email),
		ResetPassword:    gitlab.Bool(true),
		Username:         gitlab.String(payload.TelegramHandle),
		Name:             gitlab.String(payload.Name),
		SkipConfirmation: gitlab.Bool(true),
	})
	if err != nil {
		return 0, fmt.Errorf("creating user: %w", err)
	} else {
		log.Printf("[INFO] Created gitlab user %d for %s.", user.ID, payload.Email)
	}

	// Temporarily impersonate the user to disable their notifications.
	token, _, err := h.Gitlab.Users.CreateImpersonationToken(user.ID, &gitlab.CreateImpersonationTokenOptions{
		Name:      gitlab.String("provisioning-token"),
		Scopes:    strListPtr("api"),
		ExpiresAt: timePtr(time.Now().Add(24 * time.Hour)),
	})
	if err != nil {
		return 0, fmt.Errorf("creating impersonation token: %w", err)
	} else {
		log.Printf("[INFO] Created impersonation token %d for user %d", token.ID, user.ID)
	}
	defer func() {
		if _, err := h.Gitlab.Users.RevokeImpersonationToken(user.ID, token.ID); err != nil {
			log.Printf("[WARNING] Revoking impersonation token %d for user %d: %v", token.ID, user.ID, err)
		} else {
			log.Printf("[INFO] Revoked impersonation token %d for user %d.", token.ID, user.ID)
		}
	}()

	baseURL := h.Gitlab.BaseURL().String()
	subClient, err := gitlab.NewClient(token.Token, gitlab.WithBaseURL(baseURL))
	if err != nil {
		return 0, fmt.Errorf("creating user-specific subclient: %w", err)
	}
	if _, _, err := subClient.NotificationSettings.UpdateGlobalSettings(&gitlab.NotificationSettingsOptions{
		Level: gitlab.NotificationLevel(gitlab.DisabledNotificationLevel),
	}); err != nil {
		log.Printf("[WARNING] Disabling user %d notifications: %v", user.ID, err)
	} else {
		log.Printf("[INFO] Disabled user %d notifications.", user.ID)
	}

	// Add the user to group and/or project.
	if h.Config.GitlabGroup != "" {
		if _, _, err := h.Gitlab.GroupMembers.AddGroupMember(
			h.Config.GitlabGroup,
			&gitlab.AddGroupMemberOptions{
				UserID:      gitlab.Int(user.ID),
				AccessLevel: gitlab.AccessLevel(gitlab.DeveloperPermissions),
			},
		); err != nil {
			return 0, fmt.Errorf("adding group member %d: %w", user.ID, err)
		} else {
			log.Printf("[INFO] Added gitlab user %d to group %q.", user.ID, h.Config.GitlabGroup)
		}
	}

	if h.Config.GitlabProject != "" {
		if _, _, err := h.Gitlab.ProjectMembers.AddProjectMember(
			h.Config.GitlabProject,
			&gitlab.AddProjectMemberOptions{
				UserID:      gitlab.Int(user.ID),
				AccessLevel: gitlab.AccessLevel(gitlab.DeveloperPermissions),
			},
		); err != nil {
			return 0, fmt.Errorf("adding project member %d: %w", user.ID, err)
		} else {
			log.Printf("[INFO] Added gitlab user %d to project %q.", user.ID, h.Config.GitlabProject)
		}
	}
	return user.ID, nil
}

func (h *Handler) provisionMattermost(payload *OnboardingUser, authID int) error {
	nameParts := strings.Split(payload.Name, " ")
	if len(nameParts) == 0 {
		return errors.New("field name is required")
	}
	firstName := strings.Join(nameParts[:len(nameParts)-1], " ")
	lastName := nameParts[len(nameParts)-1]

	log.Printf("Provisioning user %s (%s) in Mattermost...", payload.TelegramHandle, payload.Name)
	user := &mattermost.User{
		Username:  payload.TelegramHandle,
		Email:     payload.Email,
		FirstName: firstName,
		LastName:  lastName,
	}
	if h.UseSSO {
		s := strconv.Itoa(authID)
		user.AuthService = mattermost.UserAuthServiceGitlab
		user.AuthData = &s
	} else {
		user.Password = mattermost.NewRandomString(12) +
			"?a1Z" // to pass validation
	}

	var userID string
	if userObj, _, err := h.Mattermost.CreateUser(user); err != nil {
		return fmt.Errorf("creating user: %w", err)
	} else {
		log.Printf("User %s / %s created successfully.", user.Username, userObj.Id)
		userID = userObj.Id
	}

	state := map[string][]string{} // key=team, val=channels
	for _, rule := range h.Config.Rules {
		if !has(payload.Skills(), rule.Skill) {
			continue
		}
		// TODO(quad404): Convert from team/chan name to IDs at runtime.
		if _, ok := state[rule.Team]; !ok {
			if _, _, err := h.Mattermost.AddTeamMember(rule.Team, userID); err != nil {
				return fmt.Errorf("adding user to team %q: %w", rule.Team, err)
			} else {
				log.Printf("Added user to team %s.", rule.Team)
				state[rule.Team] = []string{}
			}
		}
		for _, channel := range rule.Channels {
			if has(state[rule.Team], channel) {
				continue
			}
			if _, _, err := h.Mattermost.AddChannelMember(channel, userID); err != nil {
				return fmt.Errorf("adding user to channel %q: %w", channel, err)
			} else {
				log.Printf("Added user to channel %s.", channel)
				state[rule.Team] = append(state[rule.Team], channel)
			}
		}
	}
	return nil
}

func (h *Handler) sendWelcomeEmail(ctx context.Context, payload *OnboardingUser, uid int) error {
	msg := h.Mailgun.NewMessage(h.EmailFromAddr, "Welcome to our server", "", payload.Email)
	msg.SetTemplate(h.Config.MailgunWelcomeTemplate)

	var errors []string
	check := func(err error) {
		if err != nil {
			errors = append(errors, err.Error())
		}
	}
	check(msg.AddTemplateVariable("first", "todo fill this up"))
	check(msg.AddTemplateVariable("second", "todo fill this up too"))
	if len(errors) > 0 {
		return fmt.Errorf("template variables: %s", strings.Join(errors, ", "))
	}
	if resp, id, err := h.Mailgun.Send(ctx, msg); err != nil {
		return fmt.Errorf("sending email: %w", err)
	} else {
		log.Printf("[INFO] Sent welcome email (resp: %s, id: %s).", resp, id)
	}

	return nil
}

func has(list []string, x string) bool {
	for _, y := range list {
		if x == y {
			return true
		}
	}
	return false
}

func strListPtr(a ...string) *[]string { return &a }
func timePtr(a time.Time) *time.Time   { return &a }
