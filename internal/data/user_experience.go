package data

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var shanghaiTime = time.FixedZone("Asia/Shanghai", 8*60*60)

// grantDailyExperience serializes awards per user and caps one source within the current Shanghai calendar day.
func grantDailyExperience(tx *gorm.DB, userID uint64, source string, requested, dailyLimit int32, now time.Time) (int64, error) {
	var user userPO
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
		return 0, err
	}
	if requested <= 0 || dailyLimit <= 0 {
		return user.Experience, nil
	}

	date := shanghaiExperienceDate(now)
	var daily userDailyExperiencePO
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND experience_date = ? AND source = ?", userID, date, source).
		First(&daily).Error
	current := int32(0)
	switch {
	case err == nil:
		current = daily.Amount
	case errors.Is(err, gorm.ErrRecordNotFound):
		daily = userDailyExperiencePO{UserID: userID, ExperienceDate: date, Source: source}
	default:
		return 0, err
	}

	award := availableExperienceAward(current, requested, dailyLimit)
	if award <= 0 {
		return user.Experience, nil
	}
	if daily.ID == 0 {
		daily.Amount = award
		if err := tx.Create(&daily).Error; err != nil {
			return 0, err
		}
	} else if err := tx.Model(&daily).UpdateColumn("amount", gorm.Expr("amount + ?", award)).Error; err != nil {
		return 0, err
	}
	if err := tx.Model(&user).UpdateColumn("experience", gorm.Expr("experience + ?", award)).Error; err != nil {
		return 0, err
	}
	return user.Experience + int64(award), nil
}

func shanghaiExperienceDate(now time.Time) string {
	return now.In(shanghaiTime).Format("2006-01-02")
}

func availableExperienceAward(current, requested, dailyLimit int32) int32 {
	if requested <= 0 || dailyLimit <= current {
		return 0
	}
	return min(requested, dailyLimit-current)
}
