// Centralized application state
export var currentView = '';
export var currentPlanID = '';
export var currentPlanData = null;
export var currentFolder = localStorage.getItem('tracker_selectedfolder') || localStorage.getItem('tracker_selected_folder') || '';
export var allPlans = [];
export var customFolders = [];

try {
    customFolders = JSON.parse(localStorage.getItem('tracker_custom_folders') || '[]');
    if (!Array.isArray(customFolders)) customFolders = [];
} catch (e) {
    customFolders = [];
}

export function setCurrentView(v) { currentView = v; }
export function setCurrentPlanID(id) { currentPlanID = id; }
export function setCurrentPlanData(d) { currentPlanData = d; }
export function setCurrentFolder(f) { currentFolder = f; }
export function setAllPlans(p) { allPlans = p; }
export function setCustomFolders(f) { customFolders = f; }

export function saveCustomFolders() {
    var jsonStr = JSON.stringify(customFolders);
    try {
        localStorage.setItem('tracker_custom_folders', jsonStr);
    } catch (e) {}
    saveSetting('custom_folders', jsonStr);
}

export function saveSetting(key, value) {
    if (!key) return;
    var valStr = value !== undefined && value !== null ? String(value) : '';
    try {
        localStorage.setItem('tracker_' + key, valStr);
    } catch (e) {}
    fetch('/api/settings', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key: key, value: valStr })
    }).catch(function () {});
}
