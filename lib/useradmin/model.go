package useradmin

import (
	"fmt"

	"github.com/xanzy/go-gitlab"
)

type Config struct {
	Groups []Group
}

type Group struct {
	Name     string
	GitlabID int
	TagClass string // see bulma.io/documentation/elements/tag/#colors
}

type pagination struct {
	This     int
	Last     int
	NumAfter int
	Show     int
}

func paginate(resp *gitlab.Response) pagination {
	return pagination{
		This:     resp.CurrentPage,
		Last:     resp.TotalPages,
		NumAfter: resp.TotalPages - resp.CurrentPage,
		Show:     resp.ItemsPerPage,
	}
}

type actionLog struct {
	Title    string
	Entities []*actionLogEntity
	RefURL   string
}

type actionLogEntity struct {
	Name      string
	HasErrors bool
	Log       []actionLogEntry
}

type actionLogEntry struct {
	Type, Log string
}

func (l *actionLog) addf(namefmt string, args ...interface{}) *actionLogEntity {
	res := &actionLogEntity{Name: fmt.Sprintf(namefmt, args...)}
	l.Entities = append(l.Entities, res)
	return res
}

func (e *actionLogEntity) logf(format string, args ...interface{}) {
	e.Log = append(e.Log, actionLogEntry{"info", fmt.Sprintf(format, args...)})
}

func (e *actionLogEntity) errorf(format string, args ...interface{}) {
	e.Log = append(e.Log, actionLogEntry{"error", fmt.Sprintf(format, args...)})
	e.HasErrors = true
}
