package collector

import (
	"log"
	"path/filepath"

	"github.com/fkasasagi/ccfx/model"
)

func Collect(claudeDir string, verbose bool) (*model.RawData, error) {
	raw := &model.RawData{SourcePath: claudeDir}

	hist, err := parseHistory(filepath.Join(claudeDir, "history.jsonl"))
	if err != nil && verbose {
		log.Printf("history: %v", err)
	}
	raw.HistoryEntries = hist

	sessions, err := parseSessions(filepath.Join(claudeDir, "sessions"))
	if err != nil && verbose {
		log.Printf("sessions: %v", err)
	}
	raw.SessionFiles = sessions

	transcripts, err := parseAllTranscripts(filepath.Join(claudeDir, "projects"), verbose)
	if err != nil && verbose {
		log.Printf("transcripts: %v", err)
	}
	raw.Transcripts = transcripts

	backup, err := parseLatestBackup(filepath.Join(claudeDir, "backups"))
	if err != nil && verbose {
		log.Printf("backups: %v", err)
	}
	raw.BackupData = backup

	global, err := parseSettings(filepath.Join(claudeDir, "settings.json"))
	if err != nil && verbose {
		log.Printf("global settings: %v", err)
	}
	raw.GlobalSettings = global

	local, err := parseSettings(filepath.Join(claudeDir, "settings.local.json"))
	if err != nil && verbose {
		log.Printf("local settings: %v", err)
	}
	raw.LocalSettings = local

	cred := detectCredentials(filepath.Join(claudeDir, ".credentials.json"))
	raw.CredentialInfo = cred

	fh := scanFileHistory(filepath.Join(claudeDir, "file-history"))
	raw.FileHistStats = fh

	misc := scanMisc(claudeDir)
	raw.MiscStats = misc

	return raw, nil
}
