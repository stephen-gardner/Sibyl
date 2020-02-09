package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/stephen-gardner/intra"
)

type (
	teamReportUser struct {
		name    string
		login   string
		photo   string
		attempt int
	}
	teamReport struct {
		deliveryID  string
		groupName   string
		leader      string
		projectSlug string
		finalMark   int
		users       []teamReportUser
		repo        struct {
			url        string
			uuid       string
			status     string
			matches    string
			commits    int
			lastUpdate time.Time
		}
		createdAt time.Time
		closedAt  time.Time
	}
	reportQueue struct {
		in  chan *teamReport
		out chan *teamReport
	}
)

// Slack rate limits files.upload to 20 requests/min
var slackThrottle = time.Tick(time.Minute / 20)

func (report *teamReport) loadData(ctx context.Context, deliveryID string, wt *intra.WebTeam) error {
	it := intra.Team{}
	if err := it.GetTeam(ctx, true, wt.ID); err != nil {
		return err
	}
	report.deliveryID = deliveryID
	report.groupName = wt.Name
	report.projectSlug = wt.Project.Slug
	report.finalMark = wt.FinalMark
	for _, user := range wt.Users {
		attempt := 0
		for _, iUser := range it.Users {
			if iUser.Login == user.Login {
				attempt = iUser.Occurrence + 1
				if iUser.Leader {
					report.leader = iUser.Login
				}
				break
			}
		}
		report.users = append(report.users, teamReportUser{
			name:    user.UsualFullName,
			login:   user.Login,
			photo:   user.ImageURL,
			attempt: attempt,
		})
	}
	report.repo.url = wt.RepoURL
	if wt.RepoURL == "" {
		report.repo.url = batmanNotApplicable
	}
	report.repo.uuid = wt.RepoUUID
	report.createdAt = it.CreatedAt
	report.closedAt = it.ClosedAt
	return nil
}

func (report *teamReport) generate() (blocks string, err error) {
	if strings.Contains(report.repo.url, config.CampusDomain) {
		vog := vogConn{}
		if err = vog.connect(); err != nil {
			return
		}
		defer vog.Close()
		repo := vog.getGitRepo(report.repo.url, report.repo.uuid)
		if report.repo.lastUpdate, err = repo.getLastUpdate(); err != nil {
			return
		}
		if report.repo.commits, err = repo.countCommits(); err != nil {
			return
		}
	}
	blocks, err = composeBlocks(report)
	return
}

func (queue *reportQueue) processInput() {
	// Batman doesn't handle concurrent requests so well
	for report := range queue.in {
		status, res, err := runBatman(report.leader, report.projectSlug, report.repo.url)
		if err != nil {
			outputErr(err, false)
		}
		report.repo.status = status
		if res != nil {
			report.repo.matches = res.getFormattedOutput()
		}
		queue.out <- report
	}
}

func (queue *reportQueue) processOutput() {
	slack := getSlack()
	for report := range queue.out {
		<-slackThrottle
		blocks, err := report.generate()
		fmt.Printf("<%s> OUT:\n%s\n", report.deliveryID, blocks)
		if err == nil {
			if err = slack.postBlocks(blocks); err == nil && report.repo.matches != "" {
				err = slack.uploadMatches(report.repo.matches)
			}
		}
		if err != nil {
			outputErr(err, false)
			go func(queue *reportQueue, report *teamReport) {
				queue.out <- report
			}(queue, report)
		}
	}
}
