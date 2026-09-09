package plugins

import (
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFuzzySearch(t *testing.T) {
	list := []Plugin{
		{ID: "firefox", Name: "Firefox", Category: "Network", Description: "Web browser", Author: "Mozilla"},
		{ID: "calc", Name: "Calculator", Category: "Utility", Description: "Do math", Author: "Someone"},
	}

	assert.Len(t, FuzzySearch("", list), 2)
	assert.Equal(t, "firefox", FuzzySearch("fire", list)[0].ID)
	assert.Equal(t, "calc", FuzzySearch("CALC", list)[0].ID)
	assert.Equal(t, "firefox", FuzzySearch("moz", list)[0].ID)
	assert.Empty(t, FuzzySearch("zzz-no-match", list))
}

func TestFuzzySearch_Multibyte(t *testing.T) {
	list := []Plugin{
		{ID: "cafe", Name: "Café Notes", Category: "Office", Description: "Take notes", Author: "José"},
		{ID: "other", Name: "Plain", Category: "Utility", Description: "Nothing", Author: "Anon"},
	}

	res := FuzzySearch("café", list)
	require.Len(t, res, 1)
	assert.Equal(t, "cafe", res[0].ID)

	res = FuzzySearch("JOSÉ", list)
	require.Len(t, res, 1)
	assert.Equal(t, "cafe", res[0].ID)
}

func TestInstalledIDs(t *testing.T) {
	manager, fs, pluginsDir := setupTestManager(t)

	require.NoError(t, fs.MkdirAll(filepath.Join(pluginsDir, "Plugin1"), 0o755))
	require.NoError(t, afero.WriteFile(fs, filepath.Join(pluginsDir, "Plugin1", "plugin.json"), []byte(`{"id":"Plugin1"}`), 0o644))

	require.NoError(t, fs.MkdirAll(filepath.Join(pluginsDir, "renamed-dir"), 0o755))
	require.NoError(t, afero.WriteFile(fs, filepath.Join(pluginsDir, "renamed-dir", "plugin.json"), []byte(`{"id":"real-id"}`), 0o644))

	require.NoError(t, fs.MkdirAll(filepath.Join(pluginsDir, "nodocs"), 0o755))
	require.NoError(t, afero.WriteFile(fs, filepath.Join(pluginsDir, "stray.meta"), []byte(`x`), 0o644))

	ids, err := manager.InstalledIDs()
	require.NoError(t, err)
	assert.True(t, ids["Plugin1"])
	assert.True(t, ids["renamed-dir"])
	assert.True(t, ids["real-id"])
	assert.True(t, ids["nodocs"])
	assert.False(t, ids["stray.meta"])
	assert.False(t, ids["missing"])
}

func TestInstalledIDs_MissingDir(t *testing.T) {
	manager, _, _ := setupTestManager(t)
	ids, err := manager.InstalledIDs()
	require.NoError(t, err)
	assert.Empty(t, ids)
}
