// Package store is the data layer for the report sub-domain
// (report_schedules + reports). See HLD-014.
package store

import (
	"gorm.io/gorm"

	model "github.com/ongridio/ongrid/internal/manager/model/report"
)

// Migrate registers the report tables with gorm AutoMigrate. AutoMigrate
// adds new columns/indexes but never drops or narrows existing ones, so
// re-running on every boot is safe. Wired into the manager startup
// migration list in cmd/ongrid/main.go.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.ReportSchedule{},
		&model.Report{},
	)
}
