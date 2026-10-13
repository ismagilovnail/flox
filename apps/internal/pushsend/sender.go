package pushsend

import (
	"context"
	"io"
	"net/http"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// WebPush sends with the Web Push protocol: payload encrypted for the
// subscription (RFC 8291), signed with the deployment's VAPID keys
// (RFC 8292).
type WebPush struct {
	// Client must dial through urlsafety: the endpoint came from a
	// browser (it is also checked against known push services when
	// stored, internal/push).
	Client                         *http.Client
	PublicKey, PrivateKey, Subject string
	// TTL is how long the push service holds a message for an offline
	// device.
	TTL int
}

var _ Sender = (*WebPush)(nil)

func (w *WebPush) Send(ctx context.Context, to Subscriber, payload []byte) (int, error) {
	resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: to.Endpoint,
		Keys:     webpush.Keys{P256dh: to.P256dh, Auth: to.Auth},
	}, &webpush.Options{
		HTTPClient:      w.Client,
		Subscriber:      w.Subject,
		VAPIDPublicKey:  w.PublicKey,
		VAPIDPrivateKey: w.PrivateKey,
		TTL:             w.TTL,
		Urgency:         webpush.UrgencyNormal,
	})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, nil
}
