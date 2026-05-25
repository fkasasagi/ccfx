package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fkasasagi/ccfx/model"
)

type rawBackup struct {
	OAuthAccount json.RawMessage            `json:"oauthAccount"`
	Projects     map[string]json.RawMessage `json:"projects"`
}

type rawOAuthAccount struct {
	AccountUUID              string `json:"accountUuid"`
	EmailAddress             string `json:"emailAddress"`
	OrganizationUUID         string `json:"organizationUuid"`
	OrganizationName         string `json:"organizationName"`
	OrganizationType         string `json:"organizationType"`
	OrganizationRole         string `json:"organizationRole"`
	OrganizationRateLimitTier string `json:"organizationRateLimitTier"`
	UserRateLimitTier        string `json:"userRateLimitTier"`
}

type rawBackupProject struct {
	LastCost                         float64                `json:"lastCost"`
	LastTotalInputTokens             int64                  `json:"lastTotalInputTokens"`
	LastTotalOutputTokens            int64                  `json:"lastTotalOutputTokens"`
	LastTotalCacheCreationInputTokens int64                 `json:"lastTotalCacheCreationInputTokens"`
	LastTotalCacheReadInputTokens    int64                  `json:"lastTotalCacheReadInputTokens"`
	LastSessionFirstPrompt           string                 `json:"lastSessionFirstPrompt"`
	LastSessionModified              json.RawMessage        `json:"lastSessionModified"`
}

func parseLatestBackup(backupsDir string) (*model.BackupData, error) {
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		return nil, err
	}

	var backupFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), ".claude.json.backup.") {
			backupFiles = append(backupFiles, filepath.Join(backupsDir, e.Name()))
		}
	}
	if len(backupFiles) == 0 {
		return nil, nil
	}

	sort.Strings(backupFiles)
	latestPath := backupFiles[len(backupFiles)-1]

	data, err := os.ReadFile(latestPath)
	if err != nil {
		return nil, err
	}

	var raw rawBackup
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	result := &model.BackupData{
		Projects: make(map[string]model.BackupProjectInfo),
	}

	if raw.OAuthAccount != nil {
		var oa rawOAuthAccount
		if err := json.Unmarshal(raw.OAuthAccount, &oa); err == nil {
			result.Email = oa.EmailAddress
			result.AccountUUID = oa.AccountUUID
			result.OrganizationUUID = oa.OrganizationUUID
			result.OrganizationName = oa.OrganizationName
			result.OrganizationType = oa.OrganizationType
			result.OrganizationRole = oa.OrganizationRole
			if oa.UserRateLimitTier != "" {
				result.RateLimitTier = oa.UserRateLimitTier
			} else {
				result.RateLimitTier = oa.OrganizationRateLimitTier
			}
		}
	}

	for projPath, projRaw := range raw.Projects {
		var rp rawBackupProject
		if err := json.Unmarshal(projRaw, &rp); err != nil {
			continue
		}
		result.Projects[projPath] = model.BackupProjectInfo{
			Path:    projPath,
			CostUSD: rp.LastCost,
			Tokens: model.TokenSummary{
				InputTokens:         rp.LastTotalInputTokens,
				OutputTokens:        rp.LastTotalOutputTokens,
				CacheCreationTokens: rp.LastTotalCacheCreationInputTokens,
				CacheReadTokens:     rp.LastTotalCacheReadInputTokens,
			},
		}
	}

	return result, nil
}
