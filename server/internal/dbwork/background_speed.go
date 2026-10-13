package dbwork

import (
	"context"
	"database/sql"
	"encoding/json"
)

type backgroundSpeedKey struct{}

const SpeedGentle = "gentle"
const SpeedNormal = "normal"
const SpeedFast = "fast"

func ValidBackgroundSpeed(speed string) bool {
	return speed == SpeedGentle || speed == SpeedNormal || speed == SpeedFast
}
func WithBackgroundSpeed(ctx context.Context, speed string) context.Context {
	if !ValidBackgroundSpeed(speed) {
		speed = SpeedNormal
	}
	return context.WithValue(ctx, backgroundSpeedKey{}, speed)
}
func BackgroundSpeed(ctx context.Context) string {
	speed, _ := ctx.Value(backgroundSpeedKey{}).(string)
	if !ValidBackgroundSpeed(speed) {
		return processBackgroundSpeed()
	}
	return speed
}

// ReadBackgroundSpeed consumes the shared owner maintenance document. Call once
// per bounded batch, outside a writer transaction.
func ReadBackgroundSpeed(ctx context.Context, db *sql.DB) string {
	var raw string
	if QueryRow(ctx, db, `SELECT body FROM admin_documents WHERE scope='maintenance'`).Scan(&raw) != nil {
		return SpeedNormal
	}
	var settings struct {
		BackgroundSpeed string `json:"backgroundSpeed"`
	}
	if json.Unmarshal([]byte(raw), &settings) != nil || !ValidBackgroundSpeed(settings.BackgroundSpeed) {
		return SpeedNormal
	}
	return settings.BackgroundSpeed
}
func BackgroundConcurrency(ctx context.Context) int {
	switch BackgroundSpeed(ctx) {
	case SpeedGentle:
		return 2
	case SpeedFast:
		return 8
	}
	return 4
}
