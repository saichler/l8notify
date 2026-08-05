package channel

import (
	"fmt"
	ntf "github.com/saichler/l8types/go/types/l8notifysvc"
	"sync"
	"time"
)

// Sender delivers a message to an endpoint.
type Sender interface {
	Send(endpoint, message string) (*ntf.DeliveryResult, error)
}

var (
	customSenders = make(map[string]Sender)
	customMtx     sync.RWMutex
)

// RegisterCustomSender registers a Sender for NOTIFY_CHANNEL_CUSTOM.
// Consumer projects call this at startup to extend dispatch.
func RegisterCustomSender(name string, sender Sender) {
	customMtx.Lock()
	defer customMtx.Unlock()
	customSenders[name] = sender
}

// Dispatch sends a message to a NotifyTarget using the appropriate channel.
// smtpCfg may be nil if no SMTP is configured (email sends will fail gracefully).
// webhookSecrets maps endpoint URLs to HMAC secrets (may be nil).
func Dispatch(target *ntf.NotifyTarget, message string, smtpCfg *ntf.SmtpConfig,
	webhookSecrets map[string]string) *ntf.DeliveryResult {
	if target == nil {
		return &ntf.DeliveryResult{
			Status:       ntf.DeliveryStatus_DELIVERY_STATUS_FAILED,
			ErrorMessage: "target is nil",
			Attempt:      1,
			SentAt:       time.Now().Unix(),
		}
	}

	switch target.Channel {
	case ntf.NotifyChannel_NOTIFY_CHANNEL_WEBHOOK:
		secret := ""
		if webhookSecrets != nil {
			secret = webhookSecrets[target.Endpoint]
		}
		if secret != "" {
			return SendWebhook(&ntf.WebhookConfig{
				Url:    target.Endpoint,
				Secret: secret,
			}, message)
		}
		return SendWebhookSimple(target.Endpoint, message)

	case ntf.NotifyChannel_NOTIFY_CHANNEL_EMAIL:
		if smtpCfg == nil {
			return &ntf.DeliveryResult{
				Status:       ntf.DeliveryStatus_DELIVERY_STATUS_FAILED,
				ErrorMessage: "SMTP not configured",
				Attempt:      1,
				SentAt:       time.Now().Unix(),
			}
		}
		return SendEmail(smtpCfg, target.Endpoint, "Notification", message)

	case ntf.NotifyChannel_NOTIFY_CHANNEL_SLACK:
		return SendSlack(target.Endpoint, message)

	case ntf.NotifyChannel_NOTIFY_CHANNEL_PAGERDUTY:
		return &ntf.DeliveryResult{
			Status:       ntf.DeliveryStatus_DELIVERY_STATUS_FAILED,
			ErrorMessage: "PagerDuty channel not yet implemented",
			Attempt:      1,
			SentAt:       time.Now().Unix(),
		}

	case ntf.NotifyChannel_NOTIFY_CHANNEL_CUSTOM:
		return dispatchCustom(target.Endpoint, message)

	default:
		return &ntf.DeliveryResult{
			Status:       ntf.DeliveryStatus_DELIVERY_STATUS_FAILED,
			ErrorMessage: fmt.Sprintf("unknown channel: %s", target.Channel.String()),
			Attempt:      1,
			SentAt:       time.Now().Unix(),
		}
	}
}

func dispatchCustom(endpoint, message string) *ntf.DeliveryResult {
	customMtx.RLock()
	defer customMtx.RUnlock()

	for _, sender := range customSenders {
		result, err := sender.Send(endpoint, message)
		if err == nil && result != nil {
			return result
		}
	}

	return &ntf.DeliveryResult{
		Status:       ntf.DeliveryStatus_DELIVERY_STATUS_FAILED,
		ErrorMessage: "no custom sender handled the message",
		Attempt:      1,
		SentAt:       time.Now().Unix(),
	}
}
