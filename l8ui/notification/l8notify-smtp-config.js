(function() {
    'use strict';

    const f = Layer8FormFactory;

    window.L8NotifySmtpConfig = {
        getFormDefinition: function() {
            return f.form('SMTP Configuration', [
                f.section('Connection', [
                    ...f.text('host', 'SMTP Host', true),
                    ...f.number('port', 'Port', true),
                    ...f.checkbox('useTls', 'Use TLS')
                ]),
                f.section('Authentication', [
                    ...f.text('username', 'Username'),
                    ...f.password('password', 'Password')
                ]),
                f.section('Sender', [
                    ...f.text('fromAddress', 'From Address', true),
                    ...f.text('fromName', 'From Name')
                ])
            ]);
        },

        render: function(container, cfg, onSave) {
            if (!container) return;

            const form = L8NotifySmtpConfig.getFormDefinition();
            const formHtml = Layer8DForms.generateFormHtml(form, cfg || {});

            container.innerHTML = `
                <div class="l8notify-smtp-config">
                    <form id="l8notify-smtp-form">${formHtml}</form>
                    <div class="l8notify-smtp-actions">
                        <button type="button" class="layer8d-btn layer8d-btn-secondary layer8d-btn-small l8notify-test-btn">
                            Send Test Email
                        </button>
                        <button type="button" class="layer8d-btn layer8d-btn-primary layer8d-btn-small l8notify-save-btn">
                            Save
                        </button>
                    </div>
                </div>
            `;

            const saveBtn = container.querySelector('.l8notify-save-btn');
            if (saveBtn && onSave) {
                saveBtn.addEventListener('click', function() {
                    const formEl = container.querySelector('#l8notify-smtp-form');
                    const data = Layer8DForms.collectFormData(formEl, form);
                    onSave(data);
                });
            }
        },

        sendTest: function(smtpConfig, testEmail, endpoint) {
            if (!endpoint) {
                Layer8DNotification.warning('No test endpoint configured');
                return;
            }
            const body = JSON.stringify({
                smtpConfig: smtpConfig,
                testEmail: testEmail
            });
            fetch(endpoint, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: body
            }).then(function(resp) {
                if (resp.ok) {
                    Layer8DNotification.success('Test email sent successfully');
                } else {
                    Layer8DNotification.error('Test email failed', ['Status: ' + resp.status]);
                }
            }).catch(function(err) {
                Layer8DNotification.error('Test email failed', [err.message]);
            });
        }
    };
})();
