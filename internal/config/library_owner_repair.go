package config

import (
	"database/sql"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

func (c *Config) RepairLibraryOwnersFromDatabases() bool {
	if c == nil || len(c.Libraries) == 0 || len(c.Users) == 0 {
		return false
	}
	changed := false
	for index := range c.Libraries {
		owner, ok := c.repairedLibraryOwnerUsername(c.Libraries[index])
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(c.Libraries[index].OwnerUsername), owner) {
			continue
		}
		c.Libraries[index].OwnerUsername = owner
		changed = true
	}
	return changed
}

func (c *Config) repairedLibraryOwnerUsername(library Library) (string, bool) {
	if c == nil {
		return "", false
	}
	dbPath, err := c.DatabasePathForStorage(library.Path)
	if err != nil {
		return "", false
	}
	if _, err := os.Stat(dbPath); err != nil {
		return "", false
	}
	counts, err := libraryOwnerUsageCounts(dbPath)
	if err != nil || len(counts) == 0 {
		return "", false
	}
	if currentOwnerID, ok := c.userIDForUsername(library.OwnerUsername); ok && counts[currentOwnerID] > 0 {
		if user, _ := FindUser(c, library.OwnerUsername); user != nil {
			return user.Username, false
		}
	}
	candidates := make([]string, 0, 1)
	for index := range c.Users {
		if !UserIsAdmin(&c.Users[index]) {
			continue
		}
		userID := int64(index + 1)
		if counts[userID] <= 0 {
			continue
		}
		candidates = append(candidates, c.Users[index].Username)
	}
	if len(candidates) != 1 {
		return "", false
	}
	return candidates[0], true
}

func (c *Config) userIDForUsername(username string) (int64, bool) {
	user, index := FindUser(c, username)
	if user == nil || index < 0 {
		return 0, false
	}
	return int64(index + 1), true
}

func libraryOwnerUsageCounts(dbPath string) (map[int64]int64, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	counts := map[int64]int64{}
	for _, query := range []string{
		`SELECT uploaded_by, COUNT(*) FROM photos GROUP BY uploaded_by`,
		`SELECT created_by, COUNT(*) FROM albums GROUP BY created_by`,
		`SELECT created_by, COUNT(*) FROM share_links GROUP BY created_by`,
	} {
		if err := scanLibraryOwnerCounts(db, query, counts); err != nil {
			return nil, err
		}
	}
	return counts, nil
}

func scanLibraryOwnerCounts(db *sql.DB, query string, counts map[int64]int64) error {
	rows, err := db.Query(query)
	if err != nil {
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var userID int64
		var count int64
		if err := rows.Scan(&userID, &count); err != nil {
			return err
		}
		if userID <= 0 || count <= 0 {
			continue
		}
		counts[userID] += count
	}
	return rows.Err()
}
