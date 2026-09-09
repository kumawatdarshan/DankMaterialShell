pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import Quickshell.Wayland
import qs.Common
import qs.Services

Singleton {
    id: root
    readonly property var log: Log.scoped("ClipboardService")

    readonly property bool clipboardAvailable: DMSService.isConnected && (DMSService.capabilities.length === 0 || DMSService.capabilities.includes("clipboard"))
    property bool pasteSupported: false
    readonly property bool pasteAvailable: clipboardAvailable && pasteSupported

    readonly property var terminalAppIds: ["kitty", "foot", "footclient", "alacritty", "st", "org.wezfurlong.wezterm", "com.mitchellh.ghostty", "ghostty", "org.kde.konsole", "konsole", "org.gnome.terminal", "gnome-terminal-server", "org.gnome.console", "kgx", "com.gexperts.tilix", "tilix", "terminator", "xfce4-terminal", "lxterminal", "deepin-terminal", "io.elementary.terminal", "rio", "contour", "wayst", "urxvt", "rxvt"]

    property var clipboardEntries: []
    property var unpinnedEntries: []
    property var pinnedEntries: []
    property int pinnedCount: 0
    property int totalCount: -1
    property bool hasMore: false
    property bool isLoading: false
    property int pageSize: 100
    property int _searchSeq: 0
    property int _pinnedSeq: 0
    property var _preserveId: -1
    property bool pendingStateUpdate: false
    property string searchText: ""
    property string activeFilter: "all"
    readonly property bool filterActive: searchText.trim().length > 0 || activeFilter !== "all"
    property int selectedIndex: 0
    property bool keyboardNavigationActive: false
    property int refCount: 0
    property bool _launcherCacheValid: false
    property string _launcherCachedQuery: ""
    property var _launcherCachedEntries: []
    property int _launcherSearchSeq: 0

    signal historyCopied
    signal historyCleared
    signal launcherSearchReady(string query)

    Timer {
        id: pasteTimer
        interval: 200
        repeat: false
        onTriggered: root.sendPasteKeystroke()
    }

    Connections {
        target: DMSService
        function onIsConnectedChanged() {
            root.refreshPasteSupport();
        }
    }

    Component.onCompleted: refreshPasteSupport()

    function refreshPasteSupport() {
        if (!DMSService.isConnected) {
            pasteSupported = false;
            return;
        }
        DMSService.sendRequest("clipboard.pasteSupported", null, function (response) {
            root.pasteSupported = !response.error && response.result && response.result.supported === true;
        });
    }

    function isTerminalFocused() {
        const appId = (ToplevelManager.activeToplevel?.appId ?? "").toLowerCase();
        if (!appId) {
            return false;
        }
        return terminalAppIds.includes(appId) || appId.endsWith("term") || appId.includes("terminal");
    }

    function sendPasteKeystroke() {
        DMSService.sendRequest("clipboard.sendPaste", {
            "shift": isTerminalFocused()
        }, function (response) {
            if (response.error) {
                ToastService.showError(I18n.tr("Paste failed: %1").arg(response.error));
            }
        });
    }

    function updateFilteredModel() {
        _searchSeq++;
        root.isLoading = false;
        root.unpinnedEntries = [];
        root.hasMore = false;
        root.pendingStateUpdate = false;
        root._preserveId = -1;
        searchDebounce.restart();
    }

    Timer {
        id: searchDebounce
        interval: 150
        onTriggered: root.requestPage(true)
    }

    Timer {
        id: stateUpdateDebounceTimer
        interval: 150
        onTriggered: root._flushStateUpdate()
    }

    function updateCombined() {
        clipboardEntries = pinnedEntries.concat(unpinnedEntries);
    }

    function refresh() {
        if (!clipboardAvailable) {
            return;
        }
        searchDebounce.stop();
        requestPinned();
        requestPage(true);
    }

    function requestPage(reset) {
        if (!clipboardAvailable) {
            return;
        }
        if (reset) {
            unpinnedEntries = [];
            hasMore = false;
        }
        _searchSeq++;
        const seq = _searchSeq;
        isLoading = true;
        const params = {
            "limit": pageSize,
            "pinned": false
        };
        const query = searchText.trim();
        if (query.length > 0) {
            params.query = query;
        }
        if (activeFilter !== "all") {
            params.entryType = activeFilter;
        }
        if (!reset && unpinnedEntries.length > 0) {
            params.beforeId = unpinnedEntries[unpinnedEntries.length - 1].id;
        }
        DMSService.sendRequest("clipboard.search", params, function (response) {
            if (seq !== _searchSeq) {
                return;
            }
            root.isLoading = false;
            if (response.error) {
                log.warn("Failed to load clipboard page:", response.error);
                return;
            }
            const result = response.result || {};
            const entries = result.entries || [];
            if (reset) {
                root.unpinnedEntries = entries;
            } else {
                root.unpinnedEntries = unpinnedEntries.concat(entries);
            }
            root.hasMore = result.hasMore === true;
            if (result.totalKnown === true && typeof result.total === "number") {
                root.totalCount = result.total;
            } else {
                root.totalCount = -1;
            }
            root.restorePreservedSelection();
            root.updateCombined();
            if (root.pendingStateUpdate) {
                root.pendingStateUpdate = false;
                root._refreshHead();
            }
        });
    }

    function loadMore() {
        if (isLoading || !hasMore) {
            return;
        }
        requestPage(false);
    }

    function requestPinned() {
        if (!clipboardAvailable) {
            return;
        }
        _pinnedSeq++;
        const seq = _pinnedSeq;
        DMSService.sendRequest("clipboard.getPinnedEntries", null, function (response) {
            if (seq !== _pinnedSeq) {
                return;
            }
            if (response.error) {
                log.warn("Failed to load pinned entries:", response.error);
                return;
            }
            const entries = Array.isArray(response.result) ? response.result : [];
            root.pinnedEntries = entries;
            root.pinnedCount = entries.length;
            root.updateCombined();
        });
    }

    function restorePreservedSelection() {
        if (root._preserveId === -1) {
            return;
        }
        const keepId = root._preserveId;
        root._preserveId = -1;
        for (let i = 0; i < unpinnedEntries.length; i++) {
            if (unpinnedEntries[i].id === keepId) {
                selectedIndex = i;
                return;
            }
        }
    }

    function _refreshHead() {
        const keepId = (selectedIndex >= 0 && selectedIndex < unpinnedEntries.length) ? unpinnedEntries[selectedIndex].id : -1;
        _preserveId = keepId;
        requestPage(true);
    }

    function flushStateUpdate() {
        if (!pendingStateUpdate) {
            return;
        }
        pendingStateUpdate = false;
        stateUpdateDebounceTimer.stop();
        _refreshHead();
    }

    function _flushStateUpdate() {
        pendingStateUpdate = false;
        _refreshHead();
    }

    function requestLauncherSearch(query, limit) {
        if (!clipboardAvailable) {
            return;
        }

        const trimmed = (query || "").toString().trim();
        const maxItems = limit > 0 ? limit : 20;
        if (_launcherCacheValid && _launcherCachedQuery === trimmed) {
            return;
        }

        _launcherSearchSeq++;
        const seq = _launcherSearchSeq;
        DMSService.sendRequest("clipboard.search", {
            "query": trimmed,
            "limit": maxItems
        }, function (response) {
            if (seq !== _launcherSearchSeq) {
                return;
            }
            if (response.error) {
                log.warn("Launcher clipboard search failed:", response.error);
                _launcherCacheValid = true;
                _launcherCachedQuery = trimmed;
                _launcherCachedEntries = [];
                launcherSearchReady(trimmed);
                return;
            }
            const result = response.result || {};
            _launcherCacheValid = true;
            _launcherCachedQuery = trimmed;
            _launcherCachedEntries = result.entries || [];
            launcherSearchReady(trimmed);
        });
    }

    function getCachedLauncherSearchEntries(query, limit) {
        if (!clipboardAvailable) {
            return [];
        }

        const trimmed = (query || "").toString().trim();
        const maxItems = limit > 0 ? limit : 20;
        if (!_launcherCacheValid || _launcherCachedQuery !== trimmed) {
            requestLauncherSearch(trimmed, maxItems);
            return [];
        }
        return _launcherCachedEntries.slice(0, maxItems);
    }

    function invalidateLauncherSearchCache() {
        _launcherCacheValid = false;
        _launcherCachedQuery = "";
        _launcherCachedEntries = [];
        _launcherSearchSeq++;
    }

    function reset() {
        searchText = "";
        selectedIndex = 0;
        keyboardNavigationActive = false;
        searchDebounce.stop();
        stateUpdateDebounceTimer.stop();
        _searchSeq++;
        _pinnedSeq++;
        isLoading = false;
        hasMore = false;
        totalCount = -1;
        pendingStateUpdate = false;
        _preserveId = -1;
        pinnedEntries = [];
        pinnedCount = 0;
        unpinnedEntries = [];
        clipboardEntries = [];
    }

    function copyEntry(entry, closeCallback, textOnly) {
        const asText = textOnly === true;
        DMSService.sendRequest("clipboard.copyEntry", {
            "id": entry.id,
            "textOnly": asText
        }, function (response) {
            if (response.error) {
                ToastService.showError(I18n.tr("Failed to copy entry"));
                return;
            }
            ToastService.showInfo(entry.isImage && !asText ? I18n.tr("Image copied to clipboard") : I18n.tr("Copied to clipboard"));
            historyCopied();
            if (closeCallback) {
                closeCallback();
            }
        });
    }

    function pasteClipboard(closeCallback) {
        if (closeCallback) {
            closeCallback();
        }
        if (pasteAvailable) {
            pasteTimer.start();
        }
    }

    function pasteEntry(entry, closeCallback) {
        if (!pasteAvailable) {
            copyEntry(entry, closeCallback);
            return;
        }
        DMSService.sendRequest("clipboard.copyEntry", {
            "id": entry.id
        }, function (response) {
            if (response.error) {
                ToastService.showError(I18n.tr("Failed to copy entry"));
                return;
            }
            if (closeCallback) {
                closeCallback();
            }
            pasteTimer.start();
        });
    }

    function pasteSelected(closeCallback) {
        if (!keyboardNavigationActive || clipboardEntries.length === 0 || selectedIndex < 0 || selectedIndex >= clipboardEntries.length) {
            return;
        }
        pasteEntry(clipboardEntries[selectedIndex], closeCallback);
    }

    function deleteEntry(entry) {
        DMSService.sendRequest("clipboard.deleteEntry", {
            "id": entry.id
        }, function (response) {
            if (response.error) {
                log.warn("Failed to delete entry:", response.error);
                return;
            }
            root.requestPage(true);
        });
    }

    function deletePinnedEntry(entry, confirmDialog) {
        if (!confirmDialog) {
            return;
        }
        confirmDialog.show(I18n.tr("Delete Saved Item?"), I18n.tr("This will permanently remove this saved clipboard item. This action cannot be undone."), function () {
            DMSService.sendRequest("clipboard.deleteEntry", {
                "id": entry.id
            }, function (response) {
                if (response.error) {
                    log.warn("Failed to delete entry:", response.error);
                    return;
                }
                root.requestPinned();
                ToastService.showInfo(I18n.tr("Saved item deleted"));
            });
        }, function () {});
    }

    function pinEntry(entry) {
        DMSService.sendRequest("clipboard.getPinnedCount", null, function (countResponse) {
            if (countResponse.error) {
                ToastService.showError(I18n.tr("Failed to check pin limit"));
                return;
            }

            const maxPinned = 25;
            if (countResponse.result.count >= maxPinned) {
                ToastService.showError(I18n.tr("Maximum pinned entries reached") + " (" + maxPinned + ")");
                return;
            }

            DMSService.sendRequest("clipboard.pinEntry", {
                "id": entry.id
            }, function (response) {
                if (response.error) {
                    ToastService.showError(I18n.tr("Failed to pin entry"));
                    return;
                }
                ToastService.showInfo(I18n.tr("Entry pinned"));
                refresh();
            });
        });
    }

    function unpinEntry(entry) {
        DMSService.sendRequest("clipboard.unpinEntry", {
            "id": entry.id
        }, function (response) {
            if (response.error) {
                ToastService.showError(I18n.tr("Failed to unpin entry"));
                return;
            }
            ToastService.showInfo(I18n.tr("Entry unpinned"));
            refresh();
        });
    }

    function clearAll() {
        const hasPinned = pinnedCount > 0;
        const savedCount = pinnedCount;
        DMSService.sendRequest("clipboard.clearHistory", null, function (response) {
            if (response.error) {
                log.warn("Failed to clear history:", response.error);
                return;
            }
            refresh();
            historyCleared();
            if (hasPinned) {
                ToastService.showInfo(I18n.tr("History cleared. %1 pinned entries kept.").arg(savedCount));
            }
        });
    }

    function clearFiltered() {
        const params = {};
        const query = searchText.trim();
        if (query.length > 0) {
            params.query = query;
        }
        if (activeFilter !== "all") {
            params.entryType = activeFilter;
        }
        DMSService.sendRequest("clipboard.deleteMatching", params, function (response) {
            if (response.error) {
                log.warn("Failed to clear filtered entries:", response.error);
                return;
            }
            const deleted = response.result?.deleted ?? 0;
            root.requestPage(true);
            root.historyCleared();
            ToastService.showInfo(I18n.tr("Deleted %1 items").arg(deleted));
        });
    }

    function getEntryPreview(entry) {
        return entry.preview || "";
    }

    function getPinnedEntryByHash(entryHash) {
        if (!entryHash) {
            return null;
        }
        return clipboardEntries.find(entry => entry.pinned && entry.hash === entryHash) || null;
    }

    function hashedPinnedEntry(entryHash) {
        return getPinnedEntryByHash(entryHash) !== null;
    }

    onClipboardAvailableChanged: {
        if (!clipboardAvailable || refCount <= 0)
            return;
        refresh();
    }

    Connections {
        target: DMSService
        enabled: root.refCount > 0
        function onClipboardStateUpdate(data) {
            if (root.isLoading || root.filterActive) {
                root.pendingStateUpdate = true;
                return;
            }
            root.pendingStateUpdate = false;
            stateUpdateDebounceTimer.restart();
        }
    }
}
