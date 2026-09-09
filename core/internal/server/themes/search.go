package themes

import (
	"fmt"
	"sync"

	"github.com/AvengeMedia/DankMaterialShell/core/internal/server/models"
	"github.com/AvengeMedia/DankMaterialShell/core/internal/themes"
)

type searchIndex struct {
	list      []themes.Theme
	installed map[string]bool
}

var (
	searchCacheMu sync.Mutex
	searchCache   *searchIndex
)

// InvalidateSearchCache drops the memoized registry list. Callers mutate
// theme state through install/uninstall/update and flush on success.
func InvalidateSearchCache() {
	searchCacheMu.Lock()
	defer searchCacheMu.Unlock()
	searchCache = nil
}

func getSearchIndex() (*searchIndex, error) {
	searchCacheMu.Lock()
	defer searchCacheMu.Unlock()
	if searchCache != nil {
		return searchCache, nil
	}

	registry, err := themes.NewRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to create registry: %w", err)
	}

	themeList, err := registry.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list themes: %w", err)
	}

	manager, err := themes.NewManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create manager: %w", err)
	}

	installed := make(map[string]bool, len(themeList))
	for _, t := range themeList {
		if ok, _ := manager.IsInstalled(t); ok {
			installed[t.ID] = true
		}
	}

	searchCache = &searchIndex{list: themeList, installed: installed}
	return searchCache, nil
}

func HandleSearch(conn *models.Conn, req models.Request) {
	query, ok := models.Get[string](req, "query")
	if !ok {
		models.RespondError(conn, req.ID, "missing or invalid 'query' parameter")
		return
	}

	index, err := getSearchIndex()
	if err != nil {
		models.RespondError(conn, req.ID, err.Error())
		return
	}

	searchResults := themes.FuzzySearch(query, index.list)

	result := make([]ThemeInfo, len(searchResults))
	for i, t := range searchResults {
		result[i] = ThemeInfo{
			ID:          t.ID,
			Name:        t.Name,
			Version:     t.Version,
			Author:      t.Author,
			Description: t.Description,
			Installed:   index.installed[t.ID],
			FirstParty:  isFirstParty(t.Author),
			WCAG:        t.WCAG,
		}
	}

	models.Respond(conn, req.ID, result)
}
