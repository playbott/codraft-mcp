// i18n module: loads locale JSON files and provides translation function
import { saveSetting } from './state.js';

var lang = localStorage.getItem('tracker_lang') || 'en';
var translations = {};
var loadedLocales = {};

// Synchronously load a locale from the cache or fetch it
export function loadLocale(locale, callback) {
    if (loadedLocales[locale]) {
        translations = loadedLocales[locale];
        updateStaticI18n();
        if (callback) callback();
        return;
    }
    fetch('/locales/' + locale + '.json')
        .then(function (res) {
            if (!res.ok) throw new Error('Failed to load locale: ' + locale);
            return res.json();
        })
        .then(function (data) {
            loadedLocales[locale] = data;
            translations = data;
            updateStaticI18n();
            if (callback) callback();
        })
        .catch(function () {
            // Fallback: try English
            if (locale !== 'en') {
                loadLocale('en', callback);
            } else {
                updateStaticI18n();
                if (callback) callback();
            }
        });
}

export function t(key) {
    return (translations && translations[key]) || key;
}

export function getLang() {
    return lang;
}

export function setLang(newLang, callback) {
    lang = newLang;
    localStorage.setItem('tracker_lang', lang);
    saveSetting('lang', lang);
    loadLocale(lang, callback);
}

export function updateStaticI18n() {
    var elements = document.querySelectorAll('[data-i18n]');
    for (var i = 0; i < elements.length; i++) {
        var el = elements[i];
        var key = el.getAttribute('data-i18n');
        if (key) {
            el.textContent = t(key);
        }
    }
    var wsStatus = document.getElementById('ws-status');
    if (wsStatus) {
        if (wsStatus.classList.contains('online')) {
            wsStatus.textContent = t('wsOnline');
        } else {
            wsStatus.textContent = t('wsOffline');
        }
    }
}

export function initLanguageSelector(onLangChange) {
    var selectLang = document.getElementById('select-lang');
    if (!selectLang) return;
    selectLang.value = lang;
    selectLang.addEventListener('change', function () {
        setLang(selectLang.value, function () {
            updateStaticI18n();
            if (onLangChange) onLangChange();
        });
    });
}

// Initialize: load the current language on module load
loadLocale(lang, null);
