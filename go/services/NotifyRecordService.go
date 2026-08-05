package services

import (
	"errors"
	"time"

	common "github.com/saichler/l8common/go/common"
	"github.com/saichler/l8notify/go/channel"
	"github.com/saichler/l8notify/go/template"
	"github.com/saichler/l8types/go/ifs"
	ntf "github.com/saichler/l8types/go/types/l8notifysvc"
)

// NotifyServiceName is the service name; NotifyServiceArea is declared in
// IntegrationConfigService.go (same package, shared by both services).
const NotifyServiceName = "Notify"

// L8Query `from` clause uses the protobuf type name "NotifyRecord", NOT the ServiceName "Notify".

// ActivateNotify activates the NotifyRecord delivery-log service. Unlike
// IntegrationConfig, NotifyRecord is immutable and actively dispatches
// (email/webhook/Slack) inside Before(POST) before persisting the outcome.
func ActivateNotify(creds, dbname string, vnic ifs.IVNic) {
	common.ActivateService(common.ServiceConfig{
		ServiceName: NotifyServiceName, ServiceArea: NotifyServiceArea,
		PrimaryKey: "NotifyId", Voter: true, NonUniqueKeys: []string{"RequestedAt"},
		Replication: boolPtr(false), Callback: &NotifyCallback{},
	}, &ntf.NotifyRecord{}, &ntf.NotifyRecordList{}, creds, dbname, vnic)
}

func boolPtr(b bool) *bool { return &b }

type NotifyCallback struct{}

func (this *NotifyCallback) Before(elem interface{}, action ifs.Action, isNotification bool, vnic ifs.IVNic) (interface{}, bool, error) {
	if action == ifs.GET {
		return nil, true, nil
	}
	record, ok := elem.(*ntf.NotifyRecord)
	if !ok {
		return nil, true, errors.New("invalid notify record type")
	}
	switch action {
	case ifs.POST:
		common.GenerateID(&record.NotifyId)
		record.RequestedAt = time.Now().Unix()
		if record.Status == ntf.DeliveryStatus_DELIVERY_STATUS_UNSPECIFIED {
			record.Status = ntf.DeliveryStatus_DELIVERY_STATUS_PENDING
		}
		if record.Template != "" {
			record.Message = template.Render(record.Template, record.Vars)
		}

		target := &ntf.NotifyTarget{Channel: record.Channel, Endpoint: record.Endpoint, Template: record.Template}
		smtpCfg := resolveSmtpConfig(vnic)
		webhookSecrets := resolveWebhookSecrets(vnic)
		result := channel.Dispatch(target, record.Message, smtpCfg, webhookSecrets)

		record.Status = result.Status
		record.HttpStatus = result.HttpStatus
		record.ErrorMessage = result.ErrorMessage
		record.Attempt = result.Attempt
		record.SentAt = result.SentAt
		return record, true, nil
	case ifs.PUT:
		return nil, true, errors.New("notify records are immutable, PUT is not allowed")
	case ifs.PATCH:
		return record, true, nil
	}
	return nil, true, nil
}

func (this *NotifyCallback) After(elem interface{}, action ifs.Action, notify bool, vnic ifs.IVNic) (interface{}, bool, error) {
	return nil, true, nil
}

// resolveSmtpConfig looks up the "smtp" IntegrationConfig and resolves its
// secret via the consumer's own credentials map. Credential's return order
// is (aside, zside, yside, name, err) — aside is discarded and zside/yside
// are used as username/password, matching the exact destructuring pattern
// l8common.ActivateService already uses for DB credentials (verified against
// OpenDBConection, which builds "user=%s password=%s" from those two
// positions). See the deviation note in this phase's completion report.
func resolveSmtpConfig(vnic ifs.IVNic) *ntf.SmtpConfig {
	cfg, err := vnic.Resources().Integration().GetIntegrationConfig("smtp")
	if err != nil || cfg == nil || cfg.Host == "" {
		return nil // not configured — channel.Dispatch already handles a nil SmtpConfig gracefully
	}
	user, pass := "", ""
	if cfg.CredentialKey != "" {
		_, credUser, credPass, _, cerr := vnic.Resources().Security().Credential(cfg.CredentialKey, "smtp", vnic.Resources())
		if cerr == nil {
			user, pass = credUser, credPass
		}
	}
	return &ntf.SmtpConfig{
		Host: cfg.Host, Port: cfg.Port, UseTls: cfg.UseTls,
		FromAddress: cfg.FromAddress, FromName: cfg.FromName,
		Username: user, Password: pass,
	}
}

// resolveWebhookSecrets looks up all webhook IntegrationConfigs and resolves
// each one's secret via the consumer's own credentials map, keyed by URL.
func resolveWebhookSecrets(vnic ifs.IVNic) map[string]string {
	configs, err := vnic.Resources().Integration().ListIntegrationConfigs(ntf.IntegrationType_INTEGRATION_TYPE_WEBHOOK)
	if err != nil {
		return nil
	}
	secrets := make(map[string]string, len(configs))
	for _, cfg := range configs {
		if cfg.CredentialKey == "" {
			continue
		}
		_, secret, _, _, cerr := vnic.Resources().Security().Credential(cfg.CredentialKey, "webhook", vnic.Resources())
		if cerr == nil {
			secrets[cfg.Url] = secret
		}
	}
	return secrets
}
