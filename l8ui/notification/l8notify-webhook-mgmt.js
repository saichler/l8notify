(function() {
    'use strict';

    const col = Layer8ColumnFactory;

    function maskSecret(secret) {
        if (!secret) return '';
        if (secret.length <= 4) return '****';
        return '****' + secret.slice(-4);
    }

    window.L8NotifyWebhookMgmt = {
        getColumns: function() {
            return [
                ...col.col('url', 'URL'),
                ...col.custom('secret', 'Secret', function(item) {
                    return maskSecret(item.secret);
                }),
                ...col.number('retryCount', 'Retries'),
                ...col.number('timeoutMs', 'Timeout (ms)')
            ];
        },

        getFormDefinition: function() {
            const f = Layer8FormFactory;
            return f.form('Webhook Configuration', [
                f.section('Endpoint', [
                    ...f.text('url', 'Webhook URL', true),
                    ...f.password('secret', 'HMAC Secret')
                ]),
                f.section('Retry Settings', [
                    ...f.number('retryCount', 'Max Retries'),
                    ...f.number('timeoutMs', 'Timeout (ms)')
                ])
            ]);
        },

        render: function(container, webhooks, onSave, onDelete, onTest) {
            if (!container) return;

            const columns = L8NotifyWebhookMgmt.getColumns();
            const tableId = 'l8notify-webhook-table-' + Date.now();

            container.innerHTML = `
                <div class="l8notify-webhook-mgmt">
                    <div class="l8notify-webhook-toolbar">
                        <button type="button" class="layer8d-btn layer8d-btn-primary layer8d-btn-small l8notify-add-webhook-btn">
                            Add Webhook
                        </button>
                    </div>
                    <div id="${tableId}"></div>
                </div>
            `;

            const tableContainer = container.querySelector('#' + tableId);
            if (tableContainer && webhooks) {
                const table = new Layer8DTable(tableContainer, {
                    columns: columns,
                    data: webhooks,
                    readOnly: false,
                    onRowClick: function(item) {
                        L8NotifyWebhookMgmt._showEditForm(item, onSave, onDelete, onTest);
                    }
                });
                table.render();
            }

            const addBtn = container.querySelector('.l8notify-add-webhook-btn');
            if (addBtn && onSave) {
                addBtn.addEventListener('click', function() {
                    L8NotifyWebhookMgmt._showEditForm(
                        { retryCount: 3, timeoutMs: 5000 },
                        onSave, onDelete, onTest
                    );
                });
            }
        },

        _showEditForm: function(webhook, onSave, onDelete, onTest) {
            const form = L8NotifyWebhookMgmt.getFormDefinition();
            const buttons = [];

            if (onTest) {
                buttons.push({
                    label: 'Test',
                    className: 'layer8d-btn layer8d-btn-secondary layer8d-btn-small',
                    onClick: function() { onTest(webhook); }
                });
            }

            if (onDelete && webhook.url) {
                buttons.push({
                    label: 'Delete',
                    className: 'layer8d-btn layer8d-btn-secondary layer8d-btn-small',
                    onClick: function() {
                        onDelete(webhook);
                        Layer8DPopup.close();
                    }
                });
            }

            Layer8DPopup.show({
                title: webhook.url ? 'Edit Webhook' : 'Add Webhook',
                form: form,
                data: webhook,
                extraButtons: buttons,
                onSave: function(data) {
                    if (onSave) onSave(data);
                    Layer8DPopup.close();
                }
            });
        }
    };
})();
