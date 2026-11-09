//go:build sdksmoke

// Command smoke runs the Resend app emulator's email flows through the
// official resend-go SDK (v4.7.0, pinned by resend-go.mod) against a running
// binary. It is test-only and sits outside the repository's Go module: run.sh
// copies it next to resend-go.mod and resend-go.sum and runs it.
//
//	RESEND_EMULATOR_URL=http://localhost:4113 go run -tags sdksmoke .
//
// Prints one JSON line per case ({"sdk","case","pass","detail"}) with the same
// case names as python/smoke.py and node/smoke.mjs. resend-go types to as a
// list and reads no tags back, so those two cases report pass: null (not
// applicable). Errors from resend-go carry the message only, not the status.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/resend/resend-go/v4"
)

const (
	sdk    = "resend-go v4.7.0"
	sender = "onboarding@resend.dev"
)

var (
	base    = strings.TrimRight(os.Getenv("RESEND_EMULATOR_URL"), "/")
	ctx     = context.Background()
	client  = newClient()
	results []map[string]any
)

func newClient() *resend.Client {
	c := resend.NewClient("re_smoke_key")
	u, err := url.Parse(base + "/")
	if err != nil {
		panic(err)
	}
	c.BaseURL = u
	return c
}

// check runs one case on a freshly reset emulator, as the Python and Node
// suites do.
func check(name string, fn func() error) {
	reset()
	err := fn()
	r := map[string]any{"sdk": sdk, "case": name, "pass": err == nil, "detail": ""}
	if err != nil {
		r["detail"] = err.Error()
	}
	results = append(results, r)
}

func notApplicable(name, why string) {
	results = append(results, map[string]any{"sdk": sdk, "case": name, "pass": nil, "detail": why})
}

func email(subject string, to ...string) *resend.SendEmailRequest {
	return &resend.SendEmailRequest{From: sender, To: to, Subject: subject, Html: "<p>" + subject + "</p>"}
}

func wantErr(err error, substr string) error {
	if err == nil {
		return errors.New("no error")
	}
	if !strings.Contains(err.Error(), substr) {
		return fmt.Errorf("error %q does not mention %q", err, substr)
	}
	return nil
}

func reset() {
	req, _ := http.NewRequest(http.MethodPost, base+"/admin/reset", nil)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		resp.Body.Close()
	}
}

func main() {
	reset()

	check("send with an array recipient", func() error {
		sent, err := client.Emails.Send(email("smoke array", "delivered@resend.dev"))
		if err != nil {
			return err
		}
		if sent.Id == "" {
			return errors.New("no id")
		}
		return nil
	})

	check("send, then get by id", func() error {
		sent, err := client.Emails.Send(email("smoke get", "delivered@resend.dev"))
		if err != nil {
			return err
		}
		got, err := client.Emails.Get(sent.Id)
		if err != nil {
			return err
		}
		if got.Id != sent.Id || got.Subject != "smoke get" {
			return fmt.Errorf("got id %q subject %q", got.Id, got.Subject)
		}
		return nil
	})

	check("get an unknown id answers not_found", func() error {
		_, err := client.Emails.Get("0199a000-0000-7000-8000-000000000000")
		return wantErr(err, "not found")
	})

	notApplicable("send with a string recipient", "resend-go types to as []string")
	notApplicable("send with tags, then get them back", "resend-go's Email has no tags field")

	check("get returns Resend's read fields", func() error {
		sent, err := client.Emails.Send(email("fields", "delivered@resend.dev"))
		if err != nil {
			return err
		}
		got, err := client.Emails.Get(sent.Id)
		if err != nil {
			return err
		}
		switch {
		case got.Object != "email":
			return fmt.Errorf("object is %q", got.Object)
		case !strings.HasSuffix(got.MessageId, "@email.amazonses.com>"):
			return fmt.Errorf("message_id is %q", got.MessageId)
		case got.Text != "fields":
			return fmt.Errorf("text is %q", got.Text)
		case got.LastEvent != "delivered":
			return fmt.Errorf("last_event is %q", got.LastEvent)
		}
		if _, err := time.Parse("2006-01-02 15:04:05.000000+00", got.CreatedAt); err != nil {
			return fmt.Errorf("created_at %q: %w", got.CreatedAt, err)
		}
		return nil
	})

	batchItems := func() []*resend.SendEmailRequest {
		bad := email("smoke batch", "delivered@resend.dev")
		bad.From = ""
		return []*resend.SendEmailRequest{email("smoke batch", "delivered@resend.dev"), bad}
	}

	check("batch: strict fails on an invalid item", func() error {
		_, err := client.Batch.Send(batchItems())
		return wantErr(err, "Missing `from` field.")
	})

	check("batch: permissive sends the valid items", func() error {
		got, err := client.Batch.SendWithOptions(ctx, batchItems(),
			&resend.BatchSendEmailOptions{BatchValidation: resend.BatchValidationPermissive})
		if err != nil {
			return err
		}
		if len(got.Data) != 1 || len(got.Errors) != 1 || got.Errors[0].Index != 1 {
			return fmt.Errorf("got data %v errors %v", got.Data, got.Errors)
		}
		return nil
	})

	check("idempotent retry returns the first send", func() error {
		opts := &resend.SendEmailOptions{IdempotencyKey: fmt.Sprintf("smoke-go-%d", time.Now().UnixNano())}
		first, err := client.Emails.SendWithOptions(ctx, email("smoke idem", "delivered@resend.dev"), opts)
		if err != nil {
			return err
		}
		again, err := client.Emails.SendWithOptions(ctx, email("smoke idem", "delivered@resend.dev"), opts)
		if err != nil {
			return err
		}
		if again.Id != first.Id {
			return fmt.Errorf("ids differ: %s vs %s", first.Id, again.Id)
		}
		_, err = client.Emails.SendWithOptions(ctx, email("changed", "delivered@resend.dev"), opts)
		return wantErr(err, "request body was modified")
	})

	check("a send to bounced@resend.dev reads back bounced", func() error {
		sent, err := client.Emails.Send(email("smoke bounce", "bounced@resend.dev"))
		if err != nil {
			return err
		}
		got, err := client.Emails.Get(sent.Id)
		if err != nil {
			return err
		}
		if got.LastEvent != "bounced" {
			return fmt.Errorf("last_event is %q", got.LastEvent)
		}
		return nil
	})

	check("a send from an unverified domain answers 403", func() error {
		if _, err := client.Domains.Create(&resend.CreateDomainRequest{Name: "smoke-go-unverified.acme.dev"}); err != nil {
			return err
		}
		req := email("smoke domain", "delivered@resend.dev")
		req.From = "a@smoke-go-unverified.acme.dev"
		_, err := client.Emails.Send(req)
		return wantErr(err, "domain is not verified")
	})

	check("create and get a domain", func() error {
		created, err := client.Domains.Create(&resend.CreateDomainRequest{Name: "smoke-go-shape.acme.dev"})
		if err != nil {
			return err
		}
		if len(created.Records) != 4 || created.Records[0].Name != "resend._domainkey" || created.Status != "not_started" {
			return fmt.Errorf("create is %+v", created)
		}
		got, err := client.Domains.Get(created.Id)
		if err != nil {
			return err
		}
		if got.Name != created.Name || len(got.Records) != 4 || got.Capabilities == nil || got.Capabilities.Sending != "enabled" {
			return fmt.Errorf("get is %+v", got)
		}
		return nil
	})

	check("update, verify and delete a domain", func() error {
		created, err := client.Domains.Create(&resend.CreateDomainRequest{Name: "smoke-go-update.acme.dev"})
		if err != nil {
			return err
		}
		if _, err := client.Domains.Update(created.Id, &resend.UpdateDomainRequest{OpenTracking: true, Tls: resend.Enforced}); err != nil {
			return err
		}
		got, err := client.Domains.Get(created.Id)
		if err != nil {
			return err
		}
		if !got.OpenTracking || got.ClickTracking {
			return fmt.Errorf("after update: %+v", got)
		}
		if ok, err := client.Domains.Verify(created.Id); err != nil || !ok {
			return fmt.Errorf("verify: %v %v", ok, err)
		}
		if ok, err := client.Domains.Remove(created.Id); err != nil || !ok {
			return fmt.Errorf("remove: %v %v", ok, err)
		}
		_, err = client.Domains.Get(created.Id)
		return wantErr(err, "Domain not found")
	})

	check("create, rename and delete an API key", func() error {
		created, err := client.ApiKeys.Create(&resend.CreateApiKeyRequest{Name: "smoke-go-key"})
		if err != nil {
			return err
		}
		renamed, err := client.ApiKeys.Update(created.Id, &resend.UpdateApiKeyRequest{Name: "smoke-go-key-renamed"})
		if err != nil || renamed.Object != "api_key" || renamed.Id != created.Id {
			return fmt.Errorf("update: %+v %v", renamed, err)
		}
		keys, err := client.ApiKeys.List()
		if err != nil || len(keys.Data) != 1 || keys.Data[0].Name != "smoke-go-key-renamed" {
			return fmt.Errorf("list: %+v %v", keys, err)
		}
		if ok, err := client.ApiKeys.Remove(created.Id); err != nil || !ok {
			return fmt.Errorf("remove: %v %v", ok, err)
		}
		return nil
	})

	check("register a webhook and read its secret", func() error {
		created, err := client.Webhooks.Create(&resend.CreateWebhookRequest{
			Endpoint: "https://example.com/smoke-go-hook", Events: []string{"email.sent", "email.delivered"},
		})
		if err != nil {
			return err
		}
		if created.Object != "webhook" || !strings.HasPrefix(created.SigningSecret, "whsec_") {
			return fmt.Errorf("create is %+v", created)
		}
		got, err := client.Webhooks.Get(created.Id)
		if err != nil {
			return err
		}
		if got.Status != "enabled" || got.SigningSecret != created.SigningSecret || len(got.Events) != 2 {
			return fmt.Errorf("get is %+v", got)
		}
		return nil
	})

	check("disable, change and delete a webhook", func() error {
		created, err := client.Webhooks.Create(&resend.CreateWebhookRequest{
			Endpoint: "https://example.com/smoke-go-hook", Events: []string{"email.sent"},
		})
		if err != nil {
			return err
		}
		disabled, endpoint := "disabled", "https://example.com/smoke-go-hook-2"
		updated, err := client.Webhooks.Update(created.Id, &resend.UpdateWebhookRequest{
			Status: &disabled, Endpoint: &endpoint, Events: []string{"email.bounced"},
		})
		if err != nil || updated.Id != created.Id {
			return fmt.Errorf("update: %+v %v", updated, err)
		}
		got, err := client.Webhooks.Get(created.Id)
		if err != nil || got.Status != "disabled" || got.Endpoint != endpoint || len(got.Events) != 1 {
			return fmt.Errorf("get: %+v %v", got, err)
		}
		if _, err := client.Webhooks.Remove(created.Id); err != nil {
			return err
		}
		_, err = client.Webhooks.Get(created.Id)
		return wantErr(err, "Webhook endpoint not found")
	})

	check("verify a signed webhook delivery", func() error {
		got := make(chan *http.Request, 4)
		bodies := make(chan string, 4)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			bodies <- string(b)
			got <- r
		}))
		defer srv.Close()
		hook, err := client.Webhooks.Create(&resend.CreateWebhookRequest{Endpoint: srv.URL, Events: []string{"email.delivered"}})
		if err != nil {
			return err
		}
		if _, err := client.Emails.Send(email("smoke hook", "delivered@resend.dev")); err != nil {
			return err
		}
		select {
		case r := <-got:
			return client.Webhooks.Verify(&resend.VerifyWebhookOptions{
				Payload: <-bodies,
				Headers: resend.WebhookHeaders{
					Id: r.Header.Get("svix-id"), Timestamp: r.Header.Get("svix-timestamp"), Signature: r.Header.Get("svix-signature"),
				},
				WebhookSecret: hook.SigningSecret,
			})
		case <-time.After(5 * time.Second):
			return errors.New("no delivery within 5s")
		}
	})

	check("list domains with a limit pages newest first", func() error {
		first, err := client.Domains.Create(&resend.CreateDomainRequest{Name: "smoke-go-page-one.acme.dev"})
		if err != nil {
			return err
		}
		second, err := client.Domains.Create(&resend.CreateDomainRequest{Name: "smoke-go-page-two.acme.dev"})
		if err != nil {
			return err
		}
		one := 1
		page, err := client.Domains.ListWithOptions(context.Background(), &resend.ListOptions{Limit: &one})
		if err != nil {
			return err
		}
		if len(page.Data) != 1 || page.Data[0].Id != second.Id || !page.HasMore {
			return fmt.Errorf("page 1 is %+v", page)
		}
		page, err = client.Domains.ListWithOptions(context.Background(), &resend.ListOptions{Limit: &one, After: &second.Id})
		if err != nil {
			return err
		}
		if len(page.Data) != 1 || page.Data[0].Id != first.Id || page.HasMore {
			return fmt.Errorf("page 2 is %+v", page)
		}
		return nil
	})

	check("list sent emails newest first", func() error {
		first, err := client.Emails.Send(email("smoke list 1", "delivered@resend.dev"))
		if err != nil {
			return err
		}
		second, err := client.Emails.Send(email("smoke list 2", "delivered@resend.dev"))
		if err != nil {
			return err
		}
		page, err := client.Emails.List()
		if err != nil {
			return err
		}
		if len(page.Data) != 2 || page.Data[0].Id != second.Id || page.Data[1].Id != first.Id || page.HasMore {
			return fmt.Errorf("list is %+v", page)
		}
		one := 1
		page, err = client.Emails.ListWithOptions(context.Background(), &resend.ListOptions{Limit: &one})
		if err != nil {
			return err
		}
		if len(page.Data) != 1 || page.Data[0].Id != second.Id || !page.HasMore {
			return fmt.Errorf("limit 1 is %+v", page)
		}
		return nil
	})

	check("schedule a send in natural language", func() error {
		req := email("smoke later", "delivered@resend.dev")
		req.ScheduledAt = "in 1 hour"
		sent, err := client.Emails.Send(req)
		if err != nil {
			return err
		}
		got, err := client.Emails.Get(sent.Id)
		if err != nil {
			return err
		}
		// resend-go's Email type carries no scheduled_at field.
		if got.LastEvent != "scheduled" {
			return fmt.Errorf("last_event %q", got.LastEvent)
		}
		return nil
	})

	check("reschedule, then cancel a scheduled send", func() error {
		req := email("smoke move", "delivered@resend.dev")
		req.ScheduledAt = "in 1 hour"
		sent, err := client.Emails.Send(req)
		if err != nil {
			return err
		}
		moved, err := client.Emails.Update(&resend.UpdateEmailRequest{Id: sent.Id, ScheduledAt: "in 2 hours"})
		if err != nil {
			return err
		}
		if moved.Id != sent.Id || moved.Object != "email" {
			return fmt.Errorf("update answered %+v", moved)
		}
		cancelled, err := client.Emails.Cancel(sent.Id)
		if err != nil {
			return err
		}
		if cancelled.Id != sent.Id || cancelled.Object != "email" {
			return fmt.Errorf("cancel answered %+v", cancelled)
		}
		got, err := client.Emails.Get(sent.Id)
		if err != nil {
			return err
		}
		if got.LastEvent != "canceled" {
			return fmt.Errorf("last_event %q", got.LastEvent)
		}
		return nil
	})

	failed := false
	for _, r := range results {
		b, _ := json.Marshal(r)
		fmt.Println(string(b))
		if r["pass"] == false {
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}
