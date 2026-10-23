package mailoutbox

import (
	"context"
	"errors"
	"net/textproto"
	"time"
)

type Worker struct {
	Repo   Repository
	Sender Sender
	// Notify is signalled when mail is enqueued, by EnqueueAccountMailTx through the
	// process's notification listener. Losing one costs latency up to SafetyInterval, never
	// a delivery, so nothing here depends on it arriving.
	Notify <-chan struct{}
	// SafetyInterval is the longest an idle lane sleeps. It used to be two seconds, which on
	// an account service that sends a handful of emails a day was two queries a second
	// against an empty table, for ever.
	SafetyInterval time.Duration
}

// mailBusyInterval keeps a lane that just sent something looking again promptly: a queue with
// work in it should drain, and the two lanes bound how fast.
const mailBusyInterval = 100 * time.Millisecond

const defaultMailSafetyInterval = 30 * time.Second

// Run uses two bounded lanes and persistent leases. Startup does not send mail
// synchronously or scan the entire queue. No DB lock is held across SMTP I/O.
func (w *Worker) Run(ctx context.Context) {
	if w.Sender == nil {
		return
	}
	safety := w.SafetyInterval
	if safety <= 0 {
		safety = defaultMailSafetyInterval
	}
	done := make(chan struct{}, 2)
	for range 2 {
		go func() {
			defer func() { done <- struct{}{} }()
			timer := time.NewTimer(0)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-w.Notify:
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
				case <-timer.C:
				}
				delay := safety
				if w.Once(ctx) {
					delay = mailBusyInterval
				}
				timer.Reset(delay)
			}
		}()
	}
	<-done
	<-done
}
func (w *Worker) Once(ctx context.Context) bool {
	d, e := w.Repo.ClaimMail(ctx)
	if errors.Is(e, ErrQueueProgress) {
		return true
	}
	if e != nil || d == nil {
		return false
	}
	eligible, e := w.Repo.MailEligible(ctx, *d)
	if e != nil {
		w.finish(ctx, *d, "unavailable", false)
		return true
	}
	if !eligible {
		w.finish(ctx, *d, "ineligible", true)
		return true
	}
	raw, e := Render(*d, w.Sender.FromAddress(), w.Sender.ReplyToAddress())
	if e != nil {
		w.finish(ctx, *d, "invalid", true)
		return true
	}
	e = w.Sender.Send(ctx, d.Payload.To, raw)
	result := "sent"
	terminal := true
	if e != nil {
		result = "unavailable"
		terminal = false
		var protocol *textproto.Error
		if errors.As(e, &protocol) && protocol.Code >= 500 && protocol.Code < 600 {
			result = "rejected"
			terminal = true
		}
	}
	w.finish(ctx, *d, result, terminal)
	return true
}
func (w *Worker) finish(ctx context.Context, d Delivery, result string, terminal bool) {
	// Allow durable acknowledgement when shutdown cancels the delivery context.
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = w.Repo.FinishMail(finish, d, result, terminal)
}
