(function() {
    'use strict';

    window.L8NotifyTargetEditor = {
        getInlineTableDef: function() {
            return {
                key: 'targets',
                label: 'Notification Targets',
                columns: [
                    { key: 'targetId', label: 'ID', type: 'text', hidden: true },
                    { key: 'channel', label: 'Channel', type: 'select',
                      options: L8NotifyEnums.NOTIFY_CHANNEL },
                    { key: 'endpoint', label: 'Endpoint', type: 'text' },
                    { key: 'template', label: 'Template', type: 'textarea' }
                ]
            };
        }
    };
})();
