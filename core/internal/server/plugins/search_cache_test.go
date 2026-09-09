package plugins

import (
	"encoding/json"
	"testing"

	"github.com/AvengeMedia/DankMaterialShell/core/internal/mocks/net"
	coreplugins "github.com/AvengeMedia/DankMaterialShell/core/internal/plugins"
	"github.com/AvengeMedia/DankMaterialShell/core/internal/server/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func seedSearchCache(t *testing.T) {
	t.Helper()
	searchCacheMu.Lock()
	defer searchCacheMu.Unlock()
	searchCache = &searchIndex{
		list: []coreplugins.Plugin{
			{ID: "firefox", Name: "Firefox", Category: "Network", Description: "Web browser", Author: "Mozilla"},
		},
		installed: map[string]bool{"firefox": true},
	}
	t.Cleanup(InvalidateSearchCache)
}

func TestInvalidateSearchCache(t *testing.T) {
	seedSearchCache(t)
	InvalidateSearchCache()
	searchCacheMu.Lock()
	defer searchCacheMu.Unlock()
	assert.Nil(t, searchCache)
}

func TestHandleSearch_ServesCache(t *testing.T) {
	seedSearchCache(t)

	mc := net.NewMockConn(t)
	mc.EXPECT().SetWriteDeadline(mock.Anything).Return(nil).Maybe()
	var written []byte
	mc.EXPECT().Write(mock.Anything).RunAndReturn(func(b []byte) (int, error) {
		written = append([]byte(nil), b...)
		return len(b), nil
	}).Maybe()
	conn := models.NewConn(mc)

	HandleSearch(conn, models.Request{ID: 7, Method: "plugins.search", Params: map[string]any{"query": "fire"}})

	var resp models.Response[[]PluginInfo]
	require.NoError(t, json.Unmarshal(written, &resp))
	assert.Empty(t, resp.Error)
	require.NotNil(t, resp.Result)
	require.Len(t, *resp.Result, 1)
	assert.Equal(t, "firefox", (*resp.Result)[0].ID)
	assert.True(t, (*resp.Result)[0].Installed)
}

func TestHandleSearch_MissingQueryPreservesCache(t *testing.T) {
	seedSearchCache(t)

	mc := net.NewMockConn(t)
	mc.EXPECT().SetWriteDeadline(mock.Anything).Return(nil).Maybe()
	mc.EXPECT().Write(mock.Anything).Return(0, nil).Maybe()
	conn := models.NewConn(mc)

	HandleSearch(conn, models.Request{ID: 7, Method: "plugins.search", Params: map[string]any{}})

	searchCacheMu.Lock()
	defer searchCacheMu.Unlock()
	require.NotNil(t, searchCache)
}
