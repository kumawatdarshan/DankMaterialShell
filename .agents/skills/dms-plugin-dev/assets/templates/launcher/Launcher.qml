import QtQuick
import Quickshell
import qs.Services

Item {
    id: root

    property var pluginService: null
    property string trigger: "#"

    signal itemsChanged()

    // TODO: Define your items
    property var allItems: [
        {
            name: "Example Item",
            icon: "material:star",
            comment: "An example launcher item",
            action: "toast:Hello from my launcher!",
            categories: ["MyLauncher"]
        }
    ]

    // Search pattern: fold fields once here, score per keystroke without
    // allocating lowercase copies in the filter loop. Call
    // rebuildSearchIndex() again if allItems changes at runtime. In-tree
    // code should use qs.Common/SearchUtils.js instead of vendoring this;
    // it exports foldAndTokenize/buildNormalizedIndex/score with the same
    // semantics.
    property var _searchIndex: []

    function rebuildSearchIndex() {
        _searchIndex = allItems.map(function (item) {
            return {
                item: item,
                folded: [
                    (item.name || "").toLowerCase(),
                    (item.comment || "").toLowerCase(),
                    (item.keywords || []).map(function (k) { return (k || "").toLowerCase(); }).join(" ")
                ]
            };
        });
    }

    function getItems(query) {
        if (!query || query.length === 0) return allItems

        var q = query.toLowerCase()
        return _searchIndex.filter(function (entry) {
            return entry.folded[0].includes(q) ||
                   entry.folded[1].includes(q) ||
                   entry.folded[2].includes(q)
        }).map(function (entry) { return entry.item; })
    }

    function executeItem(item) {
        var actionParts = item.action.split(":")
        var actionType = actionParts[0]
        var actionData = actionParts.slice(1).join(":")

        switch (actionType) {
            case "toast":
                if (typeof ToastService !== "undefined")
                    ToastService.showInfo(actionData)
                break
            case "copy":
                Quickshell.execDetached(["dms", "cl", "copy", actionData])
                if (typeof ToastService !== "undefined")
                    ToastService.showInfo("Copied to clipboard")
                break
            default:
                console.warn("Unknown action type:", actionType)
        }
    }

    Component.onCompleted: {
        rebuildSearchIndex();
        if (pluginService) {
            trigger = pluginService.loadPluginData("myLauncher", "trigger", "#")
        }
    }
}
