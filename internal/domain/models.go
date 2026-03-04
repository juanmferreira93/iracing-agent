package domain

import "time"

type UploadBundle struct {
	Session Session  `json:"session"`
	Laps    []Lap    `json:"laps"`
	Samples []Sample `json:"samples"`
}

type Session struct {
	ExternalID string    `json:"external_id"`
	SourceFile string    `json:"source_file"`
	FileHash   string    `json:"file_hash"`
	FileSize   int64     `json:"file_size"`
	CapturedAt time.Time `json:"captured_at"`
	Track      string    `json:"track"`
	Car        string    `json:"car"`
	DurationS  int       `json:"duration_s"`
}

type Lap struct {
	Number        int     `json:"number"`
	LapTimeS      float64 `json:"lap_time_s"`
	AverageSpeedK float64 `json:"average_speed_kmh"`
}

type Sample struct {
	TimestampS float64 `json:"timestamp_s"`
	SpeedK     float64 `json:"speed_kmh"`
	RPM        float64 `json:"rpm"`
	Throttle   float64 `json:"throttle"`
	Brake      float64 `json:"brake"`
	Gear       int     `json:"gear"`
}
