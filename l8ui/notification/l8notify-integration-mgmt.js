(function() {
    'use strict';

    const col = Layer8ColumnFactory;
    const f = Layer8FormFactory;
    const enums = L8NotifyEnums;

    // Data-only component: no render()/DOM handling. IntegrationConfig is a
    // real, editable Prime Object — consumer wires this into a standard
    // Layer8DTable + Layer8DForms CRUD flow against /<prefix>/78/IntegCfg,
    // modelName: 'IntegrationConfig'.
    window.L8NotifyIntegrationMgmt = {
        getColumns: function() {
            return [
                ...col.col('name', 'Name'),
                ...col.enum('type', 'Type', null, enums.render.integrationType),
                ...col.custom('hostOrUrl', 'Host / URL', (item) => item.host || item.url || '', { sortKey: 'host' }),
                ...col.col('credentialKey', 'Credential Key')
            ];
        },

        getFormDefinition: function() {
            return f.form('Integration Configuration', [
                f.section('Identity', [
                    ...f.text('name', 'Name', true),
                    ...f.select('type', 'Type', enums.INTEGRATION_TYPE.enum, true)
                ]),
                f.section('SMTP', [
                    ...f.text('host', 'Host'),
                    ...f.number('port', 'Port'),
                    ...f.checkbox('useTls', 'Use TLS'),
                    ...f.text('fromAddress', 'From Address'),
                    ...f.text('fromName', 'From Name')
                ]),
                f.section('Webhook / Slack', [
                    ...f.text('url', 'URL'),
                    ...f.number('retryCount', 'Retry Count'),
                    ...f.number('timeoutMs', 'Timeout (ms)')
                ]),
                f.section('Credentials', [
                    // Plain text, NOT a password field — this is a lookup key into the
                    // consumer's own security config JSON `credentials` map, not the
                    // secret value itself. The secret never leaves the ORM row's neighborhood.
                    ...f.text('credentialKey', 'Credential Key')
                ])
            ]);
        }
    };
})();
