package core

import (
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/tools/types"
)

// LogQuery returns a new Log select query.
func (app *BaseApp) LogQuery() *dbx.SelectQuery {
	return app.AuxModelQuery(&Log{})
}

// FindLogById finds a single Log entry by its id.
func (app *BaseApp) FindLogById(id string) (*Log, error) {
	model := &Log{}

	err := app.LogQuery().
		AndWhere(dbx.HashExp{"id": id}).
		Limit(1).
		One(model)

	if err != nil {
		return nil, err
	}

	return model, nil
}

// LogsStatsItem defines the total number of logs for a specific time period.
type LogsStatsItem struct {
	Date  types.DateTime `db:"date" json:"date"`
	Total int            `db:"total" json:"total"`
}

// LogsStats returns hourly grouped logs statistics.
//
// Performance note: we GROUP BY the strftime expression *verbatim* (not by the
// `date` alias) so SQLite can match it against the precomputed
// idx_logs_created_hour expression index. Aliased GROUP BY is sometimes
// resolved post-aggregation by the planner and skips the index, which on
// tens-of-millions-of-rows tables means a full table scan instead of an
// index-only count.
func (app *BaseApp) LogsStats(expr dbx.Expression) ([]*LogsStatsItem, error) {
	result := []*LogsStatsItem{}

	const bucketExpr = "strftime('%Y-%m-%d %H:00:00', created)"

	query := app.LogQuery().
		Select("count(id) as total", bucketExpr+" as date").
		GroupBy(bucketExpr)

	if expr != nil {
		query.AndWhere(expr)
	}

	err := query.All(&result)

	return result, err
}

// DeleteOldLogs delete all logs that are created before createdBefore.
//
// For better performance the logs delete is executed as plain SQL statement,
// aka. no delete model hook events will be fired.
func (app *BaseApp) DeleteOldLogs(createdBefore time.Time) error {
	formattedDate := createdBefore.UTC().Format(types.DefaultDateLayout)
	expr := dbx.NewExp("[[created]] <= {:date}", dbx.Params{"date": formattedDate})

	_, err := app.auxNonconcurrentDB.Delete((&Log{}).TableName(), expr).Execute()

	return err
}
