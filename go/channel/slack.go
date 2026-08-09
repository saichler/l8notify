package channel

import (
	"bytes"
	"fmt"
	ntf "github.com/saichler/l8types/go/types/l8notify"
	"io"
	"net/http"
	"time"
)

// SendSlack posts a message to a Slack incoming webhook URL.
func SendSlack(webhookURL, message string) *ntf.DeliveryResult {
	if webhookURL == "" {
		return &ntf.DeliveryResult{
			Status:       ntf.DeliveryStatus_DELIVERY_STATUS_FAILED,
			ErrorMessage: "Slack webhook URL is empty",
			Attempt:      1,
			SentAt:       time.Now().Unix(),
		}
	}

	body := fmt.Sprintf(`{"text":%q}`, message)
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Post(webhookURL, "application/json", bytes.NewBufferString(body))
	if err != nil {
		return &ntf.DeliveryResult{
			Status:       ntf.DeliveryStatus_DELIVERY_STATUS_FAILED,
			ErrorMessage: fmt.Sprintf("Slack request failed: %v", err),
			Attempt:      1,
			SentAt:       time.Now().Unix(),
		}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return &ntf.DeliveryResult{
			Status:     ntf.DeliveryStatus_DELIVERY_STATUS_SENT,
			HttpStatus: int32(resp.StatusCode),
			Attempt:    1,
			SentAt:     time.Now().Unix(),
		}
	}

	return &ntf.DeliveryResult{
		Status:       ntf.DeliveryStatus_DELIVERY_STATUS_FAILED,
		HttpStatus:   int32(resp.StatusCode),
		ErrorMessage: fmt.Sprintf("Slack returned status %d", resp.StatusCode),
		Attempt:      1,
		SentAt:       time.Now().Unix(),
	}
}
