package alert

import (
	"fmt"
	"strconv"
	"time"
)

// SlackText is the Slack message for one notification. The app renders the
// same params in the viewer's language; Slack gets English, since a
// channel has no single viewer.
func SlackText(kind Kind, p map[string]any) string {
	s := func(key string) string {
		if f, ok := p[key].(float64); ok {
			return strconv.FormatFloat(f, 'f', -1, 64)
		}
		return fmt.Sprint(p[key])
	}
	switch kind {
	case KindSpendOver:
		return fmt.Sprintf(":money_with_wings: Spend on *%s* today is $%s, over the $%s alert threshold.", s("campaignName"), s("spend"), s("threshold"))
	case KindROIBelow:
		return fmt.Sprintf(":chart_with_downwards_trend: ROI of *%s* today is %s%%, below %s%% (spend $%s, revenue $%s).", s("campaignName"), s("roi"), s("threshold"), s("spend"), s("revenue"))
	case KindOfferCapped:
		return fmt.Sprintf(":no_entry: Offer *%s* reached its daily cap (%s of %s); routing skips it until midnight.", s("offerName"), s("count"), s("cap"))
	case KindOfferPaused:
		return fmt.Sprintf(":double_vertical_bar: Offer *%s* was paused; traffic of the flows that use it goes to the fallback URL.", s("offerName"))
	case KindSSLExpiring:
		if expired, _ := p["expired"].(bool); expired {
			return fmt.Sprintf(":warning: The SSL certificate of *%s* has expired — tracking links on it fail in browsers.", s("domain"))
		}
		return fmt.Sprintf(":lock: The SSL certificate of *%s* expires in %s days (%s).", s("domain"), s("daysLeft"), dateOf(p["expiresAt"]))
	}
	return fmt.Sprintf("FLOX alert: %s", kind)
}

func dateOf(v any) string {
	switch t := v.(type) {
	case time.Time:
		return t.Format(time.DateOnly)
	case string:
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			return parsed.Format(time.DateOnly)
		}
		return t
	}
	return fmt.Sprint(v)
}
