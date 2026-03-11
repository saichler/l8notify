package channel

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	ntf "github.com/saichler/l8notify/go/types/l8notify"
	"io"
	"net/http"
	"time"
)

const (
	defaultTimeout    = 5000
	defaultRetryCount = 3
	signatureHeader   = "X-L8-Signature"
)

// SendWebhook posts a JSON message to a webhook endpoint with HMAC signing and retry.
func SendWebhook(cfg *ntf.WebhookConfig, message string) *ntf.DeliveryResult {
	if cfg == nil || cfg.Url == "" {
		return &ntf.DeliveryResult{
			Status:       ntf.DeliveryStatus_DELIVERY_STATUS_FAILED,
			ErrorMessage: "webhook config or URL is empty",
			Attempt:      1,
			SentAt:       time.Now().Unix(),
		}
	}

	timeoutMs := cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultTimeout
	}
	retryCount := cfg.RetryCount
	if retryCount <= 0 {
		retryCount = defaultRetryCount
	}

	body := fmt.Sprintf(`{"message":%q}`, message)

	var lastErr error
	for attempt := int32(1); attempt <= retryCount; attempt++ {
		result := doWebhookPost(cfg.Url, body, cfg.Secret, timeoutMs, attempt)
		if result.Status == ntf.DeliveryStatus_DELIVERY_STATUS_SENT {
			return result
		}
		lastErr = fmt.Errorf("%s", result.ErrorMessage)

		if attempt < retryCount {
			// Exponential backoff: 1s, 2s, 4s...
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			time.Sleep(backoff)
		}
	}

	return &ntf.DeliveryResult{
		Status:       ntf.DeliveryStatus_DELIVERY_STATUS_FAILED,
		ErrorMessage: fmt.Sprintf("all %d attempts failed: %v", retryCount, lastErr),
		Attempt:      retryCount,
		SentAt:       time.Now().Unix(),
	}
}

// SendWebhookSimple posts a JSON message to a webhook endpoint without HMAC or retry.
func SendWebhookSimple(endpoint, message string) *ntf.DeliveryResult {
	if endpoint == "" {
		return &ntf.DeliveryResult{
			Status:       ntf.DeliveryStatus_DELIVERY_STATUS_FAILED,
			ErrorMessage: "endpoint is empty",
			Attempt:      1,
			SentAt:       time.Now().Unix(),
		}
	}

	body := fmt.Sprintf(`{"message":%q}`, message)
	return doWebhookPost(endpoint, body, "", defaultTimeout, 1)
}

func doWebhookPost(url, body, secret string, timeoutMs, attempt int32) *ntf.DeliveryResult {
	client := &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond}
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(body))
	if err != nil {
		return &ntf.DeliveryResult{
			Status:       ntf.DeliveryStatus_DELIVERY_STATUS_FAILED,
			ErrorMessage: fmt.Sprintf("failed to create request: %v", err),
			Attempt:      attempt,
			SentAt:       time.Now().Unix(),
		}
	}

	req.Header.Set("Content-Type", "application/json")

	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(body))
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set(signatureHeader, sig)
	}

	resp, err := client.Do(req)
	if err != nil {
		return &ntf.DeliveryResult{
			Status:       ntf.DeliveryStatus_DELIVERY_STATUS_FAILED,
			ErrorMessage: fmt.Sprintf("webhook request failed: %v", err),
			Attempt:      attempt,
			SentAt:       time.Now().Unix(),
		}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return &ntf.DeliveryResult{
			Status:     ntf.DeliveryStatus_DELIVERY_STATUS_SENT,
			HttpStatus: int32(resp.StatusCode),
			Attempt:    attempt,
			SentAt:     time.Now().Unix(),
		}
	}

	return &ntf.DeliveryResult{
		Status:       ntf.DeliveryStatus_DELIVERY_STATUS_FAILED,
		HttpStatus:   int32(resp.StatusCode),
		ErrorMessage: fmt.Sprintf("webhook returned status %d", resp.StatusCode),
		Attempt:      attempt,
		SentAt:       time.Now().Unix(),
	}
}
