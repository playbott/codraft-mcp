// Plan detail view
import { t } from '../i18n.js';
import { statusLabel } from '../components/issue.js';
import { formatDate, createCommentForm, loadComments, createTaskCommentsSection } from '../components/comment.js';
import { showConfirmModal, showInputModal } from '../components/modal.js';
import { createTaskCard } from '../components/card.js';
import { sendJSON } from '../api.js';
import { currentPlanID, setCurrentPlanID, setCurrentPlanData } from '../state.js';

var planDetailContent = null;
var onBack = null;
var onPlanAction = null;
var loadPlanDetailFn = null;

export function initPlanDetailView(opts) {
    planDetailContent = opts.planDetailContent;
    onBack = opts.onBack;
    loadPlanDetailFn = opts.loadPlanDetail;
}

export function renderPlanDetail(plan) {
    setCurrentPlanData(plan);
    setCurrentPlanID(plan.id);
    if (!planDetailContent) return;
    planDetailContent.innerHTML = '';

    // Back link
    var back = document.createElement('a');
    back.className = 'back-link';
    back.textContent = t('backToPlans');
    back.href = '#plans';
    planDetailContent.appendChild(back);

    // Header
    var header = document.createElement('div');
    header.className = 'plan-detail-header';

    var titleEl = document.createElement('div');
    titleEl.className = 'plan-detail-title';
    titleEl.textContent = plan.title;

    var tag = document.createElement('span');
    tag.className = 'tag ' + plan.status;
    tag.textContent = statusLabel(plan.status, 'plan');

    header.appendChild(titleEl);
    header.appendChild(tag);

    var dateMeta = document.createElement('div');
    dateMeta.className = 'plan-card-date';
    dateMeta.style.marginTop = '0.25rem';
    dateMeta.textContent = t('created') + ' ' + formatDate(plan.created_at);

    var folderSpan = document.createElement('span');
    folderSpan.className = 'folder-badge';
    folderSpan.style.marginLeft = '0.75rem';
    folderSpan.textContent = '📁 ' + (plan.folder || t('noFolder'));
    dateMeta.appendChild(folderSpan);

    header.appendChild(dateMeta);

    if (plan.description) {
        var desc = document.createElement('div');
        desc.className = 'plan-detail-desc';
        desc.textContent = plan.description;
        header.appendChild(desc);
    }

    planDetailContent.appendChild(header);

    // Route based on status
    if (plan.status === 'draft' || plan.status === 'rejected') {
        renderDraftMode(plan);
    } else if (plan.status === 'review') {
        renderReviewMode(plan);
    } else {
        renderDefaultPlanView(plan);
    }
}

function planAction(planID, action, onDone) {
    sendJSON('/api/plans/' + planID + '/' + action, 'POST', {}, function () {
        if (loadPlanDetailFn) loadPlanDetailFn(planID);
        else if (onDone) onDone();
    });
}

function approveOrRejectTask(taskID, action) {
    sendJSON('/api/tasks/' + taskID + '/' + action, 'POST', {}, function () {
        if (loadPlanDetailFn) loadPlanDetailFn(currentPlanID);
    });
}

function renderDraftMode(plan) {
    var container = document.createElement('div');

    var sectionTitle = document.createElement('div');
    sectionTitle.className = 'section-title';
    sectionTitle.textContent = t('proposedTasks');
    container.appendChild(sectionTitle);

    if (plan.tasks && plan.tasks.length > 0) {
        for (var i = 0; i < plan.tasks.length; i++) {
            var task = plan.tasks[i];
            var row = document.createElement('div');
            row.className = 'task-row';

            var info = document.createElement('div');
            var tTitle = document.createElement('span');
            tTitle.textContent = task.title;
            tTitle.style.fontWeight = '600';

            var editTaskBtn = document.createElement('button');
            editTaskBtn.className = 'btn-edit';
            editTaskBtn.textContent = t('rename');
            editTaskBtn.title = t('renameTaskTitle');
            (function (tID, tTitleVal) {
                editTaskBtn.addEventListener('click', function (e) {
                    e.stopPropagation();
                    showInputModal(t('renameTaskTitle'), t('enterTaskTitle'), tTitleVal, function (newTitle) {
                        if (!newTitle || newTitle === tTitleVal) return;
                        fetch('/api/tasks/' + encodeURIComponent(tID) + '/title', {
                            method: 'POST',
                            headers: { 'Content-Type': 'application/json' },
                            body: JSON.stringify({ title: newTitle })
                        })
                            .then(function (res) { if (!res.ok) throw new Error('HTTP ' + res.status); return res.json(); })
                            .then(function () {
                                if (loadPlanDetailFn) loadPlanDetailFn(currentPlanID);
                            })
                            .catch(function (err) { alert('Task rename error: ' + err.message); });
                    }, true);
                });
            })(task.id, task.title);

            var tTag = document.createElement('span');
            tTag.className = 'tag ' + task.status;
            tTag.textContent = statusLabel(task.status, 'task');
            tTag.style.marginLeft = '0.5rem';

            info.appendChild(tTitle);
            info.appendChild(editTaskBtn);
            info.appendChild(tTag);
            row.appendChild(info);

            if (task.status === 'needs_approval') {
                var actions = document.createElement('div');
                actions.className = 'task-row-actions';

                var approveBtn = document.createElement('button');
                approveBtn.className = 'task-row-btn approve';
                approveBtn.textContent = t('approve');
                approveBtn.addEventListener('click', function (tid) {
                    return function () {
                        showConfirmModal(t('approve'), t('confirmApproveTask'), function () {
                            approveOrRejectTask(tid, 'approve');
                        }, false);
                    };
                }(task.id));
                actions.appendChild(approveBtn);

                var rejectBtn = document.createElement('button');
                rejectBtn.className = 'task-row-btn reject';
                rejectBtn.textContent = t('reject');
                rejectBtn.addEventListener('click', function (tid) {
                    return function () {
                        showConfirmModal(t('reject'), t('confirmRejectTask'), function () {
                            approveOrRejectTask(tid, 'reject');
                        }, true);
                    };
                }(task.id));
                actions.appendChild(rejectBtn);

                row.appendChild(actions);
            }

            row.appendChild(createTaskCommentsSection(task, function () {
                if (loadPlanDetailFn) loadPlanDetailFn(currentPlanID);
            }));
            container.appendChild(row);
        }
    } else {
        var empty = document.createElement('div');
        empty.className = 'loading';
        empty.textContent = t('noProposedTasks');
        container.appendChild(empty);
    }

    // Comments
    var commentsContainer = document.createElement('div');
    commentsContainer.className = 'comments-section';
    commentsContainer.id = 'comments-plan-' + plan.id;
    container.appendChild(commentsContainer);

    container.appendChild(createCommentForm('plan', plan.id, function () {
        loadComments('plan', plan.id, commentsContainer, null);
    }));

    // Action buttons
    var actionsDiv = document.createElement('div');
    actionsDiv.className = 'plan-actions';

    if (plan.status === 'draft') {
        var approvePlan = document.createElement('button');
        approvePlan.className = 'btn btn-success';
        approvePlan.textContent = t('approvePlan');
        approvePlan.addEventListener('click', function () {
            showConfirmModal(t('approvePlan'), t('confirmApprovePlan'), function () {
                planAction(plan.id, 'approve');
            }, false);
        });
        actionsDiv.appendChild(approvePlan);

        var rejectPlan = document.createElement('button');
        rejectPlan.className = 'btn btn-danger';
        rejectPlan.textContent = t('rejectPlan');
        rejectPlan.addEventListener('click', function () {
            showConfirmModal(t('rejectPlan'), t('confirmRejectPlan'), function () {
                planAction(plan.id, 'reject');
            }, true);
        });
        actionsDiv.appendChild(rejectPlan);
    } else if (plan.status === 'rejected') {
        var reDraft = document.createElement('button');
        reDraft.className = 'btn btn-primary ui-btn';
        reDraft.textContent = t('resumePlan');
        reDraft.addEventListener('click', function () {
            showConfirmModal(t('resumePlan'), t('confirmResumePlan'), function () {
                planAction(plan.id, 'draft');
            }, false);
        });
        actionsDiv.appendChild(reDraft);
    }

    container.appendChild(actionsDiv);
    planDetailContent.appendChild(container);
    loadComments('plan', plan.id, commentsContainer, null);
}

function renderReviewMode(plan) {
    var container = document.createElement('div');
    container.className = 'split-view';

    // Left panel
    var left = document.createElement('div');
    left.className = 'walkthrough-panel';

    var leftTitle = document.createElement('div');
    leftTitle.className = 'panel-title';
    leftTitle.textContent = 'Walkthrough';
    left.appendChild(leftTitle);

    if (plan.walkthroughs && plan.walkthroughs.length > 0) {
        var last = plan.walkthroughs[0];
        var meta = document.createElement('div');
        meta.className = 'walkthrough-meta';
        meta.textContent = 'Git: ' + last.git_commit_hash;
        left.appendChild(meta);

        var wtDate = document.createElement('div');
        wtDate.className = 'wt-date';
        wtDate.style.marginBottom = '0.5rem';
        wtDate.textContent = t('created') + ' ' + formatDate(last.created_at);
        left.appendChild(wtDate);

        var summary = document.createElement('div');
        summary.className = 'walkthrough-summary';
        summary.textContent = last.summary_notes;
        left.appendChild(summary);
    }

    var taskTitle = document.createElement('div');
    taskTitle.className = 'section-title';
    taskTitle.textContent = t('proposedTasks');
    left.appendChild(taskTitle);

    if (plan.tasks && plan.tasks.length > 0) {
        for (var i = 0; i < plan.tasks.length; i++) {
            var task = plan.tasks[i];
            var taskRow = document.createElement('div');
            taskRow.className = 'task-row';

            var tInfo = document.createElement('div');
            var tName = document.createElement('span');
            tName.textContent = task.title;
            tName.style.fontWeight = '500';
            var tTag = document.createElement('span');
            tTag.className = 'tag ' + task.status;
            tTag.textContent = statusLabel(task.status, 'task');
            tTag.style.marginLeft = '0.5rem';

            tInfo.appendChild(tName);
            tInfo.appendChild(tTag);
            taskRow.appendChild(tInfo);
            left.appendChild(taskRow);
        }
    }

    container.appendChild(left);

    // Right panel
    var right = document.createElement('div');
    right.className = 'review-panel';

    var rightTitle = document.createElement('div');
    rightTitle.className = 'panel-title';
    rightTitle.textContent = t('statusReview');
    right.appendChild(rightTitle);

    if (plan.tasks && plan.tasks.length > 0) {
        for (var j = 0; j < plan.tasks.length; j++) {
            var rt = plan.tasks[j];
            var rtRow = document.createElement('div');
            rtRow.className = 'review-task-row';

            var rtTitle = document.createElement('div');
            rtTitle.className = 'review-task-title';
            rtTitle.textContent = rt.title;
            rtRow.appendChild(rtTitle);

            var rtActions = document.createElement('div');
            rtActions.className = 'review-task-actions';

            var addCommentBtn = document.createElement('button');
            addCommentBtn.className = 'btn btn-muted';
            addCommentBtn.textContent = t('addComment');
            addCommentBtn.style.fontSize = '0.8rem';
            addCommentBtn.style.padding = '0.3rem 0.6rem';
            addCommentBtn.addEventListener('click', function (taskID, taskTitleVal) {
                return function () { showQuickAction(taskID, taskTitleVal, 'comment', plan.id); };
            }(rt.id, rt.title));
            rtActions.appendChild(addCommentBtn);

            var reportBtn = document.createElement('button');
            reportBtn.className = 'btn btn-danger';
            reportBtn.textContent = t('reportIssue');
            reportBtn.style.fontSize = '0.8rem';
            reportBtn.style.padding = '0.3rem 0.6rem';
            reportBtn.addEventListener('click', function (taskID, taskTitleVal) {
                return function () { showQuickAction(taskID, taskTitleVal, 'issue', plan.id); };
            }(rt.id, rt.title));
            rtActions.appendChild(reportBtn);

            rtRow.appendChild(rtActions);
            right.appendChild(rtRow);
        }
    }

    var acceptBtn = document.createElement('button');
    acceptBtn.className = 'btn btn-success';
    acceptBtn.textContent = t('acceptWalkthrough');
    acceptBtn.style.marginTop = '1rem';
    acceptBtn.style.width = '100%';
    acceptBtn.addEventListener('click', function () {
        showConfirmModal(t('acceptWalkthrough'), t('confirmCompleteWalkthrough'), function () {
            planAction(plan.id, 'complete');
        }, false);
    });
    right.appendChild(acceptBtn);

    container.appendChild(right);
    planDetailContent.appendChild(container);
}

function renderDefaultPlanView(plan) {
    var container = document.createElement('div');

    var tasks = plan.tasks || [];
    var doneCount = 0;
    var total = tasks.length;
    for (var i = 0; i < tasks.length; i++) {
        if (tasks[i].status === 'done') doneCount++;
    }
    if (total > 0) {
        var pct = Math.round((doneCount / total) * 100);
        var meta = document.createElement('div');
        meta.className = 'plan-card-meta';
        meta.style.marginBottom = '0.5rem';
        meta.textContent = t('tasksProgress') + ' ' + doneCount + '/' + total + ' (' + pct + '%)';
        container.appendChild(meta);

        var bar = document.createElement('div');
        bar.className = 'progress-bar';
        bar.style.marginBottom = '1rem';
        var fill = document.createElement('div');
        fill.className = 'progress-fill';
        fill.style.width = pct + '%';
        bar.appendChild(fill);
        container.appendChild(bar);
    }

    if (tasks.length > 0) {
        var st = document.createElement('div');
        st.className = 'section-title';
        st.textContent = t('tasksProgress');
        container.appendChild(st);

        for (var j = 0; j < tasks.length; j++) {
            container.appendChild(createTaskCard(tasks[j], function () {
                if (loadPlanDetailFn) loadPlanDetailFn(currentPlanID);
            }));
        }
    }

    if (plan.status === 'approved' || plan.status === 'in_progress' || plan.status === 'on_hold') {
        var actions = document.createElement('div');
        actions.className = 'plan-actions';

        if (plan.status === 'in_progress') {
            var holdBtn = document.createElement('button');
            holdBtn.className = 'btn btn-warning';
            holdBtn.textContent = t('holdPlan');
            holdBtn.addEventListener('click', function () {
                showConfirmModal(t('holdPlan'), t('confirmHoldPlan'), function () {
                    planAction(plan.id, 'hold');
                }, true);
            });
            actions.appendChild(holdBtn);
        }

        if (plan.status === 'on_hold') {
            var resumeBtn = document.createElement('button');
            resumeBtn.className = 'btn btn-primary';
            resumeBtn.textContent = t('resumePlan');
            resumeBtn.addEventListener('click', function () {
                showConfirmModal(t('resumePlan'), t('confirmResumePlan'), function () {
                    planAction(plan.id, 'resume');
                }, false);
            });
            actions.appendChild(resumeBtn);
        }

        var cancelBtn = document.createElement('button');
        cancelBtn.className = 'btn btn-danger';
        cancelBtn.textContent = t('cancelPlan');
        cancelBtn.addEventListener('click', function () {
            showConfirmModal(t('cancelPlan'), t('confirmCancelPlan'), function () {
                planAction(plan.id, 'cancel');
            }, true);
        });
        actions.appendChild(cancelBtn);

        container.appendChild(actions);
    }

    var commentsContainer = document.createElement('div');
    commentsContainer.className = 'comments-section';
    commentsContainer.id = 'comments-plan-' + plan.id;
    container.appendChild(commentsContainer);

    container.appendChild(createCommentForm('plan', plan.id, function () {
        loadComments('plan', plan.id, commentsContainer, null);
    }));

    planDetailContent.appendChild(container);
    loadComments('plan', plan.id, commentsContainer, null);
}

function showQuickAction(taskID, taskTitleVal, type, planID) {
    var existing = document.querySelectorAll('.quick-action-form');
    for (var i = 0; i < existing.length; i++) {
        existing[i].parentNode.removeChild(existing[i]);
    }

    var rows = document.querySelectorAll('.review-task-row');
    var targetRow = null;
    for (var r = 0; r < rows.length; r++) {
        var rowTitle = rows[r].querySelector('.review-task-title');
        if (rowTitle && rowTitle.textContent === taskTitleVal) {
            targetRow = rows[r];
            break;
        }
    }
    if (!targetRow) return;

    var form = document.createElement('div');
    form.className = 'quick-action-form';

    var label = document.createElement('div');
    label.style.fontSize = '0.85rem';
    label.style.marginBottom = '0.4rem';
    label.style.color = 'var(--muted-color)';
    label.textContent = type === 'comment' ? (t('addComment') + ' "' + taskTitleVal + '"') : (t('reportIssue') + ' "' + taskTitleVal + '"');
    form.appendChild(label);

    var textarea = document.createElement('textarea');
    textarea.placeholder = type === 'comment' ? t('commentTextPlaceholder') : t('issueDescPlaceholder');
    form.appendChild(textarea);

    var submitBtn = document.createElement('button');
    submitBtn.className = 'btn ' + (type === 'comment' ? 'btn-primary' : 'btn-danger');
    submitBtn.textContent = type === 'comment' ? t('send') : t('reportIssue');
    submitBtn.style.fontSize = '0.8rem';
    submitBtn.addEventListener('click', function () {
        var text = textarea.value.trim();
        if (!text) return;

        var endpoint = '/api/walkthroughs/' + planID + '/' + (type === 'comment' ? 'add-comment' : 'report-issue');
        var body = { task_id: taskID };
        if (type === 'comment') {
            body.text = text;
        } else {
            body.description = text;
        }

        sendJSON(endpoint, 'POST', body, function () {
            form.parentNode.removeChild(form);
            if (loadPlanDetailFn) loadPlanDetailFn(planID);
        });
    });
    form.appendChild(submitBtn);

    var cancelBtn = document.createElement('button');
    cancelBtn.className = 'btn btn-muted';
    cancelBtn.textContent = t('cancel');
    cancelBtn.style.fontSize = '0.8rem';
    cancelBtn.style.marginLeft = '0.4rem';
    cancelBtn.addEventListener('click', function () {
        form.parentNode.removeChild(form);
    });
    form.appendChild(cancelBtn);

    targetRow.parentNode.insertBefore(form, targetRow.nextSibling);
    textarea.focus();
}
