package auth

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/markbates/goth"
)

// User stores information about the logged in user.
type User struct {
	ID       int
	Name     string
	Username string
	Groups   []string
}

func (u *User) String() string {
	data, err := json.Marshal(u)
	if err != nil {
		return ""
	}
	return string(data)
}

func parseUser(s string) (*User, error) {
	res := &User{}
	if err := json.Unmarshal([]byte(s), res); err != nil {
		return nil, err
	}
	return res, nil
}

func fromGothUser(u goth.User) (*User, error) {
	uid, err := strconv.Atoi(u.UserID)
	if err != nil {
		return nil, err
	}
	rawGroups, ok := u.RawData["groups"]
	if !ok {
		return nil, errors.New("missing groups key")
	}
	rawGroupSlice, ok := rawGroups.([]interface{})
	if !ok {
		return nil, errors.New("invalid groups value")
	}

	groups := make([]string, len(rawGroupSlice))
	for i, group := range rawGroupSlice {
		s, ok := group.(string)
		if !ok {
			return nil, errors.New("invalid group data")
		}
		groups[i] = s
	}

	return &User{
		ID:       uid,
		Username: u.NickName,
		Name:     u.Name,
		Groups:   groups,
	}, nil
}
