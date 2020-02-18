package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"github.com/stephen-gardner/intra"
)

type (
	ErasedExperience struct {
		gorm.Model
		SkillID           int
		ExperiancableID   int
		ExperiancableType string
		Amount            int
		CreationTime      time.Time
		CursusID          int
	}
	TeamRecordUser struct {
		gorm.Model
		UserID         int
		ProjectsUserID int
		CloseID        *int
		ErasedExp      []ErasedExperience
		TeamRecordID   uint
	}
	TeamRecord struct {
		gorm.Model
		TeamID          int
		Users           map[int]*TeamRecordUser `gorm:"-"`
		TeamRecordUsers []TeamRecordUser
		OriginalScore   int
		Cheated         bool
	}
)

var db *gorm.DB

func (rec *TeamRecord) create(team *intra.Team) error {
	rec.TeamID = team.ID
	rec.OriginalScore = team.FinalMark
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(rec).Error; err != nil {
			return err
		}
		for _, user := range team.Users {
			teamUser := TeamRecordUser{
				UserID:         user.ID,
				ProjectsUserID: user.ProjectsUserID,
				TeamRecordID:   rec.ID,
			}
			if err := tx.
				Model(rec).
				Association("TeamRecordUsers").
				Append(teamUser).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (rec *TeamRecord) get(teamID int) error {
	err := db.
		Where("team_id = ?", teamID).
		Preload("TeamRecordUsers").
		Preload("TeamRecordUsers.ErasedExp").
		First(rec).Error
	if err != nil && err == gorm.ErrRecordNotFound {
		team := &intra.Team{ID: teamID}
		if err := team.Get(context.Background(), false); err != nil {
			return err
		}
		err = rec.create(team)
	}
	if err == nil {
		for i := range rec.TeamRecordUsers {
			rec.Users[rec.TeamRecordUsers[i].UserID] = &rec.TeamRecordUsers[i]
		}
	}
	return err
}

func (rec *TeamRecord) addClose(userClose *intra.UserClose) error {
	teamUser := rec.Users[userClose.User.ID]
	closeID := userClose.ID
	err := db.
		Model(teamUser).
		Update("close_id", closeID).Error
	if err == nil {
		teamUser.CloseID = &closeID
	}
	return err
}

func (rec *TeamRecord) removeClose(userClose *intra.UserClose) error {
	teamUser := rec.Users[userClose.User.ID]
	err := db.
		Model(teamUser).
		Update("close_id", nil).Error
	if err == nil {
		teamUser.CloseID = nil
	}
	return err
}

func (rec *TeamRecord) setCheated(cheated bool) error {
	err := db.
		Model(rec).
		Update("cheated", cheated).Error
	if err == nil {
		rec.Cheated = cheated
	}
	return err
}

func (user *TeamRecordUser) addErasedExp(exp *intra.Experience) error {
	erased := ErasedExperience{
		SkillID:           exp.SkillID,
		ExperiancableID:   exp.ExperiancableID,
		ExperiancableType: exp.ExperiancableType,
		Amount:            exp.Amount,
		CreationTime:      exp.CreatedAt,
		CursusID:          exp.CursusID,
	}
	return db.
		Model(user).
		Association("ErasedExp").
		Append(erased).Error
}

func (user *TeamRecordUser) removeErasedExp(exp *intra.Experience) error {
	for _, erased := range user.ErasedExp {
		if erased.ExperiancableID == exp.ExperiancableID {
			return db.
				Model(user).
				Association("ErasedExp").
				Delete(erased).Error
		}
	}
	return nil
}

func openDatabaseConnection() (err error) {
	uri := fmt.Sprintf("%s:%s@(%s)/%s",
		config.Database.User,
		config.Database.Password,
		config.Database.Address,
		config.Database.Name,
	)
	if db, err = gorm.Open("mysql", uri); err == nil {
		db.DB().SetConnMaxLifetime(time.Minute * 15)
		db.DB().SetMaxIdleConns(0)
		err = db.AutoMigrate(
			&ErasedExperience{},
			&TeamRecord{},
			&TeamRecordUser{},
		).Error
	}
	return
}
