// REST API calls wrapper
import { allPlans, currentPlanID, currentView } from './state.js';

export function sendJSON(url, method, body, onSuccess) {
    fetch(url, {
        method: method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
    })
    .then(function (response) {
        if (!response.ok) {
            return response.json().then(function (data) {
                alert('Error: ' + (data.error || response.statusText));
            });
        }
        return response.json();
    })
    .then(function () {
        if (onSuccess) onSuccess();
    })
    .catch(function (err) {
        alert('Error: ' + err.message);
    });
}

export function apiFetch(url, onSuccess, onError) {
    fetch(url)
        .then(function (response) {
            if (!response.ok) throw new Error('HTTP ' + response.status);
            return response.json();
        })
        .then(onSuccess)
        .catch(onError || function () {});
}
