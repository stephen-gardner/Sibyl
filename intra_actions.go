package main

import (
	"context"
	"net/url"
	"strconv"

	"github.com/stephen-gardner/intra"
)

func isClosed(ctx context.Context, team *intra.Team) (bool, error) {
	for _, user := range team.Users {
		userCloses := intra.UserCloses{}
		params := url.Values{}
		params.Set("filter[user_id]", strconv.Itoa(user.ID))
		params.Set("filter[state]", "close")
		if err := userCloses.GetAllCloses(ctx, true, params); err != nil {
			return false, err
		}
		if len(userCloses) == 0 {
			return false, nil
		}
		closed := false
		for _, userClose := range userCloses {
			if userClose.Reason == config.InteractiveCloseReason {
				closed = true
				break
			}
		}
		if !closed {
			return false, nil
		}
	}
	return true, nil
}

func closeTeam(ctx context.Context, team *intra.Team) error {
	for _, user := range team.Users {
		userClose := intra.UserClose{}
		if _, _, err := userClose.CloseUser(
			ctx,
			true,
			user.ID,
			intra.CloseSelf,
			intra.CloseKindOther,
			config.InteractiveCloseReason,
		); err != nil {
			return err
		}
	}
	return nil
}
