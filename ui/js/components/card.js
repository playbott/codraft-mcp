// Plan card component for dashboard
import { t } from '../i18n.js';
import { statusLabel, createIssueItem } from './issue.js';
import { formatDate, createTaskCommentsSection } from './comment.js';
import { showInputModal } from './modal.js';

export function createTaskCard(task, onReload) {
    var card = document.createElement('div');
    card.className = 'task-card';

    var header = document.createElement('div');
    header.className = 'task-header';

    var left = document.createElement('div');
    var id = document.createElement('div');
    id.className = 'task-id';
    id.textContent = task.id;

    var title = document.createElement('div');
    title.className = 'task-title';
    title.textContent = task.title;

    var editTaskBtn = document.createElement('button');
    editTaskBtn.className = 'btn-edit';
    editTaskBtn.textContent = t('rename');
    editTaskBtn.title = t('renameTaskTitle');
    (function (tID, tTitle) {
        editTaskBtn.addEventListener('click', function (e) {
            e.stopPropagation();
            showInputModal(t('renameTaskTitle'), t('enterTaskTitle'), tTitle, function (newTitle) {
                if (!newTitle || newTitle === tTitle) return;
                fetch('/api/tasks/' + encodeURIComponent(tID) + '/title', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ title: newTitle })
                })
                    .then(function (res) { if (!res.ok) throw new Error('HTTP ' + res.status); return res.json(); })
                    .then(function () { if (onReload) onReload(); })
                    .catch(function (err) { alert('Task rename error: ' + err.message); });
            }, true);
        });
    })(task.id, task.title);
    title.appendChild(editTaskBtn);

    left.appendChild(id);
    left.appendChild(title);

    var tag = document.createElement('span');
    tag.className = 'tag ' + task.status;
    tag.textContent = statusLabel(task.status, 'task');

    header.appendChild(left);
    header.appendChild(tag);
    card.appendChild(header);

    if (task.issues && task.issues.length > 0) {
        var list = document.createElement('div');
        list.className = 'issues-list';

        for (var i = 0; i < task.issues.length; i++) {
            list.appendChild(createIssueItem(task.issues[i], onReload));
        }

        card.appendChild(list);
    }

    // Add per-task comments
    card.appendChild(createTaskCommentsSection(task, onReload));

    return card;
}

export function createPlanCard(plan, onNavigate) {
    var card = document.createElement('div');
    card.className = 'plan-card';
    card.draggable = true;
    (function (pID) {
        card.addEventListener('dragstart', function (e) {
            e.dataTransfer.setData('text/plain', pID);
            e.dataTransfer.effectAllowed = 'move';
            card.classList.add('dragging');
        });
        card.addEventListener('dragend', function () {
            card.classList.remove('dragging');
        });
    })(plan.id);

    if (plan.folder) {
        var folderPlain = document.createElement('div');
        folderPlain.className = 'card-folder-plain';
        folderPlain.textContent = plan.folder;
        card.appendChild(folderPlain);
    }

    var header = document.createElement('div');
    header.className = 'plan-card-header';

    var titleEl = document.createElement('span');
    titleEl.className = 'plan-card-title';
    titleEl.textContent = plan.title || plan.id;

    var tag = document.createElement('span');
    tag.className = 'tag ' + plan.status;
    tag.textContent = statusLabel(plan.status, 'plan');

    header.appendChild(titleEl);
    header.appendChild(tag);
    card.appendChild(header);

    // Description
    if (plan.description) {
        var desc = document.createElement('div');
        desc.className = 'plan-card-desc';
        desc.textContent = plan.description.substring(0, 160) + (plan.description.length > 160 ? '...' : '');
        card.appendChild(desc);
    }

    // Date + Tasks meta row
    var metaRow = document.createElement('div');
    metaRow.className = 'plan-card-meta-row';

    var dateEl = document.createElement('span');
    dateEl.className = 'plan-card-date';
    dateEl.textContent = formatDate(plan.created_at);
    metaRow.appendChild(dateEl);

    // Parse progress from dummy task
    var progress = '0/0';
    if (plan.tasks && plan.tasks.length > 0 && plan.tasks[0].id) {
        progress = plan.tasks[0].id;
    }
    var parts = progress.split('/');
    var done = parseInt(parts[0], 10) || 0;
    var total = parseInt(parts[1], 10) || 0;
    var pct = total > 0 ? Math.round((done / total) * 100) : 0;

    var tasksEl = document.createElement('span');
    tasksEl.className = 'plan-card-tasks';
    tasksEl.textContent = t('tasksProgress') + ' ' + progress;
    metaRow.appendChild(tasksEl);

    card.appendChild(metaRow);

    // Progress bar
    if (total > 0) {
        var bar = document.createElement('div');
        bar.className = 'progress-bar';
        var fill = document.createElement('div');
        fill.className = 'progress-fill';
        fill.style.width = pct + '%';
        bar.appendChild(fill);
        card.appendChild(bar);
    }

    card.addEventListener('click', function () {
        if (onNavigate) onNavigate(plan.id);
    });

    return card;
}
