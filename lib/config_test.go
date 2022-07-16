package janus

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"gitlab.operationuplift.work/operations/development/janus/lib/provisioner"
	"gitlab.operationuplift.work/operations/development/janus/lib/useradmin"
)

func TestLoadConfig(t *testing.T) {
	got, err := LoadConfig("./testdata/test_config.toml")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	want := &Config{
		Provisioner: &provisioner.Config{
			GitlabProject:          "some project",
			GitlabGroup:            "some group",
			MailgunWelcomeTemplate: "some template",
			Rules: []provisioner.Rule{
				{
					Name:     "first test entry",
					Skill:    "some skill",
					Team:     "some team",
					Channels: []string{"channel1", "channel2"},
				},
				{
					Name:     "second test entry",
					Skill:    "another skill",
					Team:     "some team",
					Channels: []string{"channel2", "channel3"},
				},
			},
		},
		UserAdmin: &useradmin.Config{
			Groups: []useradmin.Group{
				{Name: "management", GitlabID: 66},
				{Name: "helpdesk", GitlabID: 67},
			},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Error("Unexpected LoadConfig diff (-want +got):\n", diff)
	}
}
