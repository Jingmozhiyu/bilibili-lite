package data

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Seed is explicit and idempotent: restarting never resets sold or reserved stock.
func seedCatalogue(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(0x4d534545)).Error; err != nil {
			return err
		}
		for year := 2023; year <= 2026; year++ {
			id := fmt.Sprintf("bw-%d", year)
			var count int64
			if err := tx.Model(&productRow{}).Where("id=?", id).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				continue
			}
			p := productRow{
				ID:           id,
				Name:         fmt.Sprintf("BILIBILI WORLD %d 门票", year),
				Description:  "和热爱见面。动漫、游戏、舞台与同好，一张门票开启 BW 之旅。",
				Year:         int32(year),
				SaleStartsAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			}
			if err := tx.Create(&p).Error; err != nil {
				return err
			}
			for i, name := range []string{"普通票", "IP 联动票", "VIP 票"} {
				ticket := ticketRow{
					ID:         fmt.Sprintf("%s-%d", id, i+1),
					ProductID:  id,
					Name:       name,
					PriceCents: []int64{12800, 32800, 58800}[i],
					Available:  100,
				}
				if err := tx.Create(&ticket).Error; err != nil {
					return err
				}
				if err := event(tx, ticket.ID, "initial:"+ticket.ID, "initial", 100, 0, 0, 0, "demo catalogue"); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
