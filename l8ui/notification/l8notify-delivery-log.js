(function() {
    'use strict';

    const col = Layer8ColumnFactory;
    const enums = L8NotifyEnums;

    window.L8NotifyDeliveryLog = {
        getColumns: function(options) {
            const cols = [];
            cols.push(...col.date('sentAt', 'Timestamp'));
            if (!options || options.showChannel !== false) {
                cols.push(...col.enum('channel', 'Channel', null, enums.render.channel));
            }
            if (!options || options.showTarget !== false) {
                cols.push(...col.col('endpoint', 'Endpoint'));
            }
            cols.push(...col.status('status', 'Status', null, enums.render.deliveryStatus));
            cols.push(...col.number('httpStatus', 'HTTP Status'));
            cols.push(...col.number('attempt', 'Attempt'));
            cols.push(...col.col('errorMessage', 'Error'));
            return cols;
        },

        render: function(container, logs, options) {
            if (!container) return;

            const opts = options || {};
            const columns = L8NotifyDeliveryLog.getColumns(opts);
            const tableId = 'l8notify-delivery-log-' + Date.now();

            container.innerHTML = `
                <div class="l8notify-delivery-log">
                    <div id="${tableId}"></div>
                </div>
            `;

            const tableContainer = container.querySelector('#' + tableId);
            if (tableContainer) {
                const table = new Layer8DTable(tableContainer, {
                    columns: columns,
                    data: logs || [],
                    readOnly: true,
                    pageSize: opts.pageSize || 25,
                    onRowClick: function(item) {
                        L8NotifyDeliveryLog._showDetail(item);
                    }
                });
                table.render();
            }
        },

        _showDetail: function(result) {
            const statusLabel = L8NotifyEnums.DELIVERY_STATUS.enum[result.status] || 'Unknown';
            const channelLabel = L8NotifyEnums.NOTIFY_CHANNEL.enum[result.channel] || '';

            let content = `
                <div class="l8notify-delivery-detail">
                    <div class="l8notify-detail-row">
                        <label>Status</label>
                        <span>${statusLabel}</span>
                    </div>
            `;

            if (channelLabel) {
                content += `
                    <div class="l8notify-detail-row">
                        <label>Channel</label>
                        <span>${channelLabel}</span>
                    </div>
                `;
            }

            if (result.endpoint) {
                content += `
                    <div class="l8notify-detail-row">
                        <label>Endpoint</label>
                        <span>${result.endpoint}</span>
                    </div>
                `;
            }

            content += `
                    <div class="l8notify-detail-row">
                        <label>HTTP Status</label>
                        <span>${result.httpStatus || 'N/A'}</span>
                    </div>
                    <div class="l8notify-detail-row">
                        <label>Attempt</label>
                        <span>${result.attempt || 1}</span>
                    </div>
            `;

            if (result.errorMessage) {
                content += `
                    <div class="l8notify-detail-row l8notify-detail-error">
                        <label>Error</label>
                        <span>${result.errorMessage}</span>
                    </div>
                `;
            }

            if (result.sentAt) {
                const date = new Date(result.sentAt * 1000);
                content += `
                    <div class="l8notify-detail-row">
                        <label>Sent At</label>
                        <span>${date.toLocaleString()}</span>
                    </div>
                `;
            }

            content += '</div>';

            Layer8DPopup.show({
                title: 'Delivery Details',
                html: content,
                readOnly: true
            });
        }
    };
})();
