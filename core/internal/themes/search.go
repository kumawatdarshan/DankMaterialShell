package themes

import (
	"github.com/AvengeMedia/DankMaterialShell/core/internal/utils"
)

func FuzzySearch(query string, themes []Theme) []Theme {
	if query == "" {
		return themes
	}

	return utils.Filter(themes, func(t Theme) bool {
		return utils.FoldSubsequence(query, t.Name) ||
			utils.FoldSubsequence(query, t.Description) ||
			utils.FoldSubsequence(query, t.Author)
	})
}

func FindByIDOrName(idOrName string, themes []Theme) *Theme {
	if t, found := utils.Find(themes, func(t Theme) bool { return t.ID == idOrName }); found {
		return &t
	}
	if t, found := utils.Find(themes, func(t Theme) bool { return t.Name == idOrName }); found {
		return &t
	}
	return nil
}
