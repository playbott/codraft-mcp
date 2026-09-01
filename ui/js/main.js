// Main application entry point - wires all modules together
import { t, getLang, setLang, loadLocale, updateStaticI18n, initLanguageSelector } from './i18n.js';
import {
    currentView, setCurrentView,
    currentPlanID, setCurrentPlanID,
    currentFolder, setCurrentFolder,
    allPlans, setAllPlans,
    customFolders, setCustomFolders,
    saveSetting, saveCustomFolders
} from './state.js';
import { connectWS } from './ws.js';
import { renderPlans, initPlansView, setLoadPlansExternal, updatePlansFolder } from './views/plans.js';
import { renderPlanDetail, initPlanDetailView } from './views/plan-detail.js';

// ── DOM References ──
var plansGrid = document.getElementById('plans-grid');
var viewPlans = document.getElementById('view-plans');
var viewPlanDetail = document.getElementById('view-plan-detail');
var planDetailContent = document.getElementById('plan-detail-content');
var folderTabs = document.getElementById('folder-tabs');
var foldersSidebar = document.getElementById('folders-sidebar');
var folderListEl = document.getElementById('folder-list');
var sessionInfo = document.getElementById('session-info');
var wsStatus = document.getElementById('ws-status');
var selectLang = document.getElementById('select-lang');
var sidebarToggleBtn = document.getElementById('btn-toggle-sidebar');

// ── Init Views ──
initPlansView({
    plansGrid: plansGrid,
    folderTabs: folderTabs,
    foldersSidebar: foldersSidebar,
    folderListEl: folderListEl,
    onNavigate: navigateToPlan
});

initPlanDetailView({
    planDetailContent: planDetailContent,
    onBack: function () { navigateToPlans(); },
    loadPlanDetail: loadPlanDetail
});

setLoadPlansExternal(loadPlans);

// ── Session Info ──
function loadSessionInfo() {
    fetch('/api/sessions')
        .then(function (response) {
            if (!response.ok) return;
            return response.json();
        })
        .then(function (data) {
            if (!data || !sessionInfo) return;
            var parts = [];
            if (data.current_project) {
                parts.push(t('projectLabel') + ' <strong>' + data.current_project + '</strong>');
            }
            if (data.port) {
                parts.push(t('portLabel') + ' <strong>' + data.port + '</strong>');
            }
            if (data.project_path) {
                parts.push(t('pathLabel') + ' <code class="footer-path">' + data.project_path + '</code>');
            }
            sessionInfo.innerHTML = parts.join(' &middot; ');
        })
        .catch(function () {});
}

// ── Version Badge ──
// Replaces the static "alpha" placeholder with the version the backend reports,
// so the UI cannot drift from the running build.
function loadVersionBadge() {
    var badge = document.getElementById('app-version');
    if (!badge) return;
    fetch('/api/ping')
        .then(function (response) {
            if (!response.ok) return;
            return response.json();
        })
        .then(function (data) {
            if (!data || !data.version) return;
            badge.textContent = data.version;
            badge.title = data.app + ' ' + data.version;
        })
        .catch(function () {
            // keep the placeholder
        });
}

// ── Plans Loading ──
function loadPlans() {
    fetch('/api/plans')
        .then(function (response) {
            if (!response.ok) throw new Error('HTTP ' + response.status);
            return response.json();
        })
        .then(function (data) {
            setAllPlans(data || []);
            renderPlans(allPlans);
        })
        .catch(function (err) {
            plansGrid.innerHTML = '';
            var errorDiv = document.createElement('div');
            errorDiv.className = 'loading';
            errorDiv.textContent = t('loadingPlans') + ' (' + err.message + ')';
            plansGrid.appendChild(errorDiv);
        });
}

// ── Plan Detail Loading ──
function loadPlanDetail(id) {
    setCurrentPlanID(id);
    saveSetting('lastopenedplan', id);
    fetch('/api/plans/' + id)
        .then(function (response) {
            if (!response.ok) throw new Error('HTTP ' + response.status);
            return response.json();
        })
        .then(function (data) {
            renderPlanDetail(data);
        })
        .catch(function (err) {
            planDetailContent.innerHTML = '';
            var errorDiv = document.createElement('div');
            errorDiv.className = 'loading';
            errorDiv.textContent = 'Error loading plan: ' + err.message;
            planDetailContent.appendChild(errorDiv);
        });
}

// ── View Switching ──
function switchView(viewName) {
    setCurrentView(viewName);
    viewPlans.style.display = 'none';
    viewPlanDetail.style.display = 'none';

    if (viewName === 'plans') {
        viewPlans.style.display = '';
        loadPlans();
    } else if (viewName === 'plan-detail') {
        viewPlanDetail.style.display = '';
    }
}

// ── Navigation ──
function navigateToPlan(id) {
    window.location.hash = 'plan/' + id;
}

function navigateToPlans() {
    window.location.hash = 'plans';
}

function handleRoute() {
    var hash = window.location.hash.replace('#', '') || 'plans';

    if (hash === 'plans') {
        switchView('plans');
    } else if (hash.indexOf('plan/') === 0) {
        var id = hash.replace('plan/', '');
        switchView('plan-detail');
        loadPlanDetail(id);
    }
}

// ── Sidebar Toggle ──
function initSidebarToggle() {
    if (!sidebarToggleBtn || !foldersSidebar) return;

    var stored = localStorage.getItem('tracker_sidebar_collapsed');
    if (stored === 'true') foldersSidebar.classList.add('collapsed');

    sidebarToggleBtn.addEventListener('click', function () {
        var isCollapsed = foldersSidebar.classList.toggle('collapsed');
        localStorage.setItem('tracker_sidebar_collapsed', String(isCollapsed));
        saveSetting('sidebar_collapsed', String(isCollapsed));
    });
}

function refreshCurrentUI() {
    updateStaticI18n();
    loadSessionInfo();
    if (currentView === 'plans') {
        loadPlans();
    } else if (currentView === 'plan-detail' && currentPlanID) {
        loadPlanDetail(currentPlanID);
    }
}

// ── Settings Sync ──
function syncSettings() {
    fetch('/api/settings')
        .then(function (res) { if (!res.ok) return; return res.json(); })
        .then(function (settings) {
            if (!settings) return;
            if (settings.custom_folders) {
                try {
                    var loadedFolders = JSON.parse(settings.custom_folders);
                    if (Array.isArray(loadedFolders)) {
                        setCustomFolders(loadedFolders);
                        localStorage.setItem('tracker_custom_folders', JSON.stringify(customFolders));
                    }
                } catch (e) {}
            }
            if (settings.lang && settings.lang !== getLang()) {
                setLang(settings.lang, function () {
                    refreshCurrentUI();
                });
                if (selectLang) selectLang.value = settings.lang;
            }
            if (settings.loglevel) {
                var selectLogLevel = document.getElementById('select-loglevel');
                if (selectLogLevel && selectLogLevel.value !== settings.loglevel) {
                    selectLogLevel.value = settings.loglevel;
                }
            }
            if (settings.selectedfolder !== undefined && settings.selectedfolder !== currentFolder) {
                setCurrentFolder(settings.selectedfolder);
                localStorage.setItem('tracker_selectedfolder', currentFolder);
                if (allPlans && allPlans.length > 0 && currentView === 'plans') {
                    renderPlans(allPlans);
                }
            }
            if (settings.sidebar_collapsed !== undefined && foldersSidebar) {
                if (settings.sidebar_collapsed === 'true') {
                    foldersSidebar.classList.add('collapsed');
                } else {
                    foldersSidebar.classList.remove('collapsed');
                }
                localStorage.setItem('tracker_sidebar_collapsed', settings.sidebar_collapsed);
            }
        })
        .catch(function () {});
}

// ── Log Level Control ──
function initLogLevelControl() {
    var selectLogLevel = document.getElementById('select-loglevel');
    if (!selectLogLevel) return;

    fetch('/api/config/loglevel')
        .then(function (res) { return res.json(); })
        .then(function (data) {
            if (data && data.level) {
                selectLogLevel.value = data.level;
            }
        })
        .catch(function () {});

    selectLogLevel.addEventListener('change', function () {
        var lvl = selectLogLevel.value;
        saveSetting('loglevel', lvl);
        fetch('/api/config/loglevel', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ level: lvl })
        })
            .then(function (res) { return res.json(); })
            .then(function (data) {
                if (data && data.level) {
                    selectLogLevel.value = data.level;
                }
            })
            .catch(function (err) {
                alert('Log level error: ' + err.message);
            });
    });
}

// ── WebSocket Message Handler ──
function handleWSMessage(msg) {
    if (msg.event === 'refresh') {
        if (currentView === 'plans') loadPlans();
        else if (currentView === 'plan-detail' && currentPlanID) loadPlanDetail(currentPlanID);
    } else if (msg.event === 'plan_updated') {
        if (currentView === 'plans') loadPlans();
        if (currentView === 'plan-detail' && msg.plan_id === currentPlanID) loadPlanDetail(currentPlanID);
    } else if (msg.event === 'task_updated') {
        if (currentView === 'plan-detail' && currentPlanID) loadPlanDetail(currentPlanID);
        else if (currentView === 'plans') loadPlans();
    } else if (msg.event === 'comment_added' || msg.event === 'comment_deleted') {
        if (currentView === 'plan-detail' && currentPlanID) loadPlanDetail(currentPlanID);
        else if (currentView === 'plans') loadPlans();
    } else if (msg.event === 'folder_renamed') {
        if (currentView === 'plans') loadPlans();
    } else if (msg.event === 'issue_reported' || msg.event === 'issue_updated') {
        if (currentView === 'plan-detail' && msg.plan_id === currentPlanID) loadPlanDetail(currentPlanID);
    } else if (msg.event === 'walkthrough_submitted') {
        if (currentView === 'plans') loadPlans();
        if (currentView === 'plan-detail' && msg.plan_id === currentPlanID) loadPlanDetail(currentPlanID);
    } else if (msg.event === 'setting_updated') {
        if (msg.key === 'custom_folders' && msg.value) {
            try {
                var wsFolders = JSON.parse(msg.value);
                if (Array.isArray(wsFolders)) {
                    setCustomFolders(wsFolders);
                    localStorage.setItem('tracker_custom_folders', JSON.stringify(customFolders));
                    if (allPlans && currentView === 'plans') {
                        updatePlansFolder(customFolders, allPlans);
                    }
                }
            } catch (e) {}
        }
    }
}

// ── Init ──
document.addEventListener('DOMContentLoaded', function () {
    window.addEventListener('hashchange', handleRoute);

    initLanguageSelector(function () {
        refreshCurrentUI();
    });
    initSidebarToggle();

    loadLocale(getLang(), function () {
        updateStaticI18n();
        loadVersionBadge();
        loadSessionInfo();
        syncSettings();
        handleRoute();
        initLogLevelControl();
        connectWS(handleWSMessage);
    });
});
