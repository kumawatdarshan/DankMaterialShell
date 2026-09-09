package plugins

import (
	"fmt"
	"sync"

	"github.com/AvengeMedia/DankMaterialShell/core/internal/plugins"
	"github.com/AvengeMedia/DankMaterialShell/core/internal/server/models"
)

type searchIndex struct {
	list      []plugins.Plugin
	installed map[string]bool
}

var (
	searchCacheMu sync.Mutex
	searchCache   *searchIndex
)

// InvalidateSearchCache drops the memoized registry list. Callers mutate
// plugin state through install/uninstall/update and flush on success.
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

	registry, err := plugins.NewRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to create registry: %w", err)
	}

	pluginList, err := registry.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list plugins: %w", err)
	}

	manager, err := plugins.NewManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create manager: %w", err)
	}

	installed, err := manager.InstalledIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to list installed plugins: %w", err)
	}

	searchCache = &searchIndex{list: pluginList, installed: installed}
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

	searchResults := plugins.FuzzySearch(query, index.list)

	if category := models.GetOr(req, "category", ""); category != "" {
		searchResults = plugins.FilterByCategory(category, searchResults)
	}

	if compositor := models.GetOr(req, "compositor", ""); compositor != "" {
		searchResults = plugins.FilterByCompositor(compositor, searchResults)
	}

	if capability := models.GetOr(req, "capability", ""); capability != "" {
		searchResults = plugins.FilterByCapability(capability, searchResults)
	}

	searchResults = plugins.SortByFirstParty(searchResults)

	result := make([]PluginInfo, len(searchResults))
	for i, p := range searchResults {
		info := pluginInfoFromPlugin(p)
		info.Installed = index.installed[p.ID]
		result[i] = info
	}

	models.Respond(conn, req.ID, result)
}
