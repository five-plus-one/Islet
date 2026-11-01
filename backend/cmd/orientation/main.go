// orientation verifies and optionally rebuilds previews using camera direction.
package main

import (
	"context"
	"flag"
	"fmt"
	"islet/backend/internal/config"
	"islet/backend/internal/database"
	"islet/backend/internal/model"
	"islet/backend/internal/service"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	apply := flag.Bool("apply", false, "Persist corrected previews and enqueue cache refresh")
	owner := flag.Uint64("owner", 0, "Limit verification to one owner; zero includes all owners")
	selected := flag.String("ids", "", "Comma-separated image IDs from preflight")
	flag.Parse()
	cfg := config.Load()
	db, err := database.ConnectMySQL(cfg.DSN())
	if err != nil {
		return fmt.Errorf("database unavailable")
	}
	policies := service.NewPolicyService(db, service.NewSecretBox(cfg.DataDir), func() string { return cfg.AdminBaseURL }, cfg.DataDir)
	images := service.NewImageService(db, policies)
	var items []model.Image
	q := db.Unscoped().Order("id")
	ids := []uint64{}
	if *selected != "" {
		seen := map[uint64]bool{}
		for _, raw := range strings.Split(*selected, ",") {
			id, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
			if err != nil || id == 0 {
				return fmt.Errorf("invalid image selection")
			}
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
		q = q.Where("id IN ?", ids)
	}
	if *owner != 0 {
		q = q.Where("user_id = ?", *owner)
	}
	if err := q.Find(&items).Error; err != nil {
		return err
	}
	if len(ids) > 0 && len(items) != len(ids) {
		return fmt.Errorf("selected images unavailable")
	}
	changed := 0
	for _, item := range items {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		needed, err := images.RepairThumbnailOrientation(ctx, item.ID, *apply)
		cancel()
		if err != nil {
			return err
		}
		if needed {
			changed++
			fmt.Printf("image=%d corrected=%t\n", item.ID, *apply)
		}
	}
	fmt.Printf("verified=%d affected=%d applied=%t\n", len(items), changed, *apply)
	return nil
}
