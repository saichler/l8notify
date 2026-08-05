package channel

import (
	"fmt"
	ntf "github.com/saichler/l8types/go/types/l8notifysvc"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- Custom Sender Tests ---

type mockSender struct {
	lastEndpoint string
	lastMessage  string
	returnErr    error
}

func (m *mockSender) Send(endpoint, message string) (*ntf.DeliveryResult, error) {
	m.lastEndpoint = endpoint
	m.lastMessage = message
	if m.returnErr != nil {
		return nil, m.returnErr
	}
	return &ntf.DeliveryResult{
		Status:  ntf.DeliveryStatus_DELIVERY_STATUS_SENT,
		Attempt: 1,
	}, nil
}

func TestRegisterCustomSender_AndDispatch(t *testing.T) {
	sender := &mockSender{}
	RegisterCustomSender("test-sender", sender)
	defer func() {
		customMtx.Lock()
		delete(customSenders, "test-sender")
		customMtx.Unlock()
	}()

	target := &ntf.NotifyTarget{
		Channel:  ntf.NotifyChannel_NOTIFY_CHANNEL_CUSTOM,
		Endpoint: "custom://test",
	}

	result := Dispatch(target, "hello custom", nil, nil)
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_SENT {
		t.Errorf("expected SENT, got %v", result.Status)
	}
	if sender.lastEndpoint != "custom://test" {
		t.Errorf("expected endpoint 'custom://test', got %q", sender.lastEndpoint)
	}
	if sender.lastMessage != "hello custom" {
		t.Errorf("expected message 'hello custom', got %q", sender.lastMessage)
	}
}

func TestDispatchCustom_NoSenders(t *testing.T) {
	// Clear custom senders
	customMtx.Lock()
	saved := customSenders
	customSenders = make(map[string]Sender)
	customMtx.Unlock()
	defer func() {
		customMtx.Lock()
		customSenders = saved
		customMtx.Unlock()
	}()

	result := dispatchCustom("endpoint", "msg")
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_FAILED {
		t.Errorf("expected FAILED with no senders, got %v", result.Status)
	}
}

// --- Dispatch Routing Tests ---

func TestDispatch_NilTarget(t *testing.T) {
	result := Dispatch(nil, "msg", nil, nil)
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_FAILED {
		t.Errorf("expected FAILED for nil target, got %v", result.Status)
	}
	if result.ErrorMessage != "target is nil" {
		t.Errorf("unexpected error: %q", result.ErrorMessage)
	}
}

func TestDispatch_EmailNoSmtp(t *testing.T) {
	target := &ntf.NotifyTarget{
		Channel:  ntf.NotifyChannel_NOTIFY_CHANNEL_EMAIL,
		Endpoint: "user@example.com",
	}
	result := Dispatch(target, "test email", nil, nil)
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_FAILED {
		t.Errorf("expected FAILED without SMTP config, got %v", result.Status)
	}
}

func TestDispatch_PagerDutyNotImplemented(t *testing.T) {
	target := &ntf.NotifyTarget{
		Channel:  ntf.NotifyChannel_NOTIFY_CHANNEL_PAGERDUTY,
		Endpoint: "pagerduty-key",
	}
	result := Dispatch(target, "test", nil, nil)
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_FAILED {
		t.Errorf("expected FAILED for PagerDuty, got %v", result.Status)
	}
}

func TestDispatch_UnknownChannel(t *testing.T) {
	target := &ntf.NotifyTarget{
		Channel:  99,
		Endpoint: "somewhere",
	}
	result := Dispatch(target, "test", nil, nil)
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_FAILED {
		t.Errorf("expected FAILED for unknown channel, got %v", result.Status)
	}
}

// --- Webhook Tests with httptest ---

func TestDispatch_WebhookSimple(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(200)
	}))
	defer server.Close()

	target := &ntf.NotifyTarget{
		Channel:  ntf.NotifyChannel_NOTIFY_CHANNEL_WEBHOOK,
		Endpoint: server.URL,
	}
	result := Dispatch(target, "hello webhook", nil, nil)
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_SENT {
		t.Errorf("expected SENT, got %v: %s", result.Status, result.ErrorMessage)
	}
	if receivedBody == "" {
		t.Error("server received no body")
	}
}

func TestDispatch_WebhookWithHmac(t *testing.T) {
	var receivedSig string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get(signatureHeader)
		w.WriteHeader(200)
	}))
	defer server.Close()

	secrets := map[string]string{server.URL: "my-secret"}
	target := &ntf.NotifyTarget{
		Channel:  ntf.NotifyChannel_NOTIFY_CHANNEL_WEBHOOK,
		Endpoint: server.URL,
	}
	result := Dispatch(target, "signed message", nil, secrets)
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_SENT {
		t.Errorf("expected SENT, got %v: %s", result.Status, result.ErrorMessage)
	}
	if receivedSig == "" {
		t.Error("expected HMAC signature header, got none")
	}
}

func TestSendWebhook_RetryOnFailure(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	cfg := &ntf.WebhookConfig{
		Url:        server.URL,
		RetryCount: 3,
		TimeoutMs:  2000,
	}
	result := SendWebhook(cfg, "retry test")
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_SENT {
		t.Errorf("expected SENT after retries, got %v: %s", result.Status, result.ErrorMessage)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestSendWebhook_AllRetriesFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	cfg := &ntf.WebhookConfig{
		Url:        server.URL,
		RetryCount: 2,
		TimeoutMs:  1000,
	}
	result := SendWebhook(cfg, "fail test")
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_FAILED {
		t.Errorf("expected FAILED, got %v", result.Status)
	}
	if result.Attempt != 2 {
		t.Errorf("expected 2 attempts, got %d", result.Attempt)
	}
}

func TestSendWebhookSimple_EmptyEndpoint(t *testing.T) {
	result := SendWebhookSimple("", "msg")
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_FAILED {
		t.Errorf("expected FAILED for empty endpoint, got %v", result.Status)
	}
}

func TestSendWebhook_NilConfig(t *testing.T) {
	result := SendWebhook(nil, "msg")
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_FAILED {
		t.Errorf("expected FAILED for nil config, got %v", result.Status)
	}
}

// --- Slack Tests with httptest ---

func TestDispatch_Slack(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(200)
	}))
	defer server.Close()

	target := &ntf.NotifyTarget{
		Channel:  ntf.NotifyChannel_NOTIFY_CHANNEL_SLACK,
		Endpoint: server.URL,
	}
	result := Dispatch(target, "slack message", nil, nil)
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_SENT {
		t.Errorf("expected SENT, got %v: %s", result.Status, result.ErrorMessage)
	}
	if receivedBody == "" {
		t.Error("server received no body")
	}
}

func TestSendSlack_EmptyURL(t *testing.T) {
	result := SendSlack("", "msg")
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_FAILED {
		t.Errorf("expected FAILED for empty URL, got %v", result.Status)
	}
}

func TestSendSlack_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	result := SendSlack(server.URL, "error test")
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_FAILED {
		t.Errorf("expected FAILED, got %v", result.Status)
	}
	if result.HttpStatus != 500 {
		t.Errorf("expected HTTP 500, got %d", result.HttpStatus)
	}
}

// --- Email Error Path Tests ---

func TestSendEmail_NilConfig(t *testing.T) {
	result := SendEmail(nil, "user@test.com", "Subject", "Body")
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_FAILED {
		t.Errorf("expected FAILED for nil config, got %v", result.Status)
	}
}

func TestSendEmail_EmptyHost(t *testing.T) {
	cfg := &ntf.SmtpConfig{Host: "", Port: 587}
	result := SendEmail(cfg, "user@test.com", "Subject", "Body")
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_FAILED {
		t.Errorf("expected FAILED for empty host, got %v", result.Status)
	}
}

func TestSendEmail_EmptyRecipient(t *testing.T) {
	cfg := &ntf.SmtpConfig{Host: "smtp.test.com", Port: 587}
	result := SendEmail(cfg, "", "Subject", "Body")
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_FAILED {
		t.Errorf("expected FAILED for empty recipient, got %v", result.Status)
	}
}

func TestSendEmail_ConnectionFailure(t *testing.T) {
	cfg := &ntf.SmtpConfig{
		Host:     "localhost",
		Port:     19999, // no SMTP server here
		Username: "user",
		Password: "pass",
	}
	result := SendEmail(cfg, "user@test.com", "Subject", "Body")
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_FAILED {
		t.Errorf("expected FAILED for connection failure, got %v", result.Status)
	}
}

func TestSendEmail_TLSConnectionFailure(t *testing.T) {
	cfg := &ntf.SmtpConfig{
		Host:   "localhost",
		Port:   19999,
		UseTls: true,
	}
	result := SendEmail(cfg, "user@test.com", "Subject", "Body")
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_FAILED {
		t.Errorf("expected FAILED for TLS connection failure, got %v", result.Status)
	}
}

func TestBuildEmailMessage(t *testing.T) {
	msg := buildEmailMessage("Sender", "from@test.com", "to@test.com", "Test Subject", "<h1>Hello</h1>")
	s := string(msg)
	checks := []string{
		"From: Sender <from@test.com>",
		"To: to@test.com",
		"Subject: Test Subject",
		"Content-Type: text/html; charset=UTF-8",
		"<h1>Hello</h1>",
	}
	for _, check := range checks {
		if !contains(s, check) {
			t.Errorf("email message missing %q", check)
		}
	}
}

func TestSendEmail_FromAddressFallback(t *testing.T) {
	// When FromAddress is empty, should fall back to Username
	cfg := &ntf.SmtpConfig{
		Host:     "localhost",
		Port:     19999,
		Username: "fallback@test.com",
	}
	// Will fail on connection, but tests the fallback logic path
	result := SendEmail(cfg, "to@test.com", "Subject", "Body")
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_FAILED {
		t.Errorf("expected FAILED (connection), got %v", result.Status)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- HMAC Signature Verification ---

func TestWebhook_HmacSignaturePresent(t *testing.T) {
	var sig string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sig = r.Header.Get(signatureHeader)
		w.WriteHeader(200)
	}))
	defer server.Close()

	cfg := &ntf.WebhookConfig{
		Url:        server.URL,
		Secret:     "test-secret",
		RetryCount: 1,
		TimeoutMs:  2000,
	}
	result := SendWebhook(cfg, "signed payload")
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_SENT {
		t.Fatalf("expected SENT, got %v: %s", result.Status, result.ErrorMessage)
	}
	if sig == "" {
		t.Error("expected HMAC signature in header")
	}
	// Signature should be a hex string (64 chars for SHA-256)
	if len(sig) != 64 {
		t.Errorf("expected 64-char hex signature, got %d chars: %q", len(sig), sig)
	}
}

func TestWebhook_NoSignatureWithoutSecret(t *testing.T) {
	var sig string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sig = r.Header.Get(signatureHeader)
		w.WriteHeader(200)
	}))
	defer server.Close()

	cfg := &ntf.WebhookConfig{
		Url:        server.URL,
		Secret:     "",
		RetryCount: 1,
		TimeoutMs:  2000,
	}
	SendWebhook(cfg, "unsigned payload")
	if sig != "" {
		t.Errorf("expected no signature header without secret, got %q", sig)
	}
}

// --- Custom Sender Error Handling ---

func TestDispatchCustom_SenderReturnsError(t *testing.T) {
	sender := &mockSender{returnErr: fmt.Errorf("custom error")}
	RegisterCustomSender("error-sender", sender)
	defer func() {
		customMtx.Lock()
		delete(customSenders, "error-sender")
		customMtx.Unlock()
	}()

	// Clear other senders to isolate
	customMtx.Lock()
	saved := make(map[string]Sender)
	for k, v := range customSenders {
		saved[k] = v
	}
	customSenders = map[string]Sender{"error-sender": sender}
	customMtx.Unlock()
	defer func() {
		customMtx.Lock()
		customSenders = saved
		customMtx.Unlock()
	}()

	result := dispatchCustom("endpoint", "msg")
	if result.Status != ntf.DeliveryStatus_DELIVERY_STATUS_FAILED {
		t.Errorf("expected FAILED when sender returns error, got %v", result.Status)
	}
}
