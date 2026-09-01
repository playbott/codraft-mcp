// Modal dialog and inline confirm button utilities
import { t } from '../i18n.js';

export function closeModal() {
    var appModal = document.getElementById('app-modal');
    if (appModal) {
        appModal.style.display = 'none';
        var modalTitle = document.getElementById('modal-title');
        var modalBody = document.getElementById('modal-body');
        var modalActions = document.getElementById('modal-actions');
        if (modalTitle) modalTitle.innerHTML = '';
        if (modalBody) modalBody.innerHTML = '';
        if (modalActions) modalActions.innerHTML = '';
    }
}

export function showConfirmModal(titleText, bodyText, onConfirm, isDanger) {
    var appModal = document.getElementById('app-modal');
    if (!appModal) return;
    var modalTitle = document.getElementById('modal-title');
    var modalBody = document.getElementById('modal-body');
    var modalActions = document.getElementById('modal-actions');
    modalTitle.textContent = titleText;
    modalBody.textContent = bodyText;
    modalActions.innerHTML = '';

    var btnYes = document.createElement('button');
    btnYes.className = isDanger !== false ? 'btn-confirm-danger' : 'btn-confirm-yes';
    btnYes.textContent = t('yes');
    btnYes.addEventListener('click', function () {
        closeModal();
        if (onConfirm) onConfirm();
    });

    var btnNo = document.createElement('button');
    btnNo.className = 'btn-confirm-no';
    btnNo.textContent = t('no');
    btnNo.addEventListener('click', function () {
        closeModal();
    });

    modalActions.appendChild(btnNo);
    modalActions.appendChild(btnYes);
    appModal.style.display = 'flex';
}

export function showInputModal(titleText, labelText, defaultValue, onSave, isTextArea) {
    var appModal = document.getElementById('app-modal');
    if (!appModal) return;
    var modalTitle = document.getElementById('modal-title');
    var modalBody = document.getElementById('modal-body');
    var modalActions = document.getElementById('modal-actions');
    modalTitle.textContent = titleText;
    modalBody.innerHTML = '';

    if (labelText) {
        var label = document.createElement('div');
        label.style.marginBottom = '8px';
        label.textContent = labelText;
        modalBody.appendChild(label);
    }

    var input;
    if (isTextArea) {
        input = document.createElement('textarea');
        input.rows = 4;
        input.value = defaultValue || '';
        input.addEventListener('keydown', function (e) {
            if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
                e.preventDefault();
                doSave();
            }
        });
    } else {
        input = document.createElement('input');
        input.type = 'text';
        input.value = defaultValue || '';
        input.addEventListener('keypress', function (e) {
            if (e.key === 'Enter') doSave();
        });
    }
    modalBody.appendChild(input);

    modalActions.innerHTML = '';

    var btnSave = document.createElement('button');
    btnSave.className = 'btn-confirm-yes';
    btnSave.textContent = t('save');

    function doSave() {
        var val = input.value.trim();
        closeModal();
        if (onSave) onSave(val);
    }

    btnSave.addEventListener('click', doSave);

    var btnCancel = document.createElement('button');
    btnCancel.className = 'btn-confirm-no';
    btnCancel.textContent = t('cancel');
    btnCancel.addEventListener('click', function () {
        closeModal();
    });

    modalActions.appendChild(btnCancel);
    modalActions.appendChild(btnSave);
    appModal.style.display = 'flex';
    setTimeout(function () { input.focus(); if (input.select) input.select(); }, 50);
}

export function setupInlineConfirmButton(btn, normalText, confirmText, onConfirm) {
    var isConfirming = false;
    var resetTimer = null;
    if (normalText !== undefined && normalText !== null) {
        btn.textContent = normalText;
    }

    function resetState() {
        isConfirming = false;
        btn.textContent = normalText;
        btn.classList.remove('btn-confirm-inline');
        if (resetTimer) {
            clearTimeout(resetTimer);
            resetTimer = null;
        }
    }

    btn.addEventListener('click', function (e) {
        e.stopPropagation();
        if (!isConfirming) {
            isConfirming = true;
            btn.textContent = confirmText || t('clickToConfirm');
            btn.classList.add('btn-confirm-inline');
            resetTimer = setTimeout(resetState, 4000);
        } else {
            resetState();
            if (onConfirm) onConfirm();
        }
    });

    btn.addEventListener('mouseleave', function () {
        if (isConfirming && !resetTimer) {
            resetTimer = setTimeout(resetState, 2000);
        }
    });
}
