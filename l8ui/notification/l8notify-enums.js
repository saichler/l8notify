(function() {
    'use strict';

    const { createStatusRenderer, renderEnum } = Layer8DRenderers;

    const NOTIFY_CHANNEL = Layer8EnumFactory.create([
        ['Unspecified', null, ''],
        ['Email', 'email', ''],
        ['Webhook', 'webhook', ''],
        ['Slack', 'slack', ''],
        ['PagerDuty', 'pagerduty', ''],
        ['Custom', 'custom', '']
    ]);

    const DELIVERY_STATUS = Layer8EnumFactory.create([
        ['Unspecified', null, ''],
        ['Pending', 'pending', ''],
        ['Sent', 'sent', ''],
        ['Failed', 'failed', ''],
        ['Retrying', 'retrying', '']
    ]);

    const DELIVERY_STATUS_CLASSES = {
        0: '',
        1: 'status-warning',
        2: 'status-success',
        3: 'status-error',
        4: 'status-warning'
    };

    const INTEGRATION_TYPE = Layer8EnumFactory.create([
        ['Unspecified', null, ''],
        ['SMTP', 'smtp', ''],
        ['Webhook', 'webhook', ''],
        ['Slack', 'slack', ''],
        ['PagerDuty', 'pagerduty', ''],
        ['Custom', 'custom', '']
    ]);

    window.L8NotifyEnums = {
        NOTIFY_CHANNEL: NOTIFY_CHANNEL,
        DELIVERY_STATUS: DELIVERY_STATUS,
        INTEGRATION_TYPE: INTEGRATION_TYPE,
        render: {
            channel: (value) => renderEnum(value, NOTIFY_CHANNEL.enum),
            deliveryStatus: createStatusRenderer(DELIVERY_STATUS.enum, DELIVERY_STATUS_CLASSES),
            integrationType: (value) => renderEnum(value, INTEGRATION_TYPE.enum)
        }
    };
})();
