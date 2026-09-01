// Issue item component
import { t } from '../i18n.js';
import { showInputModal } from './modal.js';

export function statusLabel(status, type) {
    if (type === 'task') {
        switch (status) {
            case 'needs_approval': return t('statusNeedsApproval');
            case 'pending': return t('statusPending');
            case 'in_progress': return t('statusInProgress');
            case 'done': return t('statusDone');
            case 'canceled': return t('statusCanceled');
            default: return status;
        }
    }
    if (type === 'issue') {
        switch (status) {
            case 'open': return t('statusOpen');
            case 'in_progress': return t('statusInProgress');
            case 'resolved': return t('statusResolved');
            default: return status;
        }
    }
    if (type === 'plan') {
        switch (status) {
            case 'draft': return t('statusDraft');
            case 'approved': return t('statusApproved');
            case 'in_progress': return t('statusInProgress');
            case 'on_hold': return t('statusOnHold');
            case 'review': return t('statusReview');
            case 'completed': return t('statusCompleted');
            case 'canceled': return t('statusCanceled');
            case 'rejected': return t('statusRejected');
            default: return status;
        }
    }
    return status;
}

export function createIssueItem(issue, onUpdated) {
    var item = document.createElement('div');
    item.className = 'issue-item';

    var header = document.createElement('div');
    header.className = 'issue-header';

    var desc = document.createElement('span');
    desc.textContent = issue.description;

    var editIssueBtn = document.createElement('button');
    editIssueBtn.className = 'btn-edit';
    editIssueBtn.textContent = t('edit');
    editIssueBtn.title = t('editIssueTitle');
    (function (iID, iDesc) {
        editIssueBtn.addEventListener('click', function (e) {
            e.stopPropagation();
            showInputModal(t('editIssueTitle'), t('enterIssueDesc'), iDesc, function (newDesc) {
                if (!newDesc || newDesc === iDesc) return;
                fetch('/api/issues/' + encodeURIComponent(iID) + '/description', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ description: newDesc })
                })
                    .then(function (res) { if (!res.ok) throw new Error('HTTP ' + res.status); return res.json(); })
                    .then(function () { if (onUpdated) onUpdated(); })
                    .catch(function (err) { alert('Issue edit error: ' + err.message); });
            }, true);
        });
    })(issue.id, issue.description);
    desc.appendChild(editIssueBtn);

    var tag = document.createElement('span');
    tag.className = 'tag ' + issue.status;
    tag.textContent = statusLabel(issue.status, 'issue');

    header.appendChild(desc);
    header.appendChild(tag);
    item.appendChild(header);

    if (issue.fix_notes) {
        var notes = document.createElement('div');
        notes.className = 'issue-notes';
        notes.textContent = issue.fix_notes;
        item.appendChild(notes);
    }

    return item;
}
