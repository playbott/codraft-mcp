// Plans dashboard view
import { t } from '../i18n.js';
import { createPlanCard, createTaskCard } from '../components/card.js';
import { saveSetting, customFolders, setCustomFolders, saveCustomFolders, currentFolder, setCurrentFolder } from '../state.js';
import { showInputModal, setupInlineConfirmButton } from '../components/modal.js';

var plansGrid = null;
var folderTabs = null;
var foldersSidebar = null;
var folderListEl = null;
var allPlansRef = null;
var onNavigate = null;

export function initPlansView(opts) {
    plansGrid = opts.plansGrid;
    folderTabs = opts.folderTabs;
    foldersSidebar = opts.foldersSidebar;
    folderListEl = opts.folderListEl;
    onNavigate = opts.onNavigate;

    var btnAddFolder = document.getElementById('btn-add-folder');
    if (btnAddFolder) {
        btnAddFolder.addEventListener('click', function () {
            showInputModal(t('renameFolder'), t('enterFolderName'), '', function (name) {
                if (!name) return;
                var cf = customFolders.slice();
                if (cf.indexOf(name) === -1) cf.push(name);
                setCustomFolders(cf);
                saveCustomFolders();
                renderFoldersSidebar(allPlansRef);
            });
        });
    }
}

export function renderPlans(plans) {
    allPlansRef = plans || [];
    renderPlansGrid(allPlansRef);
    loadUnassignedTasks();
}

function loadUnassignedTasks() {
    var url = '/api/tasks';
    fetch(url)
        .then(function (response) {
            if (!response.ok) throw new Error('HTTP ' + response.status);
            return response.json();
        })
        .then(function (tasks) {
            var orphans = [];
            for (var i = 0; i < tasks.length; i++) {
                if (!tasks[i].plan_id) {
                    orphans.push(tasks[i]);
                }
            }
            if (orphans.length === 0) return;

            var sep = document.createElement('div');
            sep.className = 'section-title';
            sep.textContent = t('unassignedTasks');
            sep.style.marginTop = '2rem';
            plansGrid.appendChild(sep);

            for (var j = 0; j < orphans.length; j++) {
                plansGrid.appendChild(createTaskCard(orphans[j], null));
            }
        })
        .catch(function () {});
}

function renderPlansGrid(plans) {
    if (folderTabs) renderFolderTabs(plans);
    if (foldersSidebar) renderFoldersSidebar(plans);
    if (!plansGrid) return;
    plansGrid.innerHTML = '';

    var filtered = plans;
    if (currentFolder === '__none__') {
        filtered = plans.filter(function (p) { return !p.folder; });
    } else if (currentFolder !== '') {
        filtered = plans.filter(function (p) { return p.folder === currentFolder; });
    }

    if (!filtered || filtered.length === 0) {
        var empty = document.createElement('div');
        empty.className = 'loading';
        empty.textContent = t('noPlansInFolder');
        plansGrid.appendChild(empty);
        return;
    }

    for (var i = 0; i < filtered.length; i++) {
        plansGrid.appendChild(createPlanCard(filtered[i], onNavigate));
    }
}

function renderFolderTabs(plans) {
    if (!folderTabs) return;
    folderTabs.innerHTML = '';

    var foldersSet = {};
    for (var i = 0; i < plans.length; i++) {
        if (plans[i].folder) {
            foldersSet[plans[i].folder] = true;
        }
    }
    for (var cf = 0; cf < customFolders.length; cf++) {
        foldersSet[customFolders[cf]] = true;
    }
    var folderList = Object.keys(foldersSet).sort();

    var allTab = document.createElement('button');
    allTab.className = 'folder-tab' + (currentFolder === '' ? ' active' : '');
    allTab.textContent = t('allPlans') + ' (' + plans.length + ')';
    allTab.addEventListener('click', function () {
        setCurrentFolder('');
        renderPlansGrid(allPlansRef);
    });
    folderTabs.appendChild(allTab);

    for (var j = 0; j < folderList.length; j++) {
        (function (fname) {
            var count = plans.filter(function (p) { return p.folder === fname; }).length;
            var tab = document.createElement('button');
            tab.className = 'folder-tab' + (currentFolder === fname ? ' active' : '');
            tab.textContent = '📁 ' + fname + ' (' + count + ')';
            tab.addEventListener('click', function () {
                setCurrentFolder(fname);
                renderPlansGrid(allPlansRef);
            });
            folderTabs.appendChild(tab);
        })(folderList[j]);
    }

    var noFolderCount = plans.filter(function (p) { return !p.folder; }).length;
    if (noFolderCount > 0 && folderList.length > 0) {
        var noFolderTab = document.createElement('button');
        noFolderTab.className = 'folder-tab' + (currentFolder === '__none__' ? ' active' : '');
        noFolderTab.textContent = t('noFolder') + ' (' + noFolderCount + ')';
        noFolderTab.addEventListener('click', function () {
            setCurrentFolder('__none__');
            renderPlansGrid(allPlansRef);
        });
        folderTabs.appendChild(noFolderTab);
    }
}

function setupFolderDropTarget(element, targetFolder) {
    element.addEventListener('dragover', function (e) {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        element.classList.add('folder-drop-active');
    });

    element.addEventListener('dragleave', function () {
        element.classList.remove('folder-drop-active');
    });

    element.addEventListener('drop', function (e) {
        e.preventDefault();
        element.classList.remove('folder-drop-active');
        var planID = e.dataTransfer.getData('text/plain');
        if (!planID) return;
        fetch('/api/plans/' + encodeURIComponent(planID) + '/folder', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ folder: targetFolder })
        })
            .then(function (res) { if (!res.ok) throw new Error('HTTP ' + res.status); return res.json(); })
            .then(function () {
                if (typeof loadPlansExternal === 'function') loadPlansExternal();
            })
            .catch(function (err) { alert('Move to folder error: ' + err.message); });
    });
}

// Set by main.js after init
export var loadPlansExternal = null;
export function setLoadPlansExternal(fn) { loadPlansExternal = fn; }

function renderFoldersSidebar(plans) {
    if (!folderListEl) return;
    folderListEl.innerHTML = '';

    var foldersSet = {};
    for (var i = 0; i < plans.length; i++) {
        if (plans[i].folder) {
            foldersSet[plans[i].folder] = true;
        }
    }
    for (var cf = 0; cf < customFolders.length; cf++) {
        foldersSet[customFolders[cf]] = true;
    }
    var folderList = Object.keys(foldersSet).sort();

    // 1. "All plans"
    var allItem = document.createElement('div');
    allItem.className = 'folder-nav-item' + (currentFolder === '' ? ' active' : '');
    allItem.innerHTML = '<div class="folder-item-left">📋 ' + t('allPlans') + '</div><div class="folder-item-right"><span class="folder-count-badge">' + plans.length + '</span></div>';
    allItem.addEventListener('click', function () {
        setCurrentFolder('');
        saveSetting('selectedfolder', currentFolder);
        renderPlansGrid(allPlansRef);
    });
    setupFolderDropTarget(allItem, '');
    folderListEl.appendChild(allItem);

    // 2. Existing folders
    for (var j = 0; j < folderList.length; j++) {
        (function (fname) {
            var count = plans.filter(function (p) { return p.folder === fname; }).length;

            var item = document.createElement('div');
            item.className = 'folder-nav-item' + (currentFolder === fname ? ' active' : '');

            var left = document.createElement('div');
            left.className = 'folder-item-left';
            left.textContent = '📁 ' + fname;

            var right = document.createElement('div');
            right.className = 'folder-item-right';

            var badge = document.createElement('span');
            badge.className = 'folder-count-badge';
            badge.textContent = count;

            var editBtn = document.createElement('button');
            editBtn.className = 'btn-icon-sm';
            editBtn.textContent = '✏️';
            editBtn.title = t('rename');
            editBtn.addEventListener('click', function (e) {
                e.stopPropagation();
                showInputModal(t('renameFolder'), t('enterFolderName'), fname, function (newName) {
                    if (!newName || newName === fname) return;
                    fetch('/api/folders/rename', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ old_name: fname, new_name: newName })
                    })
                        .then(function (res) { if (!res.ok) throw new Error('HTTP ' + res.status); return res.json(); })
                        .then(function () {
                            var idx = customFolders.indexOf(fname);
                            var cf = customFolders.slice();
                            if (idx !== -1) cf[idx] = newName;
                            else cf.push(newName);
                            setCustomFolders(cf);
                            saveCustomFolders();
                            if (currentFolder === fname) {
                                setCurrentFolder(newName);
                                saveSetting('selectedfolder', currentFolder);
                            }
                            if (loadPlansExternal) loadPlansExternal();
                        })
                        .catch(function (err) { alert('Rename folder error: ' + err.message); });
                });
            }, false);

            right.appendChild(badge);
            right.appendChild(editBtn);

            var deleteFolderBtn = document.createElement('button');
            deleteFolderBtn.className = 'btn-icon-sm';
            deleteFolderBtn.textContent = '🗑️';
            deleteFolderBtn.title = t('delete');
            (function (folderNameVal, btnEl) {
                setupInlineConfirmButton(btnEl, '🗑️', '❗', function () {
                    var idx = customFolders.indexOf(folderNameVal);
                    var cf = customFolders.slice();
                    if (idx !== -1) cf.splice(idx, 1);
                    setCustomFolders(cf);
                    saveCustomFolders();
                    if (currentFolder === folderNameVal) {
                        setCurrentFolder('');
                        saveSetting('selectedfolder', currentFolder);
                    }
                    fetch('/api/folders?name=' + encodeURIComponent(folderNameVal), {
                        method: 'DELETE'
                    })
                        .then(function (res) {
                            if (!res.ok) {
                                return fetch('/api/folders/delete', {
                                    method: 'POST',
                                    headers: { 'Content-Type': 'application/json' },
                                    body: JSON.stringify({ name: folderNameVal })
                                });
                            }
                            return res;
                        })
                        .then(function (res) { if (!res.ok) throw new Error('HTTP ' + res.status); return res.json(); })
                        .then(function () { if (loadPlansExternal) loadPlansExternal(); })
                        .catch(function (err) { alert('Delete folder error: ' + err.message); });
                });
            })(fname, deleteFolderBtn);
            right.appendChild(deleteFolderBtn);

            item.appendChild(left);
            item.appendChild(right);

            item.addEventListener('click', function () {
                setCurrentFolder(fname);
                saveSetting('selectedfolder', currentFolder);
                renderPlansGrid(allPlansRef);
            });

            setupFolderDropTarget(item, fname);
            folderListEl.appendChild(item);
        })(folderList[j]);
    }

    // 4. "No folder"
    var noFolderCount = plans.filter(function (p) { return !p.folder; }).length;
    if (noFolderCount > 0 || folderList.length > 0) {
        var noFolderItem = document.createElement('div');
        noFolderItem.className = 'folder-nav-item' + (currentFolder === '__none__' ? ' active' : '');
        noFolderItem.innerHTML = '<div class="folder-item-left">📄 ' + t('noFolder') + '</div><div class="folder-item-right"><span class="folder-count-badge">' + noFolderCount + '</span><span class="folder-slot-empty"></span><span class="folder-slot-empty"></span></div>';
        noFolderItem.addEventListener('click', function () {
            setCurrentFolder('__none__');
            saveSetting('selectedfolder', currentFolder);
            renderPlansGrid(allPlansRef);
        });
        setupFolderDropTarget(noFolderItem, '');
        folderListEl.appendChild(noFolderItem);
    }
}

export function updatePlansFolder(folders, plans) {
    setCustomFolders(folders);
    renderFoldersSidebar(plans);
}
