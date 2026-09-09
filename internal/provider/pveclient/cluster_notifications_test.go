// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestPveNotificationEndpointSendmailCRUD exercises the sendmail endpoint
// wire: POST create body, GET decode (hyphenated keys, boolish disable),
// PUT with the delete query parameter, and DELETE.
func TestPveNotificationEndpointSendmailCRUD(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/notifications/endpoints/sendmail":
			body, _ := io.ReadAll(r.Body)
			sent := string(body)
			for _, want := range []string{
				`"name":"mail1"`,
				`"mailto":["ops@example.com"]`,
				`"mailto-user":["root@pam"]`,
				`"from-address":"pve@example.com"`,
				`"author":"PVE"`,
				`"disable":true`,
			} {
				if !strings.Contains(sent, want) {
					t.Fatalf("create body missing %s: %s", want, sent)
				}
			}
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/notifications/endpoints/sendmail/mail1":
			_, _ = io.WriteString(w, `{"data":{"name":"mail1","from-address":"pve@example.com",`+
				`"mailto":["ops@example.com"],"mailto-user":["root@pam"],"author":"PVE",`+
				`"disable":1,"digest":"abc123"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/notifications/endpoints/sendmail/mail1":
			if got := r.URL.Query().Get("delete"); got != "author,comment" {
				t.Fatalf("update delete fields = %q, want author,comment", got)
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"from-address":"new@example.com"`) {
				t.Fatalf("update body missing from-address: %s", body)
			}
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/notifications/endpoints/sendmail/mail1":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	err := c.CreateNotificationEndpoint(ctx, NotificationEndpoint{
		Type:        NotificationEndpointTypeSendmail,
		Name:        "mail1",
		MailTo:      []string{"ops@example.com"},
		MailToUser:  []string{"root@pam"},
		FromAddress: "pve@example.com",
		Author:      "PVE",
		Disable:     HABoolPtr(true),
	})
	if err != nil {
		t.Fatalf("CreateNotificationEndpoint: %v", err)
	}

	ep, err := c.GetNotificationEndpoint(ctx, NotificationEndpointTypeSendmail, "mail1")
	if err != nil {
		t.Fatalf("GetNotificationEndpoint: %v", err)
	}
	if ep.Name != "mail1" || ep.FromAddress != "pve@example.com" || ep.Author != "PVE" {
		t.Fatalf("decoded endpoint = %+v", ep)
	}
	if len(ep.MailTo) != 1 || ep.MailTo[0] != "ops@example.com" {
		t.Fatalf("MailTo = %v", ep.MailTo)
	}
	if len(ep.MailToUser) != 1 || ep.MailToUser[0] != "root@pam" {
		t.Fatalf("MailToUser = %v", ep.MailToUser)
	}
	if ep.Disable == nil || !*ep.Disable {
		t.Fatalf("Disable = %v, want true (boolish 1)", ep.Disable)
	}
	if ep.Digest != "abc123" {
		t.Fatalf("Digest = %q", ep.Digest)
	}

	err = c.UpdateNotificationEndpoint(ctx, NotificationEndpoint{
		Type:        NotificationEndpointTypeSendmail,
		Name:        "mail1",
		FromAddress: "new@example.com",
	}, []string{"author", "comment"})
	if err != nil {
		t.Fatalf("UpdateNotificationEndpoint: %v", err)
	}

	if err := c.DeleteNotificationEndpoint(ctx, NotificationEndpointTypeSendmail, "mail1"); err != nil {
		t.Fatalf("DeleteNotificationEndpoint: %v", err)
	}
}

// TestPveNotificationEndpointGotifyCRUD exercises the gotify endpoint wire,
// including the write-only token on create and its omission on update.
func TestPveNotificationEndpointGotifyCRUD(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/notifications/endpoints/gotify":
			body, _ := io.ReadAll(r.Body)
			sent := string(body)
			if !strings.Contains(sent, `"name":"got1"`) || !strings.Contains(sent, `"server":"https://gotify.example.com"`) || !strings.Contains(sent, `"token":"secret-token"`) {
				t.Fatalf("create body = %s", sent)
			}
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/notifications/endpoints/gotify/got1":
			_, _ = io.WriteString(w, `{"data":{"name":"got1","server":"https://gotify.example.com","disable":false,"comment":"ci alerts"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/notifications/endpoints/gotify/got1":
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), "token") {
				t.Fatalf("update must omit the write-only token when unset: %s", body)
			}
			if !strings.Contains(string(body), `"server":"https://gotify2.example.com"`) {
				t.Fatalf("update body = %s", body)
			}
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/notifications/endpoints/gotify/got1":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	if err := c.CreateNotificationEndpoint(ctx, NotificationEndpoint{
		Type:   NotificationEndpointTypeGotify,
		Name:   "got1",
		Server: "https://gotify.example.com",
		Token:  "secret-token",
	}); err != nil {
		t.Fatalf("CreateNotificationEndpoint: %v", err)
	}

	ep, err := c.GetNotificationEndpoint(ctx, NotificationEndpointTypeGotify, "got1")
	if err != nil {
		t.Fatalf("GetNotificationEndpoint: %v", err)
	}
	if ep.Server != "https://gotify.example.com" || ep.Token != "" {
		t.Fatalf("decoded endpoint = %+v", ep)
	}
	if ep.Disable == nil || *ep.Disable {
		t.Fatalf("Disable = %v, want false", ep.Disable)
	}
	if ep.Comment != "ci alerts" {
		t.Fatalf("Comment = %q", ep.Comment)
	}

	if err := c.UpdateNotificationEndpoint(ctx, NotificationEndpoint{
		Type:   NotificationEndpointTypeGotify,
		Name:   "got1",
		Server: "https://gotify2.example.com",
	}, nil); err != nil {
		t.Fatalf("UpdateNotificationEndpoint: %v", err)
	}
	if err := c.DeleteNotificationEndpoint(ctx, NotificationEndpointTypeGotify, "got1"); err != nil {
		t.Fatalf("DeleteNotificationEndpoint: %v", err)
	}
}

// TestPveNotificationEndpointSMTPCRUD exercises the smtp endpoint wire,
// including port/mode encoding and the absent password on decode.
func TestPveNotificationEndpointSMTPCRUD(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/notifications/endpoints/smtp":
			body, _ := io.ReadAll(r.Body)
			sent := string(body)
			for _, want := range []string{
				`"name":"smtp1"`,
				`"server":"smtp.example.com"`,
				`"from-address":"pve@example.com"`,
				`"username":"mailer"`,
				`"password":"pw"`,
				`"port":587`,
				`"mode":"starttls"`,
				`"mailto":["ops@example.com"]`,
			} {
				if !strings.Contains(sent, want) {
					t.Fatalf("create body missing %s: %s", want, sent)
				}
			}
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/notifications/endpoints/smtp/smtp1":
			_, _ = io.WriteString(w, `{"data":{"name":"smtp1","server":"smtp.example.com",`+
				`"from-address":"pve@example.com","username":"mailer","port":587,`+
				`"mode":"starttls","mailto":["ops@example.com"],"disable":0}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/notifications/endpoints/smtp/smtp1":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	if err := c.CreateNotificationEndpoint(ctx, NotificationEndpoint{
		Type:        NotificationEndpointTypeSMTP,
		Name:        "smtp1",
		Server:      "smtp.example.com",
		FromAddress: "pve@example.com",
		Username:    "mailer",
		Password:    "pw",
		Port:        HAInt64Ptr(587),
		Mode:        "starttls",
		MailTo:      []string{"ops@example.com"},
	}); err != nil {
		t.Fatalf("CreateNotificationEndpoint: %v", err)
	}

	ep, err := c.GetNotificationEndpoint(ctx, NotificationEndpointTypeSMTP, "smtp1")
	if err != nil {
		t.Fatalf("GetNotificationEndpoint: %v", err)
	}
	if ep.Server != "smtp.example.com" || ep.FromAddress != "pve@example.com" || ep.Username != "mailer" {
		t.Fatalf("decoded endpoint = %+v", ep)
	}
	if ep.Port == nil || *ep.Port != 587 {
		t.Fatalf("Port = %v, want 587", ep.Port)
	}
	if ep.Mode != "starttls" {
		t.Fatalf("Mode = %q", ep.Mode)
	}
	if ep.Password != "" {
		t.Fatalf("write-only password must not decode, got %q", ep.Password)
	}
	if ep.Disable == nil || *ep.Disable {
		t.Fatalf("Disable = %v, want false (boolish 0)", ep.Disable)
	}

	if err := c.DeleteNotificationEndpoint(ctx, NotificationEndpointTypeSMTP, "smtp1"); err != nil {
		t.Fatalf("DeleteNotificationEndpoint: %v", err)
	}
}

// TestPveNotificationEndpointWebhookCRUD exercises the webhook endpoint
// wire: base64 body and name=<n>,value=<base64> header/secret property
// strings on create, decoded back to plain values on read.
func TestPveNotificationEndpointWebhookCRUD(t *testing.T) {
	bodyB64 := base64.StdEncoding.EncodeToString([]byte(`{"text":"{{ title }}"}`))
	tokenB64 := base64.StdEncoding.EncodeToString([]byte("token-123"))
	secretB64 := base64.StdEncoding.EncodeToString([]byte("hmac-key"))
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/notifications/endpoints/webhook":
			raw, _ := io.ReadAll(r.Body)
			var sent map[string]any
			if err := json.Unmarshal(raw, &sent); err != nil {
				t.Fatalf("create body %q is not JSON: %v", raw, err)
			}
			if sent["name"] != "hook1" || sent["url"] != "https://hooks.example.com/pve" || sent["method"] != "post" {
				t.Fatalf("create body = %s", raw)
			}
			if sent["body"] != bodyB64 {
				t.Fatalf("body = %v, want base64 %s", sent["body"], bodyB64)
			}
			headers, ok := sent["header"].([]any)
			if !ok || len(headers) != 1 || headers[0] != "name=X-Token,value="+tokenB64 {
				t.Fatalf("header = %v, want [name=X-Token,value=%s]", sent["header"], tokenB64)
			}
			secrets, ok := sent["secret"].([]any)
			if !ok || len(secrets) != 1 || secrets[0] != "name=HMAC,value="+secretB64 {
				t.Fatalf("secret = %v, want [name=HMAC,value=%s]", sent["secret"], secretB64)
			}
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/notifications/endpoints/webhook/hook1":
			_, _ = io.WriteString(w, `{"data":{"name":"hook1","url":"https://hooks.example.com/pve",`+
				`"method":"put","body":"`+bodyB64+`",`+
				`"header":["name=X-Token,value=`+tokenB64+`"],"secret":["name=HMAC,value=`+secretB64+`"],`+
				`"disable":false}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/notifications/endpoints/webhook/hook1":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	if err := c.CreateNotificationEndpoint(ctx, NotificationEndpoint{
		Type:    NotificationEndpointTypeWebhook,
		Name:    "hook1",
		URL:     "https://hooks.example.com/pve",
		Method:  "post",
		Body:    `{"text":"{{ title }}"}`,
		Headers: map[string]string{"X-Token": "token-123"},
		Secrets: map[string]string{"HMAC": "hmac-key"},
	}); err != nil {
		t.Fatalf("CreateNotificationEndpoint: %v", err)
	}

	ep, err := c.GetNotificationEndpoint(ctx, NotificationEndpointTypeWebhook, "hook1")
	if err != nil {
		t.Fatalf("GetNotificationEndpoint: %v", err)
	}
	if ep.Method != "put" || ep.URL != "https://hooks.example.com/pve" {
		t.Fatalf("decoded endpoint = %+v", ep)
	}
	if ep.Body != `{"text":"{{ title }}"}` {
		t.Fatalf("Body = %q, want decoded plaintext", ep.Body)
	}
	if ep.Headers["X-Token"] != "token-123" {
		t.Fatalf("Headers = %v", ep.Headers)
	}
	if ep.Secrets["HMAC"] != "hmac-key" {
		t.Fatalf("Secrets = %v", ep.Secrets)
	}

	if err := c.DeleteNotificationEndpoint(ctx, NotificationEndpointTypeWebhook, "hook1"); err != nil {
		t.Fatalf("DeleteNotificationEndpoint: %v", err)
	}
}

// TestPveNotificationEndpointListAllTypes verifies ListNotificationEndpoints
// queries all four per-type collections and tags every entry with its type.
func TestPveNotificationEndpointListAllTypes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method %s", r.Method)
		}
		switch r.URL.Path {
		case "/cluster/notifications/endpoints/sendmail":
			_, _ = io.WriteString(w, `{"data":[{"name":"mail1","origin":"user-created"}]}`)
		case "/cluster/notifications/endpoints/gotify":
			_, _ = io.WriteString(w, `{"data":[{"name":"got1","server":"https://g"}]}`)
		case "/cluster/notifications/endpoints/smtp":
			_, _ = io.WriteString(w, `{"data":[]}`)
		case "/cluster/notifications/endpoints/webhook":
			_, _ = io.WriteString(w, `{"data":[{"name":"hook1","url":"https://h","method":"post"}]}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})
	list, err := c.ListNotificationEndpoints(context.Background())
	if err != nil {
		t.Fatalf("ListNotificationEndpoints: %v", err)
	}
	if len(list.Sendmail) != 1 || list.Sendmail[0].Name != "mail1" || list.Sendmail[0].Type != NotificationEndpointTypeSendmail {
		t.Fatalf("Sendmail list = %+v", list.Sendmail)
	}
	if len(list.Gotify) != 1 || list.Gotify[0].Server != "https://g" || list.Gotify[0].Type != NotificationEndpointTypeGotify {
		t.Fatalf("Gotify list = %+v", list.Gotify)
	}
	if len(list.SMTP) != 0 {
		t.Fatalf("SMTP list = %+v", list.SMTP)
	}
	if len(list.Webhook) != 1 || list.Webhook[0].Method != "post" || list.Webhook[0].Type != NotificationEndpointTypeWebhook {
		t.Fatalf("Webhook list = %+v", list.Webhook)
	}
}

// TestPveNotificationEndpointUnknownTypeRejected verifies unsupported
// endpoint types are rejected before any request is issued.
func TestPveNotificationEndpointUnknownTypeRejected(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("no request expected, got %s %s", r.Method, r.URL.Path)
	})
	ctx := context.Background()
	if err := c.CreateNotificationEndpoint(ctx, NotificationEndpoint{Type: "carrier-pigeon", Name: "x"}); err == nil {
		t.Fatal("expected error for unknown endpoint type")
	}
	if _, err := c.GetNotificationEndpoint(ctx, "carrier-pigeon", "x"); err == nil {
		t.Fatal("expected error for unknown endpoint type")
	}
	if err := c.DeleteNotificationEndpoint(ctx, "carrier-pigeon", "x"); err == nil {
		t.Fatal("expected error for unknown endpoint type")
	}
	if err := c.UpdateNotificationEndpoint(ctx, NotificationEndpoint{Type: "carrier-pigeon", Name: "x"}, nil); err == nil {
		t.Fatal("expected error for unknown endpoint type")
	}
}
