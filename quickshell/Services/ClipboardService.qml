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

    readonly property int longTextThreshold: 200

    readonly property bool clipboardAvailable: DMSService.isConnected && (DMSService.capabilities.length === 0 || DMSService.capabilities.includes("clipboard"))
    property bool pasteSupported: false
    readonly property bool pasteAvailable: clipboardAvailable && pasteSupported

    readonly property var terminalAppIds: ["kitty", "foot", "footclient", "alacritty", "st", "org.wezfurlong.wezterm", "com.mitchellh.ghostty", "ghostty", "org.kde.konsole", "konsole", "org.gnome.terminal", "gnome-terminal-server", "org.gnome.console", "kgx", "com.gexperts.tilix", "tilix", "terminator", "xfce4-terminal", "lxterminal", "deepin-terminal", "io.elementary.terminal", "rio", "contour", "wayst", "urxvt", "rxvt"]

    property var clipboardEntries: []
    property var unpinnedEntries: []
    property var pinnedEntries: []
    property var rawPinnedEntries: []
    property int pinnedCount: 0
    // Server-side total for the current recents query. unpinnedEntries holds
    // only the loaded page(s); use this for counts.
    property int totalCount: 0
    property bool isLoading: false
    property bool hasMore: false
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
    property int _searchSeq: 0
    readonly property int pageSize: 100

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

    Timer {
        id: searchDebounce
        interval: 150
        onTriggered: root.requestPage(true)
    }

    function updatePinnedFiltered() {
        const query = searchText.trim().toLowerCase();
        const filterAll = activeFilter === "all";
        pinnedEntries = rawPinnedEntries.filter(entry => {
            if (!filterAll && getEntryType(entry) !== activeFilter) {
                return false;
            }
            if (query.length > 0 && !(entry.preview || "").toLowerCase().includes(query)) {
                return false;
            }
            return true;
        });
        clipboardEntries = pinnedEntries.concat(unpinnedEntries);
    }

    // Immediate client-side filter for pinned items + debounced server paging
    // for history items.
    function updateFilteredModel() {
        if (!clipboardAvailable) {
            return;
        }
        _searchSeq++;
        updatePinnedFiltered();
        searchDebounce.restart();
    }

    function requestPage(reset) {
        if (!clipboardAvailable) {
            return;
        }
        _searchSeq++;
        const seq = _searchSeq;
        if (reset) {
            selectedIndex = 0;
        }
        isLoading = true;
        const params = {
            "query": searchText.trim(),
            "limit": pageSize,
            "pinned": false
        };
        if (!reset && unpinnedEntries.length > 0) {
            params.beforeId = unpinnedEntries[unpinnedEntries.length - 1].id;
        } else {
            params.offset = 0;
        }
        if (activeFilter !== "all") {
            params.entryType = activeFilter;
        }
        DMSService.sendRequest("clipboard.search", params, function (response) {
            if (seq !== root._searchSeq) {
                return;
            }
            root.isLoading = false;
            if (response.error) {
                log.warn("Clipboard search failed:", response.error);
                return;
            }
            const result = response.result || {};
            const entries = result.entries || [];
            if (reset) {
                root.unpinnedEntries = entries;
                root.totalCount = result.total || 0;
                root.selectedIndex = 0;
                root.keyboardNavigationActive = entries.length > 0;
            } else {
                root.unpinnedEntries = root.unpinnedEntries.concat(entries);
                if (root.selectedIndex >= root.unpinnedEntries.length) {
                    root.selectedIndex = Math.max(0, root.unpinnedEntries.length - 1);
                }
            }
            root.hasMore = result.hasMore === true;
            root.clipboardEntries = root.pinnedEntries.concat(root.unpinnedEntries);
        });
    }

    function loadMore() {
        if (isLoading || !hasMore) {
            return;
        }
        requestPage(false);
    }

    function refreshPinned() {
        if (!clipboardAvailable) {
            return;
        }
        DMSService.sendRequest("clipboard.getPinnedEntries", null, function (response) {
            if (response.error) {
                log.warn("Failed to get pinned entries:", response.error);
                return;
            }
            const entries = response.result || [];
            root.rawPinnedEntries = entries;
            root.pinnedCount = entries.length;
            root.updatePinnedFiltered();
        });
    }

    function refresh() {
        if (!clipboardAvailable) {
            return;
        }
        searchDebounce.stop();
        refreshPinned();
        requestPage(true);
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
        searchDebounce.stop();
        _searchSeq++;
        searchText = "";
        selectedIndex = 0;
        keyboardNavigationActive = false;
        unpinnedEntries = [];
        clipboardEntries = [];
        pinnedEntries = [];
        rawPinnedEntries = [];
        totalCount = 0;
        hasMore = false;
        isLoading = false;
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
        if (!keyboardNavigationActive || selectedIndex < 0) {
            return;
        }
        const entries = unpinnedEntries.length > 0 ? unpinnedEntries : pinnedEntries;
        if (selectedIndex >= entries.length) {
            return;
        }
        pasteEntry(entries[selectedIndex], closeCallback);
    }

    function removeFromPage(entry) {
        unpinnedEntries = unpinnedEntries.filter(e => e.id !== entry.id);
        totalCount = Math.max(0, totalCount - 1);
        clipboardEntries = pinnedEntries.concat(unpinnedEntries);
        if (unpinnedEntries.length === 0) {
            keyboardNavigationActive = false;
            selectedIndex = 0;
        } else if (selectedIndex >= unpinnedEntries.length) {
            selectedIndex = unpinnedEntries.length - 1;
        }
        if (hasMore && !isLoading && unpinnedEntries.length < pageSize) {
            loadMore();
        }
    }

    function deleteEntry(entry) {
        DMSService.sendRequest("clipboard.deleteEntry", {
            "id": entry.id
        }, function (response) {
            if (response.error) {
                log.warn("Failed to delete entry:", response.error);
                return;
            }
            root.removeFromPage(entry);
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
                root.rawPinnedEntries = root.rawPinnedEntries.filter(e => e.id !== entry.id);
                root.pinnedCount = root.rawPinnedEntries.length;
                root.updatePinnedFiltered();
                if (root.pinnedEntries.length === 0) {
                    root.selectedIndex = 0;
                } else if (root.selectedIndex >= root.pinnedEntries.length) {
                    root.selectedIndex = root.pinnedEntries.length - 1;
                }
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
        if (totalCount === 0) {
            return
        }
        const params = {
            "query": searchText.trim()
        }
        if (activeFilter !== "all") {
            params.entryType = activeFilter
        }
        DMSService.sendRequest("clipboard.deleteMatching", params, function (response) {
            if (response.error) {
                log.warn("Failed to clear filtered entries:", response.error);
                return;
            }
            root.refresh();
            root.historyCleared();
        });
    }

    function getEntryPreview(entry) {
        return entry.preview || "";
    }

    function getEntryType(entry) {
        if (entry.isImage) {
            return "image";
        }
        if (entry.size > longTextThreshold) {
            return "long_text";
        }
        return "text";
    }

    function getPinnedEntryByHash(entryHash) {
        if (!entryHash) {
            return null;
        }
        return rawPinnedEntries.find(e => e.hash === entryHash) || null;
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
            if (!data) {
                return
            }
            if (typeof data.pinnedCount === "number" && data.pinnedCount !== root.pinnedCount) {
                root.pinnedCount = data.pinnedCount;
                root.refreshPinned();
            }
            if (!root.filterActive && typeof data.totalCount === "number") {
                root.totalCount = data.totalCount;
            }
            // Background changes only refresh the head page of an
            // unfiltered view. Filtered views re-request on demand so
            // typing never competes with incoming updates.
            if (root.isLoading || root.filterActive || root.selectedIndex > 0 || root.unpinnedEntries.length > root.pageSize) {
                return;
            }
            root.requestPage(true);
        }
    }
}
