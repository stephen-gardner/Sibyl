package main

import (
	"context"
	"net/url"
	"strconv"

	"github.com/stephen-gardner/intra"
)

func isClosed(ctx context.Context, team *intra.Team) (bool, intra.UserCloses, error) {
	res := intra.UserCloses{}
	for _, user := range team.Users {
		userCloses := intra.UserCloses{}
		params := url.Values{}
		params.Set("filter[user_id]", strconv.Itoa(user.ID))
		params.Set("filter[state]", "close")
		if err := userCloses.GetAllCloses(ctx, false, params); err != nil {
			return false, nil, err
		}
		if len(userCloses) == 0 {
			continue
		}
		for _, userClose := range userCloses {
			if userClose.Reason == config.InteractiveCloseReason {
				res = append(res, userClose)
				break
			}
		}
	}
	return len(res) == len(team.Users), res, nil
}

func closeTeam(ctx context.Context, team *intra.Team) error {
	for _, user := range team.Users {
		userClose := intra.UserClose{}
		if _, _, err := userClose.CloseUser(
			ctx,
			false,
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

func uncloseTeam(ctx context.Context, userCloses intra.UserCloses) error {
	var err error
	for _, userClose := range userCloses {
		if _, _, e := userClose.Unclose(ctx, false); e != nil && err == nil {
			err = e
		}
	}
	return err
}
