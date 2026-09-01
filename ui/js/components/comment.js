// Comment components
import { t } from '../i18n.js';
import { setupInlineConfirmButton } from './modal.js';

export function formatDate(isoString) {
    if (!isoString) return '';
    var d = new Date(isoString);
    var dd = d.getDate();
    var mm = d.getMonth() + 1;
    var yyyy = d.getFullYear();
    var hh = d.getHours();
    var min = d.getMinutes();
    if (dd < 10) dd = '0' + dd;
    if (mm < 10) mm = '0' + mm;
    if (hh < 10) hh = '0' + hh;
    if (min < 10) min = '0' + min;
    return dd + '.' + mm + '.' + yyyy + ' ' + hh + ':' + min;
}

export function loadComments(entityType, entityID, container, onReload) {
    fetch('/api/comments?entity_type=' + encodeURIComponent(entityType) + '&entity_id=' + encodeURIComponent(entityID))
        .then(function (response) {
            if (!response.ok) throw new Error('HTTP ' + response.status);
            return response.json();
        })
        .then(function (comments) {
            container.innerHTML = '';
            if (!comments || comments.length === 0) {
                var empty = document.createElement('div');
                empty.style.color = 'var(--muted-color)';
                empty.style.fontSize = '0.85rem';
                empty.textContent = t('noComments');
                container.appendChild(empty);
                return;
            }
            for (var i = 0; i < comments.length; i++) {
                var item = document.createElement('div');
                item.className = 'comment-item';
                var author = document.createElement('div');
                author.className = 'comment-author';
                author.textContent = comments[i].author;
                var text = document.createElement('div');
                text.className = 'comment-text';
                text.textContent = comments[i].text;

                var delBtn = document.createElement('button');
                delBtn.className = 'btn-delete';
                delBtn.textContent = t('delete');
                delBtn.title = t('delete');
                (function (commentID, btnEl) {
                    setupInlineConfirmButton(btnEl, t('delete'), t('clickToConfirm'), function () {
                        fetch('/api/comments?id=' + encodeURIComponent(commentID), { method: 'DELETE' })
                            .then(function (res) {
                                if (!res.ok) throw new Error('HTTP ' + res.status);
                                return res.json();
                            })
                            .then(function () {
                                loadComments(entityType, entityID, container, onReload);
                            })
                            .catch(function (err) {
                                alert('Delete comment error: ' + err.message);
                            });
                    });
                })(comments[i].id, delBtn);

                item.appendChild(author);
                item.appendChild(text);
                item.appendChild(delBtn);
                container.appendChild(item);
            }
        })
        .catch(function () {});
}

export function createCommentForm(entityType, entityID, onSuccess) {
    var form = document.createElement('div');
    form.className = 'comment-form';

    var textarea = document.createElement('textarea');
    textarea.placeholder = t('addCommentPlaceholder');
    form.appendChild(textarea);

    var submitBtn = document.createElement('button');
    submitBtn.className = 'btn btn-primary';
    submitBtn.textContent = t('send');
    submitBtn.addEventListener('click', function () {
        var text = textarea.value.trim();
        if (!text) return;

        fetch('/api/comments', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ entity_type: entityType, entity_id: entityID, text: text })
        })
            .then(function (res) {
                if (!res.ok) throw new Error('HTTP ' + res.status);
                return res.json();
            })
            .then(function () {
                textarea.value = '';
                if (onSuccess) onSuccess();
            })
            .catch(function (err) { alert('Comment send error: ' + err.message); });
    });
    form.appendChild(submitBtn);

    return form;
}

export function createTaskCommentsSection(task, onReload) {
    var wrapper = document.createElement('div');
    wrapper.className = 'task-comments-wrapper';
    wrapper.addEventListener('click', function (e) { e.stopPropagation(); });

    var list = document.createElement('div');
    list.className = 'task-comments-list';

    if (task.comments && task.comments.length > 0) {
        for (var i = 0; i < task.comments.length; i++) {
            var c = task.comments[i];
            var bubble = document.createElement('div');
            bubble.className = 'task-comment-bubble';

            var author = document.createElement('span');
            author.className = 'task-comment-author';
            author.textContent = (c.author === 'ai' ? '🤖 AI' : '👤 ' + c.author) + ':';

            var text = document.createElement('span');
            text.className = 'task-comment-text';
            text.textContent = c.text;

            var date = document.createElement('span');
            date.className = 'task-comment-date';
            date.textContent = formatDate(c.created_at);

            var delBtn = document.createElement('button');
            delBtn.className = 'btn-delete';
            delBtn.textContent = t('delete');
            delBtn.title = t('delete');
            (function (commentID, btnEl) {
                setupInlineConfirmButton(btnEl, t('delete'), t('clickToConfirm'), function () {
                    fetch('/api/comments?id=' + encodeURIComponent(commentID), { method: 'DELETE' })
                        .then(function (res) {
                            if (!res.ok) throw new Error('HTTP ' + res.status);
                            return res.json();
                        })
                        .then(function () { if (onReload) onReload(); })
                        .catch(function (err) { alert('Comment delete error: ' + err.message); });
                });
            })(c.id, delBtn);

            bubble.appendChild(author);
            bubble.appendChild(text);
            bubble.appendChild(date);
            bubble.appendChild(delBtn);
            list.appendChild(bubble);
        }
    }
    wrapper.appendChild(list);

    var inputRow = document.createElement('div');
    inputRow.className = 'task-comment-input-row';

    var textarea = document.createElement('textarea');
    textarea.placeholder = t('commentTaskPlaceholder');

    var sendBtn = document.createElement('button');
    sendBtn.className = 'btn btn-primary';
    sendBtn.style.padding = '4px 12px';
    sendBtn.style.fontSize = '0.8rem';
    sendBtn.textContent = t('send');

    function sendComment() {
        var val = textarea.value.trim();
        if (!val) return;
        fetch('/api/comments', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ entity_type: 'task', entity_id: task.id, text: val })
        })
            .then(function (res) {
                if (!res.ok) throw new Error('HTTP ' + res.status);
                return res.json();
            })
            .then(function () {
                textarea.value = '';
                if (onReload) onReload();
            })
            .catch(function (err) { alert('Comment send error: ' + err.message); });
    }

    sendBtn.addEventListener('click', sendComment);

    inputRow.appendChild(textarea);
    inputRow.appendChild(sendBtn);
    wrapper.appendChild(inputRow);

    return wrapper;
}
