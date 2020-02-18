package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/stephen-gardner/intra"
	"net/url"
	"strconv"
	"strings"
)

type (
	Interaction struct {
		Type string `json:"type"`
		Team struct {
			ID     string `json:"id"`
			Domain string `json:"domain"`
		} `json:"team"`
		User struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Name     string `json:"name"`
			TeamID   string `json:"team_id"`
		} `json:"user"`
		APIAppID  string `json:"api_app_id"`
		Container struct {
			Type         string `json:"type"`
			MessageTs    string `json:"message_ts"`
			AttachmentID int    `json:"attachment_id"`
			ChannelID    string `json:"channel_id"`
			IsEphemeral  bool   `json:"is_ephemeral"`
			IsAppUnfurl  bool   `json:"is_app_unfurl"`
		} `json:"container"`
		TriggerID   string `json:"trigger_id"`
		ResponseURL string `json:"response_url"`
		Actions     []struct {
			Type           string `json:"type"`
			BlockID        string `json:"block_id"`
			ActionID       string `json:"action_id"`
			SelectedOption struct {
				Text struct {
					Type  string `json:"type"`
					Text  string `json:"text"`
					Emoji bool   `json:"emoji"`
				} `json:"text"`
				Value string `json:"value"`
			} `json:"selected_option"`
			ActionTs string `json:"action_ts"`
		} `json:"actions"`
	}
	interactQueue chan *Interaction
)

var errTeamUsersLocked = errors.New("team's users are already locked")
var errTeamUsersUnlocked = errors.New("team's users are not currently locked")

func lockTeamUsers(rec *TeamRecord) error {
	locked := 0
	for _, user := range rec.Users {
		if user.CloseID != nil {
			continue
		}
		locked++
		userClose := &intra.UserClose{}
		userClose.User.ID = user.UserID
		userClose.Closer.ID = user.UserID
		userClose.Reason = config.Slack.InteractiveCloseReason
		err := userClose.Create(context.Background(), false, intra.CloseKindOther)
		if err == nil {
			err = rec.addClose(userClose)
		}
		if err != nil {
			return err
		}
	}
	if locked == 0 {
		return errTeamUsersLocked
	}
	return nil
}

func unlockTeamUsers(rec *TeamRecord) error {
	unlocked := 0
	for _, user := range rec.Users {
		if user.CloseID == nil {
			continue
		}
		unlocked++
		userClose := &intra.UserClose{ID: *user.CloseID}
		err := userClose.Get(context.Background(), false)
		if err == nil {
			if err = userClose.Unclose(context.Background(), false); err == nil {
				err = rec.removeClose(userClose)
			}
		}
		if err != nil {
			return err
		}
	}
	if unlocked == 0 {
		return errTeamUsersUnlocked
	}
	return nil
}

func flagCheating(rec *TeamRecord) error {
	for _, user := range rec.Users {
		experiences := intra.Experiences{}
		err := experiences.GetForProjectsUser(
			context.Background(),
			false,
			user.ProjectsUserID,
			nil,
		)
		if err != nil {
			return err
		}
		for i := range experiences {
			exp := &experiences[i]
			if err = exp.Delete(context.Background()); err == nil {
				err = user.addErasedExp(exp)
			}
			if err != nil {
				return err
			}
		}
	}
	team := &intra.Team{ID: rec.TeamID}
	err := team.Get(context.Background(), false)
	if err == nil {
		params := url.Values{}
		params.Set("team[final_mark]", "-42")
		err = team.Patch(context.Background(), false, params)
	}
	return err
}

func clearCheating(rec *TeamRecord) error {
	for _, user := range rec.Users {
		for _, erased := range user.ErasedExperiences {
			exp := &intra.Experience{
				UserID:            user.UserID,
				SkillID:           erased.SkillID,
				ExperiancableID:   erased.ExperiancableID,
				ExperiancableType: erased.ExperiancableType,
				Amount:            erased.Amount,
				CreatedAt:         erased.CreationTime,
				CursusID:          erased.CursusID,
			}
			err := exp.Create(context.Background(), false)
			if err == nil {
				err = user.removeErasedExp(exp)
			}
			if err != nil {
				return err
			}
		}
	}
	team := &intra.Team{ID: rec.TeamID}
	err := team.Get(context.Background(), false)
	if err == nil {
		params := url.Values{}
		params.Set("team[final_mark]", strconv.Itoa(rec.OriginalScore))
		err = team.Patch(context.Background(), false, params)
	}
	return err
}

func (si *Interaction) reportError(err error) error {
	outputErr(err, false)
	msg := "Something went wrong—please try again in a moment."
	return getSlack().postEphemeralMessage(si.Container.MessageTs, si.User.ID, msg)
}

func (si *Interaction) process() error {
	value := strings.Split(si.Actions[0].SelectedOption.Value, ":")
	action := value[0]
	teamID, _ := strconv.Atoi(value[1])
	rec := &TeamRecord{}
	if err := rec.get(teamID); err != nil {
		return err
	}
	switch action {
	case "lock":
		if err := lockTeamUsers(rec); err != nil {
			if err == errTeamUsersLocked {
				msg := "This team's users have already been locked for academic integrity issues."
				return getSlack().postEphemeralMessage(si.Container.MessageTs, si.User.ID, msg)
			}
			return si.reportError(err)
		}
		msg := fmt.Sprintf("<@%s> has locked this team's users.", si.User.ID)
		return getSlack().postMessage(si.Container.MessageTs, "", msg)
	case "unlock":
		if err := unlockTeamUsers(rec); err != nil {
			if err == errTeamUsersUnlocked {
				msg := "This team's users are not currently locked for academic integrity issues."
				return getSlack().postEphemeralMessage(si.Container.MessageTs, si.User.ID, msg)
			}
			return si.reportError(err)
		}
		msg := fmt.Sprintf("<@%s> has unlocked this team's users.", si.User.ID)
		return getSlack().postMessage(si.Container.MessageTs, "", msg)
	case "flag_cheating":
		if rec.Cheated == true {
			msg := "This team has already been flagged for cheating."
			return getSlack().postEphemeralMessage(si.Container.MessageTs, si.User.ID, msg)
		}
		err := flagCheating(rec)
		if err == nil {
			err = rec.setCheated(true)
		}
		if err != nil {
			return si.reportError(err)
		}
		msg := fmt.Sprintf("<@%s> has flagged this team for cheating.", si.User.ID)
		return getSlack().postMessage(si.Container.MessageTs, "", msg)
	case "clear_cheating":
		if rec.Cheated == false {
			msg := "This team is not currently flagged for cheating."
			return getSlack().postEphemeralMessage(si.Container.MessageTs, si.User.ID, msg)
		}
		err := clearCheating(rec)
		if err == nil {
			err = rec.setCheated(false)
		}
		if err != nil {
			return si.reportError(err)
		}
		msg := fmt.Sprintf("<@%s> has cleared this team of cheating and restored their experience.", si.User.ID)
		return getSlack().postMessage(si.Container.MessageTs, "", msg)
	}
	return fmt.Errorf("unsupported action called: %s", action)
}

func (queue interactQueue) processInput() {
	for si := range queue {
		if err := si.process(); err != nil {
			outputErr(err, false)
		}
	}
}
