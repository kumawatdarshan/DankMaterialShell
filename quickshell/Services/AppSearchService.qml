pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.Common
import qs.Services
import "../Common/SearchUtils.js" as SearchUtils

Singleton {
    id: root
    readonly property var log: Log.scoped("AppSearchService")
    property int refCount: 0

    property var applications: []
    property var _cachedCategories: null
    property var _cachedVisibleApps: null
    property var _hiddenAppsSet: new Set()

    // Normalized search index over the visible apps. Each entry keeps the
    // app reference plus pre-folded fields, laid out as
    // [name, genericName, comment, id, ...keywords]. Rebuilt whenever the
    // visible set changes; null means stale.
    property var _searchIndex: null

    function invalidateVisibleApps() {
        _cachedVisibleApps = null;
        _searchIndex = null;
    }

    property var _transformCache: ({})
    property var _cachedDefaultSections: []
    property var _cachedDefaultFlatModel: []
    property bool _defaultCacheValid: false
    property int cacheVersion: 0

    function refreshApplications() {
        applications = DesktopEntries.applications.values;
        _cachedCategories = null;
        invalidateVisibleApps();
        invalidateLauncherCache();
    }

    function invalidateLauncherCache() {
        _transformCache = {};
        _defaultCacheValid = false;
        _cachedDefaultSections = [];
        _cachedDefaultFlatModel = [];
        cacheVersion++;
    }

    function getOrTransformApp(app, transformFn) {
        const id = app.id || app.execString || app.exec || "";
        if (!id)
            return transformFn(app);
        const cached = _transformCache[id];
        if (cached) {
            const currentIcon = app.icon || "";
            const cachedSourceIcon = cached._sourceIcon || "";
            if (currentIcon === cachedSourceIcon)
                return cached;
        }
        const transformed = transformFn(app);
        transformed._sourceIcon = app.icon || "";
        _transformCache[id] = transformed;
        return transformed;
    }

    function getCachedDefaultSections() {
        if (!_defaultCacheValid)
            return null;
        return _cachedDefaultSections;
    }

    function setCachedDefaultSections(sections, flatModel) {
        _cachedDefaultSections = sections.map(function (s) {
            return Object.assign({}, s, {
                items: s.items ? s.items.slice() : []
            });
        });
        _cachedDefaultFlatModel = flatModel.slice();
        _defaultCacheValid = true;
    }

    function isCacheValid() {
        return _defaultCacheValid;
    }

    function _rebuildHiddenSet() {
        _hiddenAppsSet = new Set(SessionData.hiddenApps || []);
        invalidateVisibleApps();
    }

    function isAppHidden(app) {
        if (!app)
            return false;
        const appId = app.id || app.execString || app.exec || "";
        return _hiddenAppsSet.has(appId);
    }

    function getVisibleApplications() {
        if (_cachedVisibleApps === null) {
            const seen = new Set();
            _cachedVisibleApps = applications.filter(app => {
                if (isAppHidden(app))
                    return false;
                const id = app.id;
                if (id && seen.has(id))
                    return false;
                if (id)
                    seen.add(id);
                return true;
            }).map(app => applyAppOverride(app));
        }
        return _cachedVisibleApps.slice();
    }

    function _ensureSearchIndex() {
        if (_searchIndex === null) {
            _searchIndex = SearchUtils.buildNormalizedIndex(getVisibleApplications(), ["name", "genericName", "comment", "id", "keywords"]);
        }
        return _searchIndex;
    }

    Connections {
        target: SessionData
        function onHiddenAppsChanged() {
            root._rebuildHiddenSet();
            root.invalidateLauncherCache();
        }
        function onAppOverridesChanged() {
            root.invalidateVisibleApps();
            root.invalidateLauncherCache();
        }
    }

    Connections {
        target: AppUsageHistoryData
        function onAppUsageRankingChanged() {
            root.invalidateLauncherCache();
        }
    }

    function applyAppOverride(app) {
        if (!app)
            return app;
        const appId = app.id || app.execString || app.exec || "";
        const override = SessionData.getAppOverride(appId);
        if (!override)
            return app;
        return Object.assign({}, app, {
            name: override.name || app.name,
            icon: override.icon || app.icon,
            comment: override.comment || app.comment,
            _override: override
        });
    }

    readonly property string dmsLogoPath: Qt.resolvedUrl("../assets/danklogo2.svg")

    readonly property var builtInPlugins: ({
            "dms_settings": {
                id: "dms_settings",
                name: I18n.tr("Settings", "settings window title"),
                icon: "svg+corner:" + dmsLogoPath + "|settings",
                cornerIcon: "settings",
                comment: "DMS",
                action: "ipc:settings",
                categories: ["Settings", "System"],
                defaultTrigger: "",
                isLauncher: false
            },
            "dms_notepad": {
                id: "dms_notepad",
                name: I18n.tr("Notepad", "Notepad"),
                icon: "svg+corner:" + dmsLogoPath + "|description",
                cornerIcon: "description",
                comment: "DMS",
                action: "ipc:notepad",
                categories: ["Office", "Utility"],
                defaultTrigger: "",
                isLauncher: false
            },
            "dms_sysmon": {
                id: "dms_sysmon",
                name: I18n.tr("System Monitor", "sysmon window title"),
                icon: "svg+corner:" + dmsLogoPath + "|monitor_heart",
                cornerIcon: "monitor_heart",
                comment: "DMS",
                action: "ipc:processlist",
                categories: ["System", "Monitor"],
                defaultTrigger: "",
                isLauncher: false
            },
            "dms_colorpicker": {
                id: "dms_colorpicker",
                name: I18n.tr("Color Picker"),
                icon: "svg+corner:" + dmsLogoPath + "|palette",
                cornerIcon: "palette",
                comment: "DMS",
                action: "ipc:color-picker",
                categories: ["Graphics", "Utility"],
                defaultTrigger: "",
                isLauncher: false
            },
            "dms_power": {
                id: "dms_power",
                name: I18n.tr("Power"),
                cornerIcon: "power_settings_new",
                comment: "DMS",
                defaultTrigger: "pw",
                isLauncher: true,
                viewMode: "list",
                viewModeEnforced: true,
                defaultSectionPriority: 2.3
            },
            "dms_qr_generator": {
                id: "dms_qr_generator",
                name: I18n.tr("QR Generator"),
                icon: "svg+corner:" + dmsLogoPath + "|qr_code",
                cornerIcon: "qr_code",
                comment: "DMS",
                action: "ipc:qr-generator",
                categories: ["Utility"],
                defaultTrigger: "qrg",
                isLauncher: true,
                viewMode: "list",
                viewModeEnforced: true
            },
            "dms_settings_search": {
                id: "dms_settings_search",
                name: I18n.tr("Settings Search"),
                cornerIcon: "search",
                comment: I18n.tr("DMS Settings"),
                defaultTrigger: "?",
                isLauncher: true
            },
            "dms_clipboard_search": {
                id: "dms_clipboard_search",
                name: I18n.tr("Clipboard"),
                cornerIcon: "content_paste",
                comment: "DMS",
                defaultTrigger: "cb",
                isLauncher: true,
                viewMode: "list",
                viewModeEnforced: true
            }
        })

    function getBuiltInPluginTrigger(pluginId) {
        const plugin = builtInPlugins[pluginId];
        if (!plugin)
            return null;
        return SettingsData.getBuiltInPluginSetting(pluginId, "trigger", plugin.defaultTrigger);
    }

    readonly property var coreApps: {
        SettingsData.builtInPluginSettings;
        const apps = [];
        for (const pluginId in builtInPlugins) {
            if (!SettingsData.getBuiltInPluginSetting(pluginId, "enabled", true))
                continue;
            const plugin = builtInPlugins[pluginId];
            if (plugin.isLauncher && !plugin.action)
                continue;
            apps.push({
                name: plugin.name,
                icon: plugin.icon,
                comment: plugin.comment,
                action: plugin.action,
                categories: plugin.categories,
                isCore: true,
                builtInPluginId: pluginId,
                cornerIcon: plugin.cornerIcon
            });
        }
        return apps;
    }

    function getBuiltInLauncherPlugins() {
        const result = {};
        for (const pluginId in builtInPlugins) {
            const plugin = builtInPlugins[pluginId];
            if (!plugin.isLauncher)
                continue;
            if (!SettingsData.getBuiltInPluginSetting(pluginId, "enabled", true))
                continue;
            result[pluginId] = plugin;
        }
        return result;
    }

    function getBuiltInLauncherTriggers() {
        const triggers = {};
        const launchers = getBuiltInLauncherPlugins();
        for (const pluginId in launchers) {
            const trigger = getBuiltInPluginTrigger(pluginId);
            if (trigger && trigger.trim() !== "")
                triggers[trigger] = pluginId;
        }
        return triggers;
    }

    function getBuiltInLauncherPluginsWithEmptyTrigger() {
        const result = [];
        const launchers = getBuiltInLauncherPlugins();
        for (const pluginId in launchers) {
            const trigger = getBuiltInPluginTrigger(pluginId);
            if (!trigger || trigger.trim() === "")
                result.push(pluginId);
        }
        return result;
    }

    readonly property var powerLauncherKeywords: ({
            lock: ["lock"],
            logout: ["logout", "exit", "sign out"],
            suspend: ["suspend", "sleep"],
            hibernate: ["hibernate"],
            reboot: ["reboot", "restart"],
            softreboot: ["soft reboot"],
            poweroff: ["poweroff", "shutdown", "halt"],
            restart: ["restart", "shell", "reload"]
        })

    function getPowerLauncherActions() {
        const ids = ["lock", "logout", "suspend", "hibernate", "reboot", "softreboot", "poweroff", "restart"];
        const customIds = (SettingsData.customPowerButtons || []).map((button, i) => "custom:" + i);
        return ids.filter(a => SessionService.isPowerActionSupported(a)).concat(customIds).map(a => {
            const data = SessionService.getPowerActionData(a);
            return {
                action: a,
                name: data.label,
                icon: data.icon,
                keywords: powerLauncherKeywords[a] || []
            };
        }).filter(a => a.name);
    }

    function getBuiltInLauncherItems(pluginId, query) {
        if (pluginId === "dms_power") {
            const q = (query || "").toString().trim().toLowerCase();
            return getPowerLauncherActions().filter(a => {
                if (!q)
                    return true;
                if (a.name.toLowerCase().includes(q))
                    return true;
                return a.keywords.some(k => k.includes(q));
            }).map(a => ({
                        name: a.name,
                        icon: "material:" + a.icon,
                        comment: I18n.tr("Power"),
                        action: "power:" + a.action,
                        keywords: a.keywords,
                        isBuiltInLauncher: true,
                        builtInPluginId: pluginId
                    }));
        }

        if (pluginId === "dms_clipboard_search") {
            const trimmed = (query || "").toString().trim();
            const entries = ClipboardService.getCachedLauncherSearchEntries(trimmed, 20).slice().sort((a, b) => {
                if (a.pinned !== b.pinned)
                    return b.pinned ? 1 : -1;
                return (b.id || 0) - (a.id || 0);
            });
            return entries.map(entry => ({
                        type: "clipboard",
                        data: entry
                    }));
        }

        if (pluginId === "dms_qr_generator") {
            const text = (query || "").toString().trim();
            return [
                {
                    name: text.length > 0 ? text : I18n.tr("Enter text to encode"),
                    icon: "material:qr_code",
                    comment: I18n.tr("QR Generator"),
                    action: "qr_generate:" + text,
                    isBuiltInLauncher: true,
                    builtInPluginId: pluginId
                }
            ];
        }

        if (pluginId !== "dms_settings_search")
            return [];

        const results = SettingsSearchService.searchForLauncher(query);
        const items = [];
        for (let i = 0; i < results.length; i++) {
            const r = results[i];
            items.push({
                name: r.label,
                type: "setting",
                section: "settings",
                icon: "material:" + r.icon,
                comment: r.description || r.category,
                action: "settings_nav:" + r.tabIndex + ":" + r.section,
                categories: ["Settings"],
                keywords: r.keywords || [],
                source: I18n.tr("Settings", "settings window title"),
                badgeLabel: I18n.tr("Setting"),
                isCore: true,
                isBuiltInLauncher: true,
                builtInPluginId: pluginId
            });
        }
        return items;
    }

    function executeBuiltInLauncherItem(item) {
        if (!item?.action)
            return false;

        const parts = item.action.split(":");
        switch (parts[0]) {
        case "settings_nav":
            {
                const tabIndex = parseInt(parts[1]);
                const section = parts.slice(2).join(":");
                SettingsSearchService.navigateToSection(section);
                PopoutService.openSettingsWithTabIndex(tabIndex);
                return true;
            }
        case "qr_generate":
            PopoutService.showQRGeneratorModal(parts.slice(1).join(":"));
            return true;
        case "power":
            return executePowerLauncherAction(parts.slice(1).join(":"));
        }
        return false;
    }

    function executePowerLauncherAction(action) {
        if (action === "lock") {
            IdleService.lockRequested();
            return true;
        }
        return SessionService.executePowerAction(action);
    }

    function getCoreApps(query) {
        if (!query || query.length === 0)
            return coreApps;
        const lowerQuery = query.toLowerCase();
        return coreApps.filter(app => app.name.toLowerCase().includes(lowerQuery) || app.comment.toLowerCase().includes(lowerQuery));
    }

    function executeCoreApp(app) {
        if (!app?.action)
            return false;

        const parts = app.action.split(":");
        if (parts[0] !== "ipc")
            return false;

        switch (parts[1]) {
        case "settings":
            PopoutService.focusOrToggleSettings();
            return true;
        case "notepad":
            PopoutService.toggleNotepad();
            return true;
        case "processlist":
            PopoutService.toggleProcessListModal();
            return true;
        case "color-picker":
            PopoutService.showColorPicker();
            return true;
        case "qr-generator":
            PopoutService.showQRGeneratorModal();
            return true;
        }
        return false;
    }

    Connections {
        target: DesktopEntries
        function onApplicationsChanged() {
            root.refreshApplications();
        }
    }

    Connections {
        target: SettingsData
        function onBuiltInPluginSettingsChanged() {
            root.invalidateLauncherCache();
        }
        function onLauncherPluginVisibilityChanged() {
            root.invalidateLauncherCache();
        }
    }

    Component.onCompleted: {
        _rebuildHiddenSet();
        refreshApplications();
    }

    // Filter-only gate over the normalized index: admits every visible app
    // with a cheap substring hit on any folded field, unsorted and uncapped.
    // Ranking (including typo-tolerant fuzzy) is Scorer.scoreItems' job in
    // the Controller. Note this drops typo-only matches that share no
    // substring with any field; exact, prefix, boundary, keyword, generic,
    // id, and multi-word queries are unaffected.
    function searchApplications(query) {
        if (!query || query.length === 0)
            return getVisibleApplications();
        if (applications.length === 0)
            return [];

        const queryLower = query.toLowerCase().trim();
        if (queryLower.length === 0)
            return getVisibleApplications();

        const matches = [];
        const index = _ensureSearchIndex();

        for (const entry of index) {
            const fields = entry.folded;
            for (let i = 0; i < fields.length; i++) {
                if (fields[i].indexOf(queryLower) !== -1) {
                    matches.push(entry.item);
                    break;
                }
            }
        }
        return matches;
    }

    function searchAppActions(query, apps) {
        const results = [];
        for (const app of apps) {
            if (!app.actions || app.actions.length === 0)
                continue;
            for (const action of app.actions) {
                const actionName = (action.name || "").toLowerCase();
                if (!actionName)
                    continue;

                let score = 0;
                if (actionName === query) {
                    score = 8000;
                } else if (actionName.startsWith(query)) {
                    score = 4000;
                } else if (actionName.includes(query)) {
                    score = 400;
                }

                if (score > 0) {
                    results.push({
                        app: {
                            name: action.name,
                            icon: action.icon || app.icon,
                            comment: app.name,
                            categories: app.categories || [],
                            isAction: true,
                            parentApp: app,
                            actionData: action
                        },
                        score: score
                    });
                }
            }
        }
        return results;
    }

    readonly property var _categoryMap: ({
            "AudioVideo": I18n.tr("Media"),
            "Audio": I18n.tr("Media"),
            "Video": I18n.tr("Media"),
            "Development": I18n.tr("Development"),
            "TextEditor": I18n.tr("Development"),
            "IDE": I18n.tr("Development"),
            "Education": I18n.tr("Education"),
            "Game": I18n.tr("Games"),
            "Graphics": I18n.tr("Graphics"),
            "Photography": I18n.tr("Graphics"),
            "Network": I18n.tr("Internet"),
            "WebBrowser": I18n.tr("Internet"),
            "Email": I18n.tr("Internet"),
            "Office": I18n.tr("Office"),
            "WordProcessor": I18n.tr("Office"),
            "Spreadsheet": I18n.tr("Office"),
            "Presentation": I18n.tr("Office"),
            "Science": I18n.tr("Science"),
            "Settings": I18n.tr("Settings"),
            "System": I18n.tr("System"),
            "Utility": I18n.tr("Utilities"),
            "Accessories": I18n.tr("Utilities"),
            "FileManager": I18n.tr("Utilities"),
            "TerminalEmulator": I18n.tr("Utilities")
        })

    on_CategoryMapChanged: _cachedCategories = null

    function getCategoriesForApp(app) {
        if (!app?.categories)
            return [];

        const mappedCategories = new Set();
        for (const cat of app.categories) {
            const mapped = _categoryMap[cat];
            if (mapped)
                mappedCategories.add(mapped);
        }
        return Array.from(mappedCategories);
    }

    property var categoryIcons: ({
            "All": "apps",
            "Media": "music_video",
            "Development": "code",
            "Games": "sports_esports",
            "Graphics": "photo_library",
            "Internet": "web",
            "Office": "content_paste",
            "Settings": "settings",
            "System": "host",
            "Utilities": "build"
        })

    function getCategoryIcon(category) {
        // Check if it's a plugin category
        const pluginIcon = getPluginCategoryIcon(category);
        if (pluginIcon) {
            return pluginIcon;
        }
        return categoryIcons[category] || "folder";
    }

    function getAllCategories() {
        if (_cachedCategories)
            return _cachedCategories;

        const categories = new Set([I18n.tr("All")]);
        for (const app of applications) {
            const appCategories = getCategoriesForApp(app);
            appCategories.forEach(cat => categories.add(cat));
        }

        for (const app of coreApps) {
            const appCategories = getCategoriesForApp(app);
            appCategories.forEach(cat => categories.add(cat));
        }

        _cachedCategories = Array.from(categories).sort();
        return _cachedCategories;
    }

    function getAppsInCategory(category) {
        const visibleApps = getVisibleApplications();
        if (category === I18n.tr("All"))
            return visibleApps;

        return visibleApps.filter(app => {
            const appCategories = getCategoriesForApp(app);
            return appCategories.includes(category);
        });
    }

    function getPluginIdForCategory(category) {
        if (typeof PluginService === "undefined")
            return null;

        const launchers = PluginService.getLauncherPlugins();
        for (const pluginId in launchers) {
            if ((launchers[pluginId].name || pluginId) === category)
                return pluginId;
        }
        return null;
    }

    // Plugin launcher support functions
    function getPluginCategories() {
        if (typeof PluginService === "undefined") {
            return [];
        }

        const categories = [];
        const launchers = PluginService.getLauncherPlugins();

        for (const pluginId in launchers) {
            const plugin = launchers[pluginId];
            const categoryName = plugin.name || pluginId;
            categories.push(categoryName);
        }

        return categories;
    }

    function getPluginCategoryIcon(category) {
        const pluginId = getPluginIdForCategory(category);
        if (!pluginId)
            return null;

        return PluginService.getLauncherPlugins()[pluginId].icon || "extension";
    }

    function getPluginItems(category, query) {
        const pluginId = getPluginIdForCategory(category);
        if (!pluginId)
            return [];

        return getPluginItemsForPlugin(pluginId, query);
    }

    function getPluginItemsForPlugin(pluginId, query) {
        if (typeof PluginService === "undefined") {
            return [];
        }

        const instance = PluginService.ensureLauncherInstance(pluginId);
        if (!instance)
            return [];

        try {
            if (typeof instance.getItems === "function")
                return instance.getItems(query || "") || [];
        } catch (e) {
            log.warn("Error getting items from plugin", pluginId, ":", e);
        }

        return [];
    }

    function executePluginItem(item, pluginId) {
        if (typeof PluginService === "undefined")
            return false;

        const instance = PluginService.ensureLauncherInstance(pluginId);
        if (!instance)
            return false;

        try {
            if (typeof instance.executeItem === "function") {
                instance.executeItem(item);
                return true;
            }
        } catch (e) {
            log.warn("Error executing item from plugin", pluginId, ":", e);
        }

        return false;
    }

    function getPluginPasteArgs(pluginId, item) {
        if (typeof PluginService === "undefined")
            return null;

        const instance = PluginService.ensureLauncherInstance(pluginId);
        if (!instance)
            return null;

        if (typeof instance.getPasteArgs === "function")
            return instance.getPasteArgs(item);

        if (typeof instance.getPasteText === "function") {
            const text = instance.getPasteText(item);
            if (text)
                return ["dms", "cl", "copy", text];
        }

        return null;
    }

    function getPluginLauncherCategories(pluginId) {
        if (typeof PluginService === "undefined")
            return [];

        const instance = PluginService.ensureLauncherInstance(pluginId);
        if (!instance)
            return [];

        if (typeof instance.getCategories !== "function")
            return [];

        try {
            return instance.getCategories() || [];
        } catch (e) {
            log.warn("Error getting categories from plugin", pluginId, ":", e);
            return [];
        }
    }

    function setPluginLauncherCategory(pluginId, categoryId) {
        if (typeof PluginService === "undefined")
            return;

        const instance = PluginService.ensureLauncherInstance(pluginId);
        if (!instance)
            return;

        if (typeof instance.setCategory !== "function")
            return;

        try {
            instance.setCategory(categoryId);
        } catch (e) {
            log.warn("Error setting category on plugin", pluginId, ":", e);
        }
    }
}
