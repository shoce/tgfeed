/*

GoGet GoFmt GoBuildNull GoBuild

*/

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
	NL = "\n"
)

type TgFeedConfig struct {
	YssUrl string `yaml:"-"`

	DEBUG bool `yaml:"DEBUG"`

	Interval        time.Duration `yaml:"Interval"`
	MessageInterval time.Duration `yaml:"MessageInterval"`

	TgApiUrlBase string `yaml:"TgApiUrlBase"` // = "https://api.telegram.org"

	TgToken  string `yaml:"TgToken"`
	TgChatId string `yaml:"TgChatId"`

	XmlDefaultSpace string `yaml:"XmlDefaultSpace"` // = "http://www.w3.org/2005/Atom"

	FeedsCheckLast time.Time `yaml:"FeedsCheckLast"`

	FeedsUrls []string `yaml:"FeedsUrls"`
	// = https://github.com/golang/go/releases.atom
	// = https://gitea.com/gitea/helm-actions/atom/branch/main
	// = https://gitea.com/gitea/helm-actions.atom
}

var (
	Config TgFeedConfig

	TZIST = time.FixedZone("IST", 330*60)

	Ctx context.Context

	MessageIntervalDefault = 3 * time.Second

	HttpClient = &http.Client{}
)

func init() {
	Ctx = context.TODO()

	if s := os.Getenv("YssUrl"); s != "" {
		Config.YssUrl = s
	}
	if Config.YssUrl == "" {
		log("ERROR YssUrl empty")
		os.Exit(1)
	}

	if err := Config.Get(); err != nil {
		log("ERROR Config.Get %v", err)
		os.Exit(1)
	}

	if Config.DEBUG {
		log("DEBUG <true>")
	}

	log("Interval <%v>", Config.Interval)
	if Config.Interval == 0 {
		log("ERROR Interval <0>")
		os.Exit(1)
	}
	log("MessageInterval <%v>", Config.MessageInterval)
	if Config.MessageInterval == 0 {
		Config.MessageInterval = MessageIntervalDefault
		log("MessageInterval <%v>", Config.MessageInterval)
	}

	if Config.TgToken == "" {
		log("ERROR TgToken empty")
		os.Exit(1)
	}

	tg.ApiToken = Config.TgToken

	if Config.TgChatId == "" {
		log("ERROR TgChatId empty")
		os.Exit(1)
	}

	log("FeedsCheckLast <%v>", Config.FeedsCheckLast)

	log("FeedsUrls ( %s )", strings.Join(Config.FeedsUrls, " "))
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
	for _, feedurl := range Config.FeedsUrls {
		if Config.DEBUG {
			log("DEBUG url %s", feedurl)
		}

		resp, err := http.Get(feedurl)
		if err != nil {
			log("ERROR %s %v", feedurl, err)
			continue
		}
		defer resp.Body.Close()

		decoder := xml.NewDecoder(resp.Body)
		decoder.DefaultSpace = Config.XmlDefaultSpace

		var feed Feed
		if err := decoder.Decode(&feed); err != nil {
			log("ERROR %s xml decode %v", feedurl, err)
			continue
		}

		sort.Slice(feed.Entries, func(i, j int) bool {
			return feed.Entries[i].Updated.Time.Before(feed.Entries[j].Updated.Time)
		})

		for _, e := range feed.Entries {
			if Config.DEBUG {
				log("DEBUG url %s title [%s] updated <%s>", feedurl, e.Title, e.Updated.Time)
			}

			if e.Updated.Time.Before(Config.FeedsCheckLast) {
				continue
			}

			if _, tgerr := tg.SendMessage(tg.SendMessageRequest{
				ChatId: Config.TgChatId,
				Text: tg.Bold(tg.Link(
					fmt.Sprintf("%s • %s", feed.Title, e.Updated.Time.In(TZIST).Format("Jan/2 15:04")),
					e.Link.Href,
				)) + NL +
					tg.Esc(e.Title),

				LinkPreviewOptions: tg.LinkPreviewOptions{IsDisabled: true},
			}); tgerr != nil {
				return tgerr
			}

			time.Sleep(Config.MessageInterval)
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
		"%d%02d%02d:%02d%02d+",
		tnow.Year()%1000, tnow.Month(), tnow.Day(),
		tnow.Hour(), tnow.Minute(),
	)
}

func log(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, ts()+" "+msg+NL, args...)
}

func tglog(msg string, args ...interface{}) (err error) {
	log(msg, args...)
	_, err = tg.SendMessage(tg.SendMessageRequest{
		ChatId: Config.TgChatId,
		Text:   tg.Esc(msg, args...),

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
		//log("DEBUG Config.Get %+v", config)
	}

	return nil
}

func (config *TgFeedConfig) Put() error {
	if config.DEBUG {
		//log("DEBUG Config.Put %s %+v", config.YssUrl, config)
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
