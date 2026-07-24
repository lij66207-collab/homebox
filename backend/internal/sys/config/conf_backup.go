package config

type BackupConf struct {
	// Enabled turns on the scheduled automatic backup job (zip export of
	// every collection, including attachments).
	Enabled bool `yaml:"enabled"        conf:"default:false"`
	// Hour is the local hour of day (0-23) at which the backup runs.
	Hour int `yaml:"hour"           conf:"default:3"`
	// RetentionDays is how long automatic backup artifacts are kept before
	// the daily purge removes them.
	RetentionDays int `yaml:"retention_days" conf:"default:30"`
}
