package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/template"
	"time"
)

type slack struct {
	token   string
	channel string
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
	lastUpdate := "never _(0 commits)_"
	if report.repo.url == batmanNotApplicable {
		lastUpdate = batmanNotApplicable
	} else if !report.repo.lastUpdate.IsZero() {
		report.repo.lastUpdate.Unix()
		timestamp := fmt.Sprintf("<!date^%d^{date_num} {time_secs}|%s>",
			report.repo.lastUpdate.Unix(),
			report.repo.lastUpdate.Local().Format(time.RFC1123),
		)
		lastUpdate = fmt.Sprintf("%s _(%d commits)_", timestamp, report.repo.commits)
	}
	groupName, _ := json.Marshal(&report.groupName)
	data := &bytes.Buffer{}
	err = tmpl.Execute(data, struct {
		GroupName    string
		UserElements string
		ProjectSlug  string
		Grade        int
		CreatedAt    int64
		CreatedAtAlt string
		ClosedAt     int64
		ClosedAtAlt  string
		CheckResult  string
		RepoURL      string
		LastUpdate   string
		Commits      int
	}{
		GroupName:    string(groupName[1 : len(groupName)-1]),
		UserElements: getUserBlockElements(report),
		ProjectSlug:  report.projectSlug,
		Grade:        report.finalMark,
		CreatedAt:    report.createdAt.Unix(),
		CreatedAtAlt: report.createdAt.Local().Format(time.RFC1123),
		ClosedAt:     report.closedAt.Unix(),
		ClosedAtAlt:  report.closedAt.Local().Format(time.RFC1123),
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

func (slack *slack) postBlocks(blocks string) error {
	params := url.Values{}
	params.Set("token", slack.token)
	params.Set("channel", slack.channel)
	params.Set("blocks", blocks)
	params.Encode()
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
		channel: config.SlackReportChannel,
	}
}
