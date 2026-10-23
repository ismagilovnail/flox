package main

import (
	"fmt"
	"math/rand/v2"
	"net/url"
	"strconv"
)

// traffic generates clicks shaped like paid social traffic: 70% Facebook
// and Instagram, 30% TikTok, mostly mobile, each with its own click id.
// IPs come from a pool of 300,000, so some visitors return, as they do.
// What matters for the cost is the bytes a row carries and how well they
// compress — long unique click ids, real user agents, few distinct
// campaign and ad ids — not where the clicks are routed.
type traffic struct {
	r          *rand.Rand
	ips        []string
	fbCampaign []string
	fbAdset    []string
	fbAd       []string
	ttCampaign []string
	ttAd       []string
}

// vegetaTarget is vegeta's JSON target format (-format=json).
type vegetaTarget struct {
	Method string              `json:"method"`
	URL    string              `json:"url"`
	Header map[string][]string `json:"header"`
}

func newTraffic(r *rand.Rand) *traffic {
	t := &traffic{r: r}
	t.ips = make([]string, 300_000)
	for i := range t.ips {
		t.ips[i] = t.publicIP()
	}
	digits := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = "238" + strconv.FormatInt(r.Int64N(1e14), 10)
		}
		return out
	}
	t.fbCampaign, t.fbAdset, t.fbAd = digits(20), digits(80), digits(300)
	t.ttCampaign, t.ttAd = digits(10), digits(120)
	return t
}

func (t *traffic) publicIP() string {
	if t.r.IntN(10) == 0 { // a tenth on IPv6
		return fmt.Sprintf("2a02:%x:%x:%x::%x", t.r.IntN(0xffff), t.r.IntN(0xffff), t.r.IntN(0xffff), t.r.IntN(0xffff))
	}
	for {
		a := 1 + t.r.IntN(223)
		if a == 10 || a == 127 || a == 100 || a == 169 || a == 172 || a == 192 {
			continue // private, loopback, CGNAT, link-local: all trusted as proxies
		}
		return fmt.Sprintf("%d.%d.%d.%d", a, t.r.IntN(256), t.r.IntN(256), 1+t.r.IntN(254))
	}
}

var userAgents = []string{
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_6_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 [FBAN/FBIOS;FBAV/478.0.0.40.103;FBBV/631528390;FBDV/iPhone15,2;FBMD/iPhone;FBSN/iOS;FBSV/17.6.1;FBSS/3;FBID/phone;FBLC/en_US;FBOP/5;FBRV/633417601]",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 Instagram 349.0.0.21.105 (iPhone14,5; iOS 18_0; en_US; en; scale=3.00; 1170x2532; 638437521)",
	"Mozilla/5.0 (Linux; Android 14; SM-A546B Build/UP1A.231005.007; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/129.0.6668.81 Mobile Safari/537.36 [FB_IAB/FB4A;FBAV/483.0.0.53.84;]",
	"Mozilla/5.0 (Linux; Android 13; Redmi Note 12 Build/TKQ1.221114.001; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/128.0.6613.146 Mobile Safari/537.36 Instagram 348.0.0.40.109 Android (33/13; 440dpi; 1080x2400; Xiaomi/Redmi; 23021RAAEG; topaz; qcom; en_US; 636282344)",
	"Mozilla/5.0 (Linux; Android 12; V2207 Build/SP1A.210812.003; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/127.0.6533.103 Mobile Safari/537.36 musical_ly_2023605030 JsSdk/1.0 NetType/WIFI Channel/googleplay AppName/musical_ly app_version/36.5.3 ByteLocale/en ByteFullLocale/en Region/BR Spark/1.6.0.6-bugfix AppVersion/36.5.3 BytedanceWebview/d8a21c6",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 musical_ly_36.4.0 JsSdk/2.0 NetType/4G Channel/App Store ByteLocale/pt Region/BR isDarkMode/1 Spark/1.6.0.6-bugfix",
	"Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Mobile Safari/537.36",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.6 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Linux; Android 14; Pixel 7 Build/AP2A.240905.003; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/129.0.6668.70 Mobile Safari/537.36 [FB_IAB/FB4A;FBAV/484.0.0.62.109;]",
}

var languages = []string{"en-US,en;q=0.9", "pt-BR,pt;q=0.9", "es-MX,es;q=0.9", "de-DE,de;q=0.9,en;q=0.8", "ru-RU,ru;q=0.9", "tr-TR,tr;q=0.9", "id-ID,id;q=0.9"}
var placements = []string{"Facebook_Mobile_Feed", "Instagram_Feed", "Instagram_Stories", "Instagram_Reels", "Facebook_Reels", "an"}

const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"

func (t *traffic) token(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[t.r.IntN(len(alphabet))]
	}
	return string(b)
}

func (t *traffic) pick(s []string) string { return s[t.r.IntN(len(s))] }

func (t *traffic) next(target, host string) vegetaTarget {
	q := url.Values{}
	header := map[string][]string{
		"Host":            {host},
		"X-Forwarded-For": {t.pick(t.ips)},
		"User-Agent":      {t.pick(userAgents)},
		"Accept-Language": {t.pick(languages)},
	}
	if t.r.IntN(10) < 7 {
		camp := t.pick(t.fbCampaign)
		q.Set("fbclid", "IwZXh0bgNhZW0BMABhZGlkAa"+t.token(150))
		q.Set("utm_source", "facebook")
		q.Set("utm_medium", "paid")
		q.Set("utm_campaign", camp)
		q.Set("utm_content", t.pick(t.fbAd))
		q.Set("sub1", camp)
		q.Set("sub2", t.pick(t.fbAdset))
		q.Set("sub3", t.pick(t.fbAd))
		q.Set("sub4", t.pick(placements))
		q.Set("sub5", "fb")
		if t.r.IntN(2) == 0 {
			header["Referer"] = []string{"https://m.facebook.com/"}
		}
	} else {
		q.Set("ttclid", "E.C.P."+t.token(110))
		q.Set("utm_source", "tiktok")
		q.Set("sub1", t.pick(t.ttCampaign))
		q.Set("sub2", t.pick(t.ttAd))
		q.Set("sub3", "__CID_NAME__")
		q.Set("sub4", "PLACEMENT_TIKTOK")
	}
	return vegetaTarget{Method: "GET", URL: target + "?" + q.Encode(), Header: header}
}
