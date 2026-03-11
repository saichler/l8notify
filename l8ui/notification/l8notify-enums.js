(function() {
    'use strict';

    const { createStatusRenderer, renderEnum } = Layer8DRenderers;

    const NOTIFY_CHANNEL = Layer8EnumFactory.create([
        { value: 0, label: 'Unspecified' },
        { value: 1, label: 'Email' },
        { value: 2, label: 'Webhook' },
        { value: 3, label: 'Slack' },
        { value: 4, label: 'PagerDuty' },
        { value: 5, label: 'Custom' }
    ]);

    const DELIVERY_STATUS = Layer8EnumFactory.create([
        { value: 0, label: 'Unspecified' },
        { value: 1, label: 'Pending' },
        { value: 2, label: 'Sent' },
        { value: 3, label: 'Failed' },
        { value: 4, label: 'Retrying' }
    ]);

    const DELIVERY_STATUS_CLASSES = {
        0: '',
        1: 'status-warning',
        2: 'status-success',
        3: 'status-error',
        4: 'status-warning'
    };

    window.L8NotifyEnums = {
        NOTIFY_CHANNEL: NOTIFY_CHANNEL,
        DELIVERY_STATUS: DELIVERY_STATUS,
        render: {
            channel: (value) => renderEnum(value, NOTIFY_CHANNEL.enum),
            deliveryStatus: createStatusRenderer(DELIVERY_STATUS.enum, DELIVERY_STATUS_CLASSES)
        }
    };
})();
