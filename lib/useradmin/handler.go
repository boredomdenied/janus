package useradmin

import (
	"log"
	"net/http"
	"sort"
	"strconv"

	"github.com/julienschmidt/httprouter"
	mattermost "github.com/mattermost/mattermost-server/v6/model"
	"github.com/unrolled/render"
	"github.com/xanzy/go-gitlab"
	"gitlab.operationuplift.work/operations/development/janus/lib/auth"
)

type Handler struct {
	*render.Render

	Config     *Config
	Gitlab     *gitlab.Client
	Mattermost *mattermost.Client4
}

// RegisterRoutes configures the router with the routes to handle useradmin
// requests. Prefix must not contain a trailing slash.
func (h *Handler) RegisterRoutes(r *httprouter.Router, prefix string) {
	r.GET(prefix+"/", auth.MustHaveGroup(h.Render, "management", h.listUsers))
	r.POST(prefix+"/", auth.MustHaveGroup(h.Render, "management", h.updateUsers))
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	type userData struct {
		ID       int
		Username string
		Name     string
		Email    string
		State    string
		IsAdmin  bool
		Groups   []string
	}
	type userListData struct {
		Users      []userData
		Pages      pagination
		GroupClass map[string]string
	}

	users, resp, err := h.Gitlab.Users.ListUsers(&gitlab.ListUsersOptions{
		OrderBy: gitlab.String("username"),
		ListOptions: gitlab.ListOptions{
			Page:    intValue(r, "page", 1),
			PerPage: intValue(r, "show", 25),
		},
	})

	if err != nil {
		log.Printf("[ERROR] Listing gitlab users: %v", err)
		h.HTML(w, http.StatusInternalServerError, "error", "Error listing users.")
		return
	}

	// TODO(quad404): cache this!
	groups := map[string]map[int]bool{}
	for _, group := range h.Config.Groups {
		groups[group.Name] = h.getGroupMembers(group.GitlabID)
	}

	data := &userListData{
		Pages:      paginate(resp),
		GroupClass: groupClass(h.Config.Groups),
	}
	for _, u := range users {
		data.Users = append(data.Users, userData{
			ID:       u.ID,
			Username: u.Username,
			Name:     u.Name,
			Email:    u.Email,
			State:    u.State,
			IsAdmin:  u.IsAdmin,
			Groups:   groupsForUser(groups, u.ID),
		})
	}

	h.HTML(w, http.StatusOK, "useradmin/listusers", data)
}

func (h *Handler) updateUsers(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if err := r.ParseForm(); err != nil {
		log.Printf("[WARNING] Invalid form data in bulkUpdate: %v", err)
		h.HTML(w, http.StatusBadRequest, "error", "Bad request.")
		return
	}
	users, err := intSlice(r.PostForm["user"])
	if err != nil {
		log.Printf("[WARNING] Invalid form data in bulkUpdate: %v", err)
		h.HTML(w, http.StatusBadRequest, "error", "Bad request.")
		return
	}

	var alog actionLog
	switch r.FormValue("action") {
	case "block":
		alog = h.blockUsers(users)
	case "unblock":
		alog = h.unblockUsers(users)
	case "addgroup":
		alog = h.addGroup(users, r.FormValue("param"))
	case "removegroup":
		alog = h.removeGroup(users, r.FormValue("param"))
	default:
		h.HTML(w, http.StatusNotImplemented, "error", "Unsupported action.")
		return
	}
	alog.RefURL = r.Header.Get("Referer")
	h.HTML(w, http.StatusOK, "useradmin/actionlog", alog)
}

func (h *Handler) blockUsers(users []int) actionLog {
	alog := actionLog{
		Title: "Blocking users",
	}

	for _, uid := range users {
		user, _, err := h.Gitlab.Users.GetUser(uid, gitlab.GetUsersOptions{})
		if err != nil {
			alog.addf("user id %d", uid).errorf("getting user from gitlab: %v", sanitize(err))
			continue
		}
		le := alog.addf("user %s (id %d)", user.Username, uid)
		if user.IsAdmin {
			le.errorf("blocking gitlab admin account not allowed")
			continue
		}
		if user.State == "blocked" {
			le.errorf("account was already blocked")
			continue
		}
		le.logf("account state was previously %q", user.State)
		if err := h.Gitlab.Users.BlockUser(uid); err != nil {
			le.errorf("blocking gitlab account failed: %v", sanitize(err))
			continue
		}
		le.logf("gitlab account is now blocked")

		mmUser, _, err := h.Mattermost.GetUserByUsername(user.Username, "")
		if err != nil {
			le.errorf("looking up mattermost user: %v", sanitize(err))
			continue
		}
		if _, err := h.Mattermost.UpdateUserActive(mmUser.Id, false); err != nil {
			le.errorf("updating mattermost account: %v", sanitize(err))
			continue
		}
		le.logf("mattermost account is now disabled")
	}
	return alog
}

func (h *Handler) unblockUsers(users []int) actionLog {
	alog := actionLog{
		Title: "Unblocking users",
	}
	for _, uid := range users {
		user, _, err := h.Gitlab.Users.GetUser(uid, gitlab.GetUsersOptions{})
		if err != nil {
			alog.addf("user id %d", uid).errorf("getting user from gitlab: %v", sanitize(err))
			continue
		}
		le := alog.addf("user %s (id %d)", user.Username, uid)
		if user.IsAdmin {
			le.errorf("unblocking gitlab admin account not allowed")
			continue
		}
		if user.State == "active" {
			le.errorf("account was already active")
			continue
		}
		le.logf("account state was previously %q", user.State)
		if err := h.Gitlab.Users.UnblockUser(uid); err != nil {
			le.errorf("unblocking gitlab account failed: %v", sanitize(err))
			continue
		}
		le.logf("gitlab account is now unblocked")

		mmUser, _, err := h.Mattermost.GetUserByUsername(user.Username, "")
		if err != nil {
			le.errorf("looking up mattermost user: %v", sanitize(err))
			continue
		}
		if _, err := h.Mattermost.UpdateUserActive(mmUser.Id, true); err != nil {
			le.errorf("updating mattermost account: %v", sanitize(err))
			continue
		}
		le.logf("mattermost account is now active")
	}
	return alog
}

func (h *Handler) addGroup(users []int, group string) actionLog {
	alog := actionLog{
		Title: "Adding users to group",
	}
	gid := findGroup(h.Config.Groups, group)
	if gid == 0 {
		alog.addf("internal server error").errorf("could not find group %q", group)
		return alog
	}
	for _, uid := range users {
		user, _, err := h.Gitlab.Users.GetUser(uid, gitlab.GetUsersOptions{})
		if err != nil {
			alog.addf("user id %d", uid).errorf("getting user from gitlab: %v", sanitize(err))
			continue
		}
		le := alog.addf("user %s (id %d)", user.Username, uid)
		if user.State != "active" {
			le.errorf("account is blocked, cannot make changes")
			continue
		}
		if _, _, err := h.Gitlab.GroupMembers.AddGroupMember(gid, &gitlab.AddGroupMemberOptions{
			UserID:      gitlab.Int(uid),
			AccessLevel: gitlab.AccessLevel(gitlab.GuestPermissions),
		}); err != nil {
			le.errorf("failed to add group: %v", sanitize(err))
			continue
		}
		le.logf("user added to group %q", group)
	}
	return alog
}

func (h *Handler) removeGroup(users []int, group string) actionLog {
	alog := actionLog{
		Title: "Removing users from group",
	}
	gid := findGroup(h.Config.Groups, group)
	if gid == 0 {
		alog.addf("internal server error").errorf("could not find group %q", group)
		return alog
	}
	for _, uid := range users {
		user, _, err := h.Gitlab.Users.GetUser(uid, gitlab.GetUsersOptions{})
		if err != nil {
			alog.addf("user id %d", uid).errorf("getting user from gitlab: %v", sanitize(err))
			continue
		}
		le := alog.addf("user %s (id %d)", user.Username, uid)
		if user.State != "active" {
			le.errorf("account is blocked, cannot make changes")
			continue
		}
		if _, err := h.Gitlab.GroupMembers.RemoveGroupMember(gid, uid); err != nil {
			le.errorf("failed to remove group: %v", sanitize(err))
			continue
		}
		le.logf("user removed from group %q", group)
	}
	return alog
}

func (h *Handler) getGroupMembers(gid int) map[int]bool {
	members, _, err := h.Gitlab.Groups.ListGroupMembers(gid, &gitlab.ListGroupMembersOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: 1000,
		},
	})
	if err != nil {
		log.Printf("[WARNING] Could not fetch group %d members: %v", gid, err)
	}
	res := map[int]bool{}
	for _, m := range members {
		res[m.ID] = true
	}
	return res
}

func groupsForUser(groups map[string]map[int]bool, uid int) []string {
	var res []string
	for group, mapping := range groups {
		if mapping[uid] {
			res = append(res, group)
		}
	}
	sort.Strings(res)
	return res
}

func groupClass(groups []Group) map[string]string {
	res := map[string]string{}
	for _, group := range groups {
		res[group.Name] = group.TagClass
	}
	return res
}

func findGroup(groups []Group, group string) int {
	for _, g := range groups {
		if group == g.Name {
			return g.GitlabID
		}
	}
	return 0
}

func intValue(r *http.Request, name string, def int) int {
	str := r.FormValue(name)
	if str == "" {
		return def
	}
	if val, err := strconv.Atoi(str); err == nil {
		return val
	}
	return def
}

func intSlice(ss []string) ([]int, error) {
	res := make([]int, len(ss))
	for i, s := range ss {
		v, err := strconv.Atoi(s)
		if err != nil {
			return nil, err
		}
		res[i] = v
	}
	return res, nil
}

func sanitize(err error) string {
	var res string
	if errResp, ok := err.(*gitlab.ErrorResponse); ok {
		res = errResp.Message
	}
	// TODO(quad404): sanitize mattermost errors.
	if res == "" {
		res = err.Error()
	}
	return res
}
