// GoGet GoFmt GoBuildNull GoBuild

package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/shoce/tg"
)

const (
	SP = " "
	NL = "\n"

	CmdAdd    = "/add"
	CmdRemove = "/rm"
	CmdList   = "/list"

	TgApiUrlDefault        = "https://api.telegram.org"
	XmlDefaultSpaceDefault = "http://www.w3.org/2005/Atom"

	IntervalDefault             = 99 * time.Second
	TgGetUpdatesIntervalDefault = 59 * time.Second
	TgSendIntervalDefault       = 3 * time.Second
	FeedsCheckIntervalDefault   = 11 * time.Minute
)

type TgFeedConfig struct {
	YssUrl string `yaml:"-"`

	DEBUG bool `yaml:"DEBUG"`

	Interval             time.Duration `yaml:"Interval"`
	TgGetUpdatesInterval time.Duration `yaml:"TgGetUpdatesInterval"`
	TgSendInterval       time.Duration `yaml:"TgSendInterval"`

	TgApiUrl         string    `yaml:"TgApiUrl"`
	TgToken          string    `yaml:"TgToken"`
	TgBossChatId     string    `yaml:"TgBossChatId"`
	TgGetUpdatesLast time.Time `yaml:"TgGetUpdatesLast"`
	TgUpdatesOffset  int64     `yaml:"TgUpdatesOffset"`
	TgChatId         string    `yaml:"TgChatId"`

	XmlDefaultSpace string `yaml:"XmlDefaultSpace"`

	FeedsCheckInterval time.Duration `yaml:"FeedsCheckInterval"`
	FeedsCheckLast     time.Time     `yaml:"FeedsCheckLast"`

	FeedsUrls []string `yaml:"FeedsUrls"`
	// ( [https://github.com/golang/go/releases.atom] )
}

var (
	Config TgFeedConfig

	TZIST = time.FixedZone("IST", 330*60)

	Ctx context.Context

	HttpClient = &http.Client{}
)

func ConfigGet() error {
	if err := Config.Get(); err != nil {
		return fmt.Errorf("Config.Get %v", err)
	}

	if Config.DEBUG {
		perr("DEBUG <true>")
	}

	if Config.DEBUG {
		perr("Interval <%v>", Config.Interval)
	}
	if Config.Interval == 0 {
		Config.Interval = IntervalDefault
		perr("Interval <%v>", Config.Interval)
	}

	if Config.DEBUG {
		perr("TgApiUrl [%s]", Config.TgApiUrl)
	}
	if Config.TgApiUrl == "" {
		Config.TgApiUrl = TgApiUrlDefault
		perr("TgApiUrl [%s]", Config.TgApiUrl)
	}

	if Config.DEBUG {
		perr("XmlDefaultSpace [%s]", Config.XmlDefaultSpace)
	}
	if Config.XmlDefaultSpace == "" {
		Config.XmlDefaultSpace = XmlDefaultSpaceDefault
		perr("XmlDefaultSpace [%s]", Config.XmlDefaultSpace)
	}

	if Config.DEBUG {
		perr("TgGetUpdatesInterval <%v>", Config.TgGetUpdatesInterval)
	}
	if Config.TgGetUpdatesInterval == 0 {
		Config.TgGetUpdatesInterval = TgGetUpdatesIntervalDefault
		perr("TgGetUpdatesInterval <%v>", Config.TgGetUpdatesInterval)
	}

	if Config.DEBUG {
		perr("TgSendInterval <%v>", Config.TgSendInterval)
	}
	if Config.TgSendInterval == 0 {
		Config.TgSendInterval = TgSendIntervalDefault
		perr("TgSendInterval <%v>", Config.TgSendInterval)
	}

	if Config.TgToken == "" {
		return fmt.Errorf("TgToken empty")
	}

	tg.ApiToken = Config.TgToken

	if Config.TgBossChatId == "" {
		return fmt.Errorf("TgBossChatId empty")
	}

	if Config.TgChatId == "" {
		return fmt.Errorf("TgChatId empty")
	}

	if Config.DEBUG {
		perr("TgUpdatesOffset <%v>", Config.TgUpdatesOffset)
	}

	if Config.DEBUG {
		perr("FeedsCheckInterval <%v>", Config.FeedsCheckInterval)
	}
	if Config.FeedsCheckInterval == 0 {
		Config.FeedsCheckInterval = FeedsCheckIntervalDefault
		perr("FeedsCheckInterval <%v>", Config.FeedsCheckInterval)
	}

	if Config.DEBUG {
		perr("FeedsCheckLast <%v>", Config.FeedsCheckLast)
	}
	if Config.DEBUG {
		perr("FeedsUrls ( %s )", strings.Join(Config.FeedsUrls, SP))
	}

	return nil
}

func TgGetUpdates() error {
	if time.Since(Config.TgGetUpdatesLast) < Config.TgGetUpdatesInterval {
		return nil
	}

	uu, _, err := tg.GetUpdates(Config.TgUpdatesOffset + 1)
	if err != nil {
		perr("ERROR tg.GetUpdates %v", err)
	}
	Config.TgGetUpdatesLast = time.Now()

	for _, u := range uu {
		var m tg.Message
		if u.Message.MessageId != 0 {
			perr("Update <%d> Message %s", u.UpdateId, strings.ReplaceAll(tg.F("%+v", u.Message), NL, "<NL>"))
			m = u.Message
		} else if u.ChannelPost.MessageId != 0 {
			perr("Update <%d> ChannelPost %s", u.UpdateId, strings.ReplaceAll(tg.F("%+v", u.ChannelPost), NL, "<NL>"))
			m = u.ChannelPost
		} else {
			perr("Update <%d> %s", u.UpdateId, strings.ReplaceAll(tg.F("%+v", u), NL, "<NL>"))
		}

		if m.MessageId == 0 {
			perr("Update <%d> MessageId <0>", u.UpdateId)
			Config.TgUpdatesOffset = u.UpdateId
			continue
		}

		if tg.F("%d", m.Chat.Id) != Config.TgBossChatId {
			perr("Update <%d> not from TgBossChatId", u.UpdateId)
			Config.TgUpdatesOffset = u.UpdateId
			continue
		}

		if tgerr := tg.SetMessageReaction(tg.SetMessageReactionRequest{
			ChatId:    fmt.Sprintf("%d", m.Chat.Id),
			MessageId: m.MessageId,
			Reaction:  []tg.ReactionTypeEmoji{tg.ReactionTypeEmoji{Emoji: "👾"}},
		}); tgerr != nil {
			perr("ERROR tg.SetMessageReaction %v", tgerr)
		}

		mtext := strings.TrimSpace(m.Text)

		if mtff := strings.Fields(mtext); (len(mtff) == 2 && mtff[0] == CmdAdd && strings.HasPrefix(mtff[1], "https://")) || (len(mtff) == 1 && strings.HasPrefix(mtff[0], "https://")) {

			perr("ADD feed [%s]", mtext)
			Config.FeedsUrls = append(Config.FeedsUrls, mtext)

			FeedsUrlsMap := make(map[string]struct{}, len(Config.FeedsUrls))
			FeedsUrlsUniq := make([]string, 0, len(Config.FeedsUrls))
			for _, v := range Config.FeedsUrls {
				if _, ok := FeedsUrlsMap[v]; ok {
					continue
				}
				FeedsUrlsMap[v] = struct{}{}
				FeedsUrlsUniq = append(FeedsUrlsUniq, v)
			}
			Config.FeedsUrls = FeedsUrlsUniq

			if tgerr := tg.SetMessageReaction(tg.SetMessageReactionRequest{
				ChatId:    fmt.Sprintf("%d", m.Chat.Id),
				MessageId: m.MessageId,
				Reaction:  []tg.ReactionTypeEmoji{tg.ReactionTypeEmoji{Emoji: "👍"}},
			}); tgerr != nil {
				perr("ERROR tg.SetMessageReaction %v", tgerr)
			}

		} else if mtff := strings.Fields(mtext); len(mtff) == 2 && mtff[0] == CmdRemove && strings.HasPrefix(mtff[1], "https://") {

			perr("REMOVE feed [%s]", mtff[1])

			FeedsUrlsNew := make([]string, 0, len(Config.FeedsUrls))
			for _, v := range Config.FeedsUrls {
				if v == mtff[1] {
					continue
				}
				FeedsUrlsNew = append(FeedsUrlsNew, v)
			}
			Config.FeedsUrls = FeedsUrlsNew

			if tgerr := tg.SetMessageReaction(tg.SetMessageReactionRequest{
				ChatId:    fmt.Sprintf("%d", m.Chat.Id),
				MessageId: m.MessageId,
				Reaction:  []tg.ReactionTypeEmoji{tg.ReactionTypeEmoji{Emoji: "👍"}},
			}); tgerr != nil {
				perr("ERROR tg.SetMessageReaction %v", tgerr)
			}

		} else if mtext == CmdList {

			perr("LIST feeds")

			tgmsg := ""
			for _, f := range Config.FeedsUrls {
				tgmsg += tg.Esc(f) + NL
			}
			if tgmsg == "" {
				tgmsg = tg.Italic("no feeds")
			}
			if _, tgerr := tg.SendMessage(tg.SendMessageRequest{
				ChatId:             fmt.Sprintf("%d", m.Chat.Id),
				ReplyToMessageId:   m.MessageId,
				Text:               tgmsg,
				LinkPreviewOptions: tg.LinkPreviewOptions{IsDisabled: true},
			}); tgerr != nil {
				perr("ERROR tg.SendMessage %v", tgerr)
			}

			if tgerr := tg.SetMessageReaction(tg.SetMessageReactionRequest{
				ChatId:    fmt.Sprintf("%d", m.Chat.Id),
				MessageId: m.MessageId,
				Reaction:  []tg.ReactionTypeEmoji{tg.ReactionTypeEmoji{Emoji: "👍"}},
			}); tgerr != nil {
				perr("ERROR tg.SetMessageReaction %v", tgerr)
			}

		} else {

			if tgerr := tg.SetMessageReaction(tg.SetMessageReactionRequest{
				ChatId:    fmt.Sprintf("%d", m.Chat.Id),
				MessageId: m.MessageId,
				Reaction:  []tg.ReactionTypeEmoji{tg.ReactionTypeEmoji{Emoji: "🤷‍♂"}},
			}); tgerr != nil {
				perr("ERROR tg.SetMessageReaction %v", tgerr)
			}

		}

		Config.TgUpdatesOffset = u.UpdateId
	}

	if err := Config.Put(); err != nil {
		perr("ERROR Config.Put %v", err)
		return err
	}

	return nil
}

func init() {
	Ctx = context.TODO()

	if s := os.Getenv("YssUrl"); s != "" {
		Config.YssUrl = s
	}
	if Config.YssUrl == "" {
		perr("ERROR YssUrl empty")
		os.Exit(1)
	}

	if err := ConfigGet(); err != nil {
		perr("ERROR ConfigGet %v", err)
		os.Exit(1)
	}
}

func main() {
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)
	go func(sigterm chan os.Signal) {
		<-sigterm
		tglog("%s sigterm", os.Args[0])
		os.Exit(1)
	}(sigterm)

	for {
		t0 := time.Now()

		if err := ConfigGet(); err != nil {
			perr("ERROR ConfigGet %v", err)
			os.Exit(1)
		}

		if err := TgGetUpdates(); err != nil {
			perr("ERROR TgGetUpdates %v", err)
		}

		if err := FeedsCheck(); err != nil {
			tglog("ERROR FeedsCheck %v", err)
		}

		if dur := time.Now().Sub(t0); dur < Config.Interval {
			time.Sleep(Config.Interval - dur)
		}
	}
}

type Feed struct {
	Updated Time    `xml:"updated"`
	Title   string  `xml:"title"`
	Entries []Entry `xml:"entry"`
}

type Entry struct {
	Updated Time   `xml:"updated"`
	Title   string `xml:"title"`
	Link    Link   `xml:"link"`
}

type Link struct {
	Href string `xml:"href,attr"`
}

type Time struct {
	time.Time
}

func (t *Time) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var v string
	if err := d.DecodeElement(&v, &start); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

func FeedsCheck() error {
	if time.Since(Config.FeedsCheckLast) < Config.FeedsCheckInterval {
		return nil
	}

	for _, feedurl := range Config.FeedsUrls {
		if Config.DEBUG {
			perr("DEBUG url %s", feedurl)
		}

		resp, err := http.Get(feedurl)
		if err != nil {
			perr("ERROR %s %v", feedurl, err)
			continue
		}
		defer resp.Body.Close()

		decoder := xml.NewDecoder(resp.Body)
		decoder.DefaultSpace = Config.XmlDefaultSpace

		var feed Feed
		if err := decoder.Decode(&feed); err != nil {
			perr("ERROR %s xml decode %v", feedurl, err)
			continue
		}

		sort.Slice(feed.Entries, func(i, j int) bool {
			return feed.Entries[i].Updated.Time.Before(feed.Entries[j].Updated.Time)
		})

		for _, e := range feed.Entries {
			etitle := strings.TrimSpace(e.Title)

			if Config.DEBUG {
				perr("DEBUG url %s title [%s] updated <%s> link [%s]", feedurl, etitle, e.Updated.Time, e.Link.Href)
			}

			if e.Updated.Time.Before(Config.FeedsCheckLast) {
				continue
			}

			tgmsg := tg.Bold(tg.Link(
				fmt.Sprintf("%s • %s", feed.Title, e.Updated.Time.In(TZIST).Format("Jan/2 15:04")),
				e.Link.Href,
			)) + NL +
				tg.Esc(etitle)
			if Config.DEBUG {
				perr("DEBUG tgmsg [%s]", tgmsg)
			}
			if _, tgerr := tg.SendMessage(tg.SendMessageRequest{
				ChatId:             Config.TgChatId,
				Text:               tgmsg,
				LinkPreviewOptions: tg.LinkPreviewOptions{IsDisabled: true},
			}); tgerr != nil {
				return tgerr
			}

			time.Sleep(Config.TgSendInterval)
		}
	}

	Config.FeedsCheckLast = time.Now()
	if err := Config.Put(); err != nil {
		return fmt.Errorf("Config.Put %v", err)
	}

	return nil
}

func ts() string {
	tnow := time.Now().In(time.FixedZone("IST", 330*60))
	return fmt.Sprintf(
		"%d%02d%02d:%02d%02dॐ",
		tnow.Year()%1000, tnow.Month(), tnow.Day(),
		tnow.Hour(), tnow.Minute(),
	)
}

func perr(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, ts()+" "+msg+NL, args...)
}

func tglog(msg string, args ...interface{}) (err error) {
	perr(msg, args...)
	_, err = tg.SendMessage(tg.SendMessageRequest{
		ChatId: Config.TgBossChatId,
		Text:   tg.Esc(tg.F(msg, args...)),

		DisableNotification: true,
		LinkPreviewOptions:  tg.LinkPreviewOptions{IsDisabled: true},
	})
	return err
}

func (config *TgFeedConfig) Get() error {
	req, err := http.NewRequest(http.MethodGet, config.YssUrl, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("yss response status %s", resp.Status)
	}

	rbb, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(rbb, config); err != nil {
		return err
	}

	if Config.DEBUG {
		//perr("DEBUG Config.Get %+v", config)
	}

	return nil
}

func (config *TgFeedConfig) Put() error {
	if config.DEBUG {
		//perr("DEBUG Config.Put %s %+v", config.YssUrl, config)
	}

	rbb, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, config.YssUrl, bytes.NewBuffer(rbb))
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("yss response status %s", resp.Status)
	}

	return nil
}
