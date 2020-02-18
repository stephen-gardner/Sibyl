package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type slack struct {
	token   string
	channel string
}

func getSlackTimestamp(timestamp time.Time) string {
	if timestamp.IsZero() {
		return "N/A"
	}
	return fmt.Sprintf(
		"<!date^%d^{date_short_pretty} at {time_secs}|%s>",
		timestamp.Unix(),
		timestamp.Format(time.RFC822),
	)
}

func getUserBlockElements(report *teamReport) string {
	elements := make([]string, 2*len(report.users))
	for i, user := range report.users {
		// We need this hack because the photo URI in anonymized profiles point to invalid resources
		photo := user.photo
		if strings.Contains(photo, "3b3") {
			photo = config.ListenDomain + "3b3.jpg"
		}
		elements[2*i] = fmt.Sprintf(`{"type":"image","image_url":"%s","alt_text":"%s"}`, photo, user.name)
		text := fmt.Sprintf(
			"<https://projects.intra.42.fr/projects/%s/projects_users/%s|%s> (%d)",
			report.projectSlug,
			user.login,
			user.login,
			user.attempt,
		)
		if user.login == report.leader {
			text = "*" + text + "*"
		}
		elements[(2*i)+1] = fmt.Sprintf(`{"type":"mrkdwn","text":"%s"}`, text)
	}
	return "[" + strings.Join(elements, ",") + "]"
}

// Ugly function, but haven't yet come up with a better alternative
func composeBlocks(report *teamReport) (blocks string, err error) {
	var tmpl *template.Template
	tmpl, err = template.ParseFiles("templates/slack_message.json")
	if err != nil {
		return
	}
	groupName, _ := json.Marshal(&report.name)
	grade := strconv.Itoa(report.finalMark)
	if report.teamCancelled {
		grade += " _(cancelled)_"
	} else if report.passed {
		grade += " _(passed)_"
	} else {
		grade += " _(failed)_"
	}
	var lastUpdate string
	if report.repo.url == batmanNotApplicable {
		lastUpdate = batmanNotApplicable
	} else if report.repo.lastUpdate.IsZero() {
		lastUpdate = "never _(0 commits)_"
	} else {
		lastUpdate = fmt.Sprintf(
			"%s _(%d commits)_",
			getSlackTimestamp(report.repo.lastUpdate.Local()),
			report.repo.commits,
		)
	}
	data := &bytes.Buffer{}
	err = tmpl.Execute(data, struct {
		TeamID       int
		GroupName    string
		UserElements string
		ProjectSlug  string
		Grade        string
		CreatedAt    string
		ClosedAt     string
		CheckResult  string
		RepoURL      string
		LastUpdate   string
		Commits      int
	}{
		TeamID:       report.teamID,
		GroupName:    string(groupName[1 : len(groupName)-1]),
		UserElements: getUserBlockElements(report),
		ProjectSlug:  report.projectSlug,
		Grade:        grade,
		CreatedAt:    getSlackTimestamp(report.createdAt.Local()),
		ClosedAt:     getSlackTimestamp(report.closedAt.Local()),
		CheckResult:  report.repo.status,
		RepoURL:      report.repo.url,
		LastUpdate:   lastUpdate,
		Commits:      report.repo.commits,
	})
	compacted := &bytes.Buffer{}
	err = json.Compact(compacted, data.Bytes())
	blocks = compacted.String()
	return
}

func (slack *slack) postEphemeralMessage(threadTS, userID, msg string) error {
	params := url.Values{}
	params.Set("token", slack.token)
	params.Set("channel", slack.channel)
	if threadTS != "" {
		params.Set("thread_ts", threadTS)
	}
	params.Set("user", userID)
	params.Set("text", msg)
	resp, err := http.PostForm("https://slack.com/api/chat.postEphemeral", params)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

func (slack *slack) postMessage(threadTS, blocks, msg string) error {
	params := url.Values{}
	params.Set("token", slack.token)
	params.Set("channel", slack.channel)
	if threadTS != "" {
		params.Set("thread_ts", threadTS)
	}
	if blocks != "" {
		params.Set("blocks", blocks)
	}
	if msg != "" {
		params.Set("text", msg)
	}
	resp, err := http.PostForm("https://slack.com/api/chat.postMessage", params)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

func (slack *slack) uploadMatches(matches string) error {
	params := url.Values{}
	params.Set("token", slack.token)
	params.Set("channels", slack.channel)
	params.Set("title", "Matches")
	params.Set("content", matches)
	resp, err := http.PostForm("https://slack.com/api/files.upload", params)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

func getSlack() *slack {
	return &slack{
		token:   os.Getenv("SLACK_TOKEN"),
		channel: config.Slack.Channel,
	}
}
