package themes

import (
	"encoding/json"
	"testing"

	"github.com/AvengeMedia/DankMaterialShell/core/internal/mocks/net"
	"github.com/AvengeMedia/DankMaterialShell/core/internal/server/models"
	corethemes "github.com/AvengeMedia/DankMaterialShell/core/internal/themes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func seedSearchCache(t *testing.T) {
	t.Helper()
	searchCacheMu.Lock()
	defer searchCacheMu.Unlock()
	searchCache = &searchIndex{
		list: []corethemes.Theme{
			{ID: "nord", Name: "Nord", Description: "Arctic theme", Author: "Arctic"},
		},
		installed: map[string]bool{},
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

	HandleSearch(conn, models.Request{ID: 7, Method: "themes.search", Params: map[string]any{"query": "nord"}})

	var resp models.Response[[]ThemeInfo]
	require.NoError(t, json.Unmarshal(written, &resp))
	assert.Empty(t, resp.Error)
	require.NotNil(t, resp.Result)
	require.Len(t, *resp.Result, 1)
	assert.Equal(t, "nord", (*resp.Result)[0].ID)
	assert.False(t, (*resp.Result)[0].Installed)
}
