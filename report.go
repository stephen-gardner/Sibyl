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
		deliveryID     string
		teamID         int
		batmanAttempts int
		name           string
		leader         string
		projectSlug    string
		finalMark      int
		users          []teamReportUser
		repo           struct {
			url        string
			uuid       string
			status     string
			matches    string
			commits    int
			lastUpdate time.Time
		}
		createdAt     time.Time
		closedAt      time.Time
		teamCancelled bool
		passed        bool
	}
	reportQueue struct {
		in  chan *teamReport
		out chan *teamReport
	}
)

func isCancelledTeam(team *intra.Team, ps *intra.ProjectSession) bool {
	required := 0
	for _, scale := range ps.Scales {
		if scale.Primary {
			required = scale.CorrectionNumber
			break
		}
	}
	return len(team.ScaleTeams) < required
}

func (report *teamReport) loadData(ctx context.Context, deliveryID string, wt *intra.WebTeam) error {
	it := intra.Team{ID: wt.ID}
	if err := it.Get(ctx, false); err != nil {
		return err
	}
	ps := intra.ProjectSession{ID: it.ProjectSessionID}
	if err := ps.Get(ctx, false); err != nil {
		return err
	}
	report.deliveryID = deliveryID
	report.teamID = wt.ID
	cursusName := ps.Cursus.Name
	if ps.Cursus.Name == "" {
		project := intra.Project{ID: wt.Project.ID}
		if err := project.Get(ctx, false); err != nil {
			// We can't get deprecated projects from Intra for some reason
			if !strings.Contains(err.Error(), "exist") {
				return err
			}
		}
		if len(project.Cursus) > 0 {
			cursusName = project.Cursus[0].Name
		} else {
			cursusName = "?"
		}
	}
	report.name = fmt.Sprintf("[%s] _%s_", cursusName, wt.Name)
	report.projectSlug = wt.Project.Slug
	report.finalMark = wt.FinalMark
	report.passed = it.Validated
	report.leader = wt.Leader.Login
	for _, user := range wt.Users {
		for _, iUser := range it.Users {
			if iUser.Login == user.Login {
				report.users = append(report.users, teamReportUser{
					name:    user.UsualFullName,
					login:   user.Login,
					photo:   user.ImageURL,
					attempt: iUser.Occurrence + 1,
				})
				break
			}
		}
	}
	report.repo.url = wt.RepoURL
	if wt.RepoURL == "" {
		report.repo.url = batmanNotApplicable
	}
	report.repo.uuid = wt.RepoUUID
	report.createdAt = it.CreatedAt
	report.closedAt = it.ClosedAt
	report.teamCancelled = isCancelledTeam(&it, &ps)
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
		report.batmanAttempts++
		status, res, err := runBatman(report.leader, report.projectSlug, report.repo.url)
		if err != nil {
			outputErr(err, false)
		}
		if status == batmanError && report.batmanAttempts < config.BatmanMaxAttempts {
			go func(queue *reportQueue, report *teamReport) {
				queue.in <- report
			}(queue, report)
			continue
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
	// Slack rate limits files.upload to 20 requests/min
	slackThrottle := time.Tick(time.Minute / 20)
	for report := range queue.out {
		<-slackThrottle
		blocks, err := report.generate()
		if err == nil {
			if err = slack.postMessage("", blocks, ""); err == nil && report.repo.matches != "" {
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
