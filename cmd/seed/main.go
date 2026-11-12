// 预置演示数据：门店、服务项目、角色账号、宠物、3 个未来时段（各 3 名额）。幂等可重复执行。
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/zm1464688361-svg/pa/internal/config"
	"github.com/zm1464688361-svg/pa/internal/model"
	"github.com/zm1464688361-svg/pa/internal/repo"
)

func main() {
	dsn := config.LoadDSN()
	db, err := repo.Open(dsn, true)
	if err != nil {
		log.Fatal(err)
	}
	_ = repo.EnsureClockRow(context.Background(), db)

	var n int64
	db.Model(&model.Shop{}).Count(&n)
	if n == 0 {
		db.Create(&model.Shop{ID: 1, Name: "宠序·示例门店"})
	}
	db.Model(&model.ServiceItem{}).Count(&n)
	if n == 0 {
		db.Create(&model.ServiceItem{ShopID: 1, Name: "基础洗护", PriceFen: 9900})
	}

	users := []model.User{
		{Username: "owner", Role: "owner"},
		{Username: "user1", Role: "pet_owner"},
		{Username: "user2", Role: "pet_owner"},
		{Username: "user3", Role: "pet_owner"},
		{Username: "front", Role: "front", ShopID: 1},
		{Username: "support", Role: "support"},
	}
	for i := range users {
		var cnt int64
		db.Model(&model.User{}).Where("username = ?", users[i].Username).Count(&cnt)
		if cnt == 0 {
			db.Create(&users[i])
		}
	}

	pets := []model.Pet{
		{UserID: 2, Name: "豆包", Breed: "柯基"},
		{UserID: 3, Name: "毛毛", Breed: "金毛"},
		{UserID: 4, Name: "年糕", Breed: "布偶"},
	}
	for i := range pets {
		var cnt int64
		db.Model(&model.Pet{}).Where("user_id = ? AND name = ?", pets[i].UserID, pets[i].Name).Count(&cnt)
		if cnt == 0 {
			db.Create(&pets[i])
		}
	}

	// 3 个未来时段：明天 14:00/16:00/18:00，各 3 名额（幂等：按开始时间去重）
	tomorrow := time.Now().AddDate(0, 0, 1)
	day := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, time.Local)
	type slotDef struct{ h int }
	for _, s := range []slotDef{{14}, {16}, {18}} {
		start := time.Date(day.Year(), day.Month(), day.Day(), s.h, 0, 0, 0, time.Local)
		end := start.Add(time.Hour)
		var cnt int64
		db.Model(&model.TimeSlot{}).Where("start_time = ?", start).Count(&cnt)
		if cnt == 0 {
			db.Create(&model.TimeSlot{
				ShopID: 1, ServiceItemID: 1, StartTime: start, EndTime: end,
				Capacity: 3, Remaining: 3, Status: model.SlotOpen,
			})
		}
	}
	fmt.Println("seed 完成：门店/项目/6 账号/3 宠物/3 时段（明天 14:00、16:00、18:00，各 3 名额）")
}
