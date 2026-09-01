const vscode = require('vscode');
const fs = require('fs');
const path = require('path');
const http = require('http');
const os = require('os');

const PORT_FILE_NAME = 'tracker.port';
const CODRAFT_DIR = '.codraft';
const REGISTRY_FILE_NAME = 'ports.json';
const REGISTRY_TTL_MS = 45000;

let watcher = null;
let lastOpenedPort = null;
let debounceTimer = null;
let healthTimer = null;
let sweepInProgress = false;

function extLog(msg) {
    try {
        const p = path.join(os.tmpdir(), 'codraft-extension.log');
        fs.appendFileSync(p, new Date().toISOString() + ' ' + msg + '\n');
    } catch (e) {}
}

function normalizePath(p) {
    if (!p) return '';
    return String(p).toLowerCase().replace(/\\/g, '/').replace(/\/+$/, '');
}

function readPortFromFile(portFilePath) {
    try {
        if (!fs.existsSync(portFilePath)) return 0;
        const content = fs.readFileSync(portFilePath, 'utf8').trim();
        if (!/^\d+$/.test(content)) return 0;
        return parseInt(content, 10);
    } catch (e) {
        return 0;
    }
}

async function closeSimpleBrowserTabs() {
    try {
        const tabGroups = vscode.window.tabGroups;
        if (!tabGroups || !tabGroups.all) return;

        const tabsToClose = [];
        for (const group of tabGroups.all) {
            for (const tab of group.tabs) {
                if (tab.input && tab.input.viewType === 'simpleBrowser.view') {
                    tabsToClose.push(tab);
                    continue;
                }
                if (tab.label) {
                    const label = tab.label.toLowerCase();
                    if (label.includes('simple browser') || label.includes('task tracker') || label.includes('tracker') || label.includes('codraft')) {
                        tabsToClose.push(tab);
                    }
                }
            }
        }
        if (tabsToClose.length > 0) {
            await tabGroups.close(tabsToClose, true);
        }
    } catch (e) {
        console.error('Error closing Simple Browser tabs', e);
    }
}

function checkServerHealth(port) {
    if (!port) return;
    const req = http.get("http://localhost:" + port + "/api/ping", { timeout: 1000 }, (res) => {
        if (res.statusCode !== 200) {
            onServerDead();
        }
    });
    req.on('error', () => {
        onServerDead();
    });
}

function onServerDead() {
    if (lastOpenedPort) {
        extLog('server dead, closing tab (port=' + lastOpenedPort + ')');
        lastOpenedPort = null;
        closeSimpleBrowserTabs();
    }
}

function registryUrlForPort(port) {
    try {
        const regPath = path.join(os.homedir(), CODRAFT_DIR, REGISTRY_FILE_NAME);
        if (!fs.existsSync(regPath)) return "http://localhost:" + port;
        const data = JSON.parse(fs.readFileSync(regPath, 'utf8'));
        if (!data || typeof data !== 'object') return "http://localhost:" + port;
        for (const key in data) {
            const entry = data[key];
            if (entry && typeof entry === 'object' && parseInt(entry.port, 10) === port) {
                let url = "http://localhost:" + port;
                if (entry.client || entry.ide || entry.cwd || entry.pid) {
                    url += "?client=" + encodeURIComponent(entry.client || '') +
                        "&ide=" + encodeURIComponent(entry.ide || '') +
                        "&cwd=" + encodeURIComponent(entry.cwd || '') +
                        "&pid=" + encodeURIComponent(entry.pid || '');
                }
                return url;
            }
        }
    } catch (e) {}
    return "http://localhost:" + port;
}

function queueOpenSimpleBrowser(port, url) {
    if (!port || !url) return;

    if (port === lastOpenedPort) {
        return;
    }

    if (debounceTimer) {
        clearTimeout(debounceTimer);
    }

    debounceTimer = setTimeout(async () => {
        debounceTimer = null;
        try {
            extLog('show tab: ' + url);
            await closeSimpleBrowserTabs();
            lastOpenedPort = port;
            await vscode.commands.executeCommand('simpleBrowser.show', url);
        } catch (e) {
            extLog('error opening Simple Browser: ' + e);
        }
    }, 400);
}

function sweepStaleTabs(rootPath, portFiles) {
    if (sweepInProgress) return;
    const count = countSimpleBrowserTabs();
    if (count <= 1) {
        extLog('sweep: tabs=' + count + ', nothing to close');
        return;
    }
    extLog('sweep: tabs=' + count + ', closing extras and reopening ours');
    sweepInProgress = true;
    closeSimpleBrowserTabs().then(() => {
        sweepInProgress = false;
        lastOpenedPort = null;
        for (const f of portFiles) {
            const port = readPortFromFile(f);
            if (port > 0) {
                queueOpenSimpleBrowser(port, registryUrlForPort(port));
                return;
            }
        }
    });
}

function countSimpleBrowserTabs() {
    try {
        const tabGroups = vscode.window.tabGroups;
        if (!tabGroups || !tabGroups.all) return 0;
        let count = 0;
        for (const group of tabGroups.all) {
            for (const tab of group.tabs) {
                if (tab.input && tab.input.viewType === 'simpleBrowser.view') {
                    count++;
                } else if (tab.label) {
                    const l = tab.label.toLowerCase();
                    if (l.includes('simple browser') || l.includes('codraft') || l.includes('tracker') || l.includes('localhost')) {
                        count++;
                    }
                }
            }
        }
        return count;
    } catch (e) {
        return 0;
    }
}

function checkInitial(rootPath, portFiles) {
    for (const f of portFiles) {
        const port = readPortFromFile(f);
        if (port <= 0) continue;
        const req = http.get("http://127.0.0.1:" + port + "/api/ping", { timeout: 800 }, (res) => {
            if (res.statusCode === 200) {
                console.log('[codraft-ui-opener] initial check: server alive on ' + port + ', opening tab');
                queueOpenSimpleBrowser(port, registryUrlForPort(port));
            } else {
                console.log('[codraft-ui-opener] initial check: port ' + port + ' not responding, waiting for watcher event');
            }
        });
        req.on('error', () => {
            console.log('[codraft-ui-opener] initial check: port ' + port + ' unreachable, waiting for watcher event');
        });
        req.setTimeout(800, () => { req.destroy(); });
        return;
    }
}

function activate(context) {
    const workspaceFolders = vscode.workspace.workspaceFolders;
    if (!workspaceFolders) return;

    const rootPath = workspaceFolders[0].uri.fsPath;
    const portFiles = [
        path.join(rootPath, CODRAFT_DIR, PORT_FILE_NAME),
        path.join(rootPath, '.vscode', PORT_FILE_NAME),
        path.join(rootPath, PORT_FILE_NAME)
    ];

    setTimeout(() => {
        checkInitial(rootPath, portFiles);
    }, 1000);

    watcher = vscode.workspace.createFileSystemWatcher('{**/' + CODRAFT_DIR + '/' + PORT_FILE_NAME + ',**/.vscode/' + PORT_FILE_NAME + ',**/' + PORT_FILE_NAME + '}');

    watcher.onDidCreate((uri) => {
        const port = readPortFromFile(uri.fsPath);
        if (port > 0) {
            queueOpenSimpleBrowser(port, registryUrlForPort(port));
        }
    });
    watcher.onDidChange((uri) => {
        const port = readPortFromFile(uri.fsPath);
        if (port > 0) {
            queueOpenSimpleBrowser(port, registryUrlForPort(port));
        }
    });
    watcher.onDidDelete(() => {
    });

    healthTimer = setInterval(() => {
        if (lastOpenedPort) {
            checkServerHealth(lastOpenedPort);
        }
    }, 2000);

    setTimeout(() => {
        sweepStaleTabs(rootPath, portFiles);
    }, 3000);
    setTimeout(() => {
        sweepStaleTabs(rootPath, portFiles);
    }, 10000);

    context.subscriptions.push(watcher);
}

function deactivate() {
    if (healthTimer) {
        clearInterval(healthTimer);
    }
    closeSimpleBrowserTabs();
    if (watcher) {
        watcher.dispose();
    }
}

module.exports = {
    activate,
    deactivate
}
