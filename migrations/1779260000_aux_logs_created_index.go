package migrations

import (
	"github.com/pocketbase/pocketbase/core"
)

// Adds a regular B-tree index on _logs.created so that the admin UI's
// time-window filters (WHERE created >= ?) and default sort (-created)
// don't full-scan the table when log volume reaches the tens of millions.
//
// Background: the table already has idx_logs_created_hour, but that's an
// *expression* index over strftime('%Y-%m-%d %H:00:00', created) — SQLite's
// planner uses it only for queries that match that exact expression, NOT for
// range comparisons on the raw `created` column. Without an index on
// created itself, both LogsStats and the paginated /api/logs list end up
// scanning the entire table.
//
// One index, no schema change, no behaviour change — old queries get faster,
// new queries that already work keep working.
func init() {
	core.SystemMigrations.Register(
		func(txApp core.App) error {
			_, err := txApp.AuxDB().NewQuery(`
				CREATE INDEX IF NOT EXISTS idx_logs_created on {{_logs}} ([[created]]);
			`).Execute()
			return err
		},
		func(txApp core.App) error {
			_, err := txApp.AuxDB().NewQuery(`
				DROP INDEX IF EXISTS idx_logs_created;
			`).Execute()
			return err
		},
	)
}
