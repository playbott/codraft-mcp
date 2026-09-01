// WebSocket connection manager
import { t, updateStaticI18n } from './i18n.js';

var ws = null;
var reconnectDelay = 3000;
var maxReconnectDelay = 15000;

// Callbacks set by main.js
export var onMessage = null;

function setOnline() {
    var wsStatus = document.getElementById('ws-status');
    if (!wsStatus) return;
    wsStatus.textContent = t('wsOnline');
    wsStatus.classList.remove('offline');
    wsStatus.classList.add('online');
}

function setOffline() {
    var wsStatus = document.getElementById('ws-status');
    if (!wsStatus) return;
    wsStatus.textContent = t('wsOffline');
    wsStatus.classList.remove('online');
    wsStatus.classList.add('offline');
}

export function connectWS(messageHandler) {
    if (messageHandler) onMessage = messageHandler;
    ws = new WebSocket('ws://' + location.host + '/ws');

    ws.onopen = function () {
        setOnline();
        reconnectDelay = 3000;
    };

    ws.onclose = function () {
        setOffline();
        ws = null;

        fetch('/api/ping')
            .then(function (res) {
                if (!res.ok) throw new Error('Dead');
            })
            .catch(function () {
                try {
                    window.close();
                } catch (e) {}
            });

        setTimeout(function () {
            connectWS(null);
            if (reconnectDelay < maxReconnectDelay) {
                reconnectDelay = Math.min(reconnectDelay * 2, maxReconnectDelay);
            }
        }, reconnectDelay);
    };

    ws.onerror = function () {
        if (ws) {
            ws.close();
        }
    };

    ws.onmessage = function (event) {
        try {
            var msg = JSON.parse(event.data);
            if (onMessage) onMessage(msg);
        } catch (e) {
            // ignore malformed messages
        }
    };
}
