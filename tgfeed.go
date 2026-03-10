// GoGet GoFmt GoBuildNull

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

	// https://pkg.go.dev/github.com/goccy/go-yaml
	yaml "github.com/goccy/go-yaml"

	"github.com/shoce/tg"
)

const (
	SP = " "
	NL = "\n"

	TgCmdAddDefault    = "/add"
	TgCmdRemoveDefault = "/rem"
	TgCmdListDefault   = "/list"

	TgApiUrlDefault        = "https://api.telegram.org"
	XmlDefaultSpaceDefault = "http://www.w3.org/2005/Atom"

	IntervalDefault             = 99 * time.Second
	TgGetUpdatesIntervalDefault = 59 * time.Second
	TgSendIntervalDefault       = 3 * time.Second
	FeedsCheckIntervalDefault   = 11 * time.Minute

	FeedCheckLastAgoDefault = 240 * time.Hour
)

type Feed struct {
	Url       string    `yaml:"Url"`
	CheckLast time.Time `yaml:"CheckLast"`
}

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

	TgCmdAdd    string `yaml:"TgCmdAdd"`
	TgCmdRemove string `yaml:"TgCmdRemove"`
	TgCmdList   string `yaml:"TgCmdList"`

	XmlDefaultSpace string `yaml:"XmlDefaultSpace"`

	FeedsCheckInterval time.Duration `yaml:"FeedsCheckInterval"`
	FeedsCheckLast     time.Time     `yaml:"FeedsCheckLast"`

	Feeds []Feed `yaml:"Feeds"`
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

	perr("DEBUG Interval <%v>", Config.Interval)
	if Config.Interval == 0 {
		Config.Interval = IntervalDefault
		perr("Interval <%v>", Config.Interval)
	}

	perr("DEBUG TgApiUrl [%s]", Config.TgApiUrl)
	if Config.TgApiUrl == "" {
		Config.TgApiUrl = TgApiUrlDefault
		perr("TgApiUrl [%s]", Config.TgApiUrl)
	}

	perr("DEBUG XmlDefaultSpace [%s]", Config.XmlDefaultSpace)
	if Config.XmlDefaultSpace == "" {
		Config.XmlDefaultSpace = XmlDefaultSpaceDefault
		perr("XmlDefaultSpace [%s]", Config.XmlDefaultSpace)
	}

	perr("DEBUG TgGetUpdatesInterval <%v>", Config.TgGetUpdatesInterval)
	if Config.TgGetUpdatesInterval == 0 {
		Config.TgGetUpdatesInterval = TgGetUpdatesIntervalDefault
		perr("TgGetUpdatesInterval <%v>", Config.TgGetUpdatesInterval)
	}

	perr("DEBUG TgSendInterval <%v>", Config.TgSendInterval)
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

	if Config.TgCmdAdd == "" {
		Config.TgCmdAdd = TgCmdAddDefault
	}
	if Config.TgCmdRemove == "" {
		Config.TgCmdRemove = TgCmdRemoveDefault
	}
	if Config.TgCmdList == "" {
		Config.TgCmdList = TgCmdListDefault
	}

	perr("DEBUG TgUpdatesOffset <%v>", Config.TgUpdatesOffset)

	perr("DEBUG FeedsCheckInterval <%v>", Config.FeedsCheckInterval)
	if Config.FeedsCheckInterval == 0 {
		Config.FeedsCheckInterval = FeedsCheckIntervalDefault
		perr("FeedsCheckInterval <%v>", Config.FeedsCheckInterval)
	}

	perr("DEBUG FeedsCheckLast <%v>", fmttime(Config.FeedsCheckLast))
	feedsurls := make([]string, len(Config.Feeds))
	for i := range Config.Feeds {
		feedsurls[i] = "[" + Config.Feeds[i].Url + "]"
	}
	perr("DEBUG Feeds <%d>( %s )", len(Config.Feeds), strings.Join(feedsurls, SP))

	return nil
}

func TgGetUpdates() error {
	if time.Since(Config.TgGetUpdatesLast) < Config.TgGetUpdatesInterval {
		return nil
	}

	uu, _, err := tg.GetUpdates(Config.TgUpdatesOffset + 1)
	if err != nil {
		return fmt.Errorf("tg.GetUpdates %v", err)
	}
	Config.TgGetUpdatesLast = time.Now()

	for _, u := range uu {

		Config.TgUpdatesOffset = u.UpdateId

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
			continue
		}

		if tg.F("%d", m.Chat.Id) != Config.TgBossChatId {
			perr("Update <%d> not from TgBossChatId", u.UpdateId)
			continue
		}

		if tgerr := tg.SetMessageReaction(tg.SetMessageReactionRequest{
			ChatId:    fmt.Sprintf("%d", m.Chat.Id),
			MessageId: m.MessageId,
			Reaction:  []tg.ReactionTypeEmoji{tg.ReactionTypeEmoji{Emoji: "👾"}},
		}); tgerr != nil {
			perr("ERROR tg.SetMessageReaction %v", tgerr)
		}

		mtff := strings.Fields(m.Text)

		if (len(mtff) == 2 && mtff[0] == Config.TgCmdAdd && strings.HasPrefix(mtff[1], "https://")) || (len(mtff) == 1 && strings.HasPrefix(mtff[0], "https://")) {

			mtfu := mtff[len(mtff)-1]
			perr("ADD feed [%s]", mtfu)

			xmlfeed, err := FeedGet(mtfu)
			if err != nil {
				perr("FeedGet [%s] %v", mtfu, err)
				tgmsg := tg.Esc(tg.F("FeedGet %v", err))
				if _, tgerr := tg.SendMessage(tg.SendMessageRequest{
					ChatId:             fmt.Sprintf("%d", m.Chat.Id),
					ReplyToMessageId:   m.MessageId,
					Text:               tgmsg,
					LinkPreviewOptions: tg.LinkPreviewOptions{IsDisabled: true},
				}); tgerr != nil {
					perr("ERROR tg.SendMessage %v", tgerr)
				}
				continue
			}
			if len(xmlfeed.Entries) > 0 {
				err := XmlFeedEntryTgSend(xmlfeed, xmlfeed.Entries[len(xmlfeed.Entries)-1])
				if err != nil {
					tglog("XmlFeedEntryTgSend [%s] %v", mtfu, err)
					continue
				}
			}

			add := true
			for _, f := range Config.Feeds {
				if f.Url == mtfu {
					add = false
				}
			}
			if add {
				Config.Feeds = append(Config.Feeds, Feed{Url: mtfu, CheckLast: time.Now().Add(-FeedCheckLastAgoDefault)})
			}

			if tgerr := tg.SetMessageReaction(tg.SetMessageReactionRequest{
				ChatId:    fmt.Sprintf("%d", m.Chat.Id),
				MessageId: m.MessageId,
				Reaction:  []tg.ReactionTypeEmoji{tg.ReactionTypeEmoji{Emoji: "👍"}},
			}); tgerr != nil {
				perr("ERROR tg.SetMessageReaction %v", tgerr)
			}

		} else if len(mtff) == 2 && mtff[0] == Config.TgCmdRemove && strings.HasPrefix(mtff[1], "https://") {

			mtfu := mtff[len(mtff)-1]
			perr("REMOVE feed [%s]", mtfu)

			FeedsNew := make([]Feed, 0, len(Config.Feeds))
			for _, f := range Config.Feeds {
				if f.Url == mtfu {
					continue
				}
				FeedsNew = append(FeedsNew, f)
			}
			if len(FeedsNew) != len(Config.Feeds) {
				Config.Feeds = FeedsNew
			}

			if tgerr := tg.SetMessageReaction(tg.SetMessageReactionRequest{
				ChatId:    fmt.Sprintf("%d", m.Chat.Id),
				MessageId: m.MessageId,
				Reaction:  []tg.ReactionTypeEmoji{tg.ReactionTypeEmoji{Emoji: "👍"}},
			}); tgerr != nil {
				perr("ERROR tg.SetMessageReaction %v", tgerr)
			}

		} else if len(mtff) == 1 && mtff[0] == Config.TgCmdList {

			perr("LIST feeds")

			tgmsg := "(" + NL
			for i, f := range Config.Feeds {
				tgmsg += tg.F("#<%d> [%s]", i+1, f.Url) + NL
			}
			tgmsg += ")" + NL
			tgmsg = tg.Esc(tgmsg)
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
	}

	if err := Config.Put(); err != nil {
		return fmt.Errorf("Config.Put %v", err)
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
	var err error

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)
	go func(sigterm chan os.Signal) {
		<-sigterm
		tglog("%s sigterm", os.Args[0])
		os.Exit(1)
	}(sigterm)

	for {
		t0 := time.Now()

		if err = ConfigGet(); err != nil {
			tglog("ERROR ConfigGet %v", err)
			os.Exit(1)
		}

		if err = TgGetUpdates(); err != nil {
			tglog("ERROR TgGetUpdates %v", err)
		} else if err = AllFeedsTgSend(); err != nil {
			tglog("ERROR AllFeedsTgSend %v", err)
		}

		if dur := time.Now().Sub(t0); dur < Config.Interval {
			time.Sleep(Config.Interval - dur)
		}
	}
}

type XmlFeed struct {
	Updated XmlTime        `xml:"updated"`
	Title   string         `xml:"title"`
	Entries []XmlFeedEntry `xml:"entry"`
}

type XmlFeedEntry struct {
	Updated XmlTime          `xml:"updated"`
	Title   string           `xml:"title"`
	Link    XmlFeedEntryLink `xml:"link"`
}

type XmlFeedEntryLink struct {
	Href string `xml:"href,attr"`
}

type XmlTime struct {
	time.Time
}

func (t *XmlTime) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
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

func AllFeedsTgSend() error {
	for fi := range Config.Feeds {
		if time.Since(Config.Feeds[fi].CheckLast) < Config.FeedsCheckInterval {
			continue
		}
		err := FeedAllEntriesTgSend(Config.Feeds[fi])
		if err != nil {
			return fmt.Errorf("FeedCheck [%s] %v", Config.Feeds[fi].Url, err)
		}
		Config.Feeds[fi].CheckLast = time.Now()
	}

	Config.FeedsCheckLast = time.Now()
	if err := Config.Put(); err != nil {
		return fmt.Errorf("Config.Put %v", err)
	}

	return nil
}

func FeedGet(feedurl string) (xmlfeed *XmlFeed, err error) {
	perr("DEBUG FeedGet url [%s]", feedurl)

	resp, err := http.Get(feedurl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	decoder := xml.NewDecoder(resp.Body)
	decoder.DefaultSpace = Config.XmlDefaultSpace

	err = decoder.Decode(&xmlfeed)
	if err != nil {
		return nil, fmt.Errorf("xml decode %v", err)
	}
	if xmlfeed.Updated.IsZero() && xmlfeed.Title == "" {
		return nil, fmt.Errorf("feed title and time are empty")
	}

	xmlfeed.Title = strings.TrimSpace(xmlfeed.Title)
	for i := range xmlfeed.Entries {
		xmlfeed.Entries[i].Title = strings.TrimSpace(xmlfeed.Entries[i].Title)
	}

	sort.Slice(xmlfeed.Entries, func(i, j int) bool {
		return xmlfeed.Entries[i].Updated.Time.Before(xmlfeed.Entries[j].Updated.Time)
	})

	return xmlfeed, nil
}

func XmlFeedEntryTgSend(xmlfeed *XmlFeed, xmlfeedentry XmlFeedEntry) error {
	tgmsg := tg.Bold(tg.Link(
		fmt.Sprintf("%s • %s", xmlfeed.Title, xmlfeedentry.Updated.Time.In(TZIST).Format("Jan/2 15:04")),
		xmlfeedentry.Link.Href,
	)) + NL +
		tg.Esc(xmlfeedentry.Title)
	perr("DEBUG XmlFeedEntryTgSend tgmsg [%s]", tgmsg)

	if _, tgerr := tg.SendMessage(tg.SendMessageRequest{
		ChatId:             Config.TgChatId,
		Text:               tgmsg,
		LinkPreviewOptions: tg.LinkPreviewOptions{IsDisabled: true},
	}); tgerr != nil {
		return tgerr
	}

	return nil
}

func FeedAllEntriesTgSend(f Feed) error {
	xmlfeed, err := FeedGet(f.Url)
	if err != nil {
		return err
	}

	for _, xmlfeedentry := range xmlfeed.Entries {
		perr("DEBUG FeedAllEntriesTgSend url [%s] title [%s] updated <%s> link [%s]", f.Url, xmlfeedentry.Title, xmlfeedentry.Updated.Time, xmlfeedentry.Link.Href)

		if xmlfeedentry.Updated.Time.Before(f.CheckLast) {
			continue
		}

		err := XmlFeedEntryTgSend(xmlfeed, xmlfeedentry)
		if err != nil {
			return err
		}

		time.Sleep(Config.TgSendInterval)
	}

	return nil
}

func fmttime(t time.Time) string {
	return fmt.Sprintf(
		"<%03d:%02d%02d:%02d%02d%02dॐ>",
		t.Year()%1000, t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second(),
	)
}

func perr(msg string, args ...interface{}) {
	if strings.HasPrefix(msg, "DEBUG ") && !Config.DEBUG {
		return
	}
	ts := fmttime(time.Now().In(TZIST))
	msgtext := msg
	if len(args) > 0 {
		msgtext = tg.F(msg, args...)
	}
	fmt.Fprint(os.Stderr, ts+SP+msgtext+NL)
}

func tglog(msg string, args ...interface{}) (err error) {
	msgtext := msg
	if len(args) > 0 {
		msgtext = tg.F(msg, args...)
	}
	perr(msgtext)
	_, err = tg.SendMessage(tg.SendMessageRequest{
		ChatId: Config.TgBossChatId,
		Text:   tg.Esc(msgtext),

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

	//perr("DEBUG Config.Get %+v", config)

	return nil
}

func (config *TgFeedConfig) Put() error {
	//perr("DEBUG Config.Put %s %+v", config.YssUrl, config)

	rbb, err := yaml.MarshalWithOptions(config, yaml.JSON(), yaml.Flow(false))
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
