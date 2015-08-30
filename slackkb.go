package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	ZkbUrl            = "https://zkillboard.com/"
	ZkbDateFmt        = "200601021504"
	ZkbTsFormat       = "200601021504"
	ZkbKillTimeFormat = "2006-01-02 15:04:05"

	Testing    = flag.Bool("test", false, "output to stdout only")
	ConfigPath = flag.String("config", "config.json", "path to JSON config file")
	IgnorePath = flag.String("ignore", "", "path to newline delimited list of systems to ignore")
)

func main() {
	flag.Parse()

	log.Printf("starting up. config path: %s\nignore path: %s\ntesting? %t", *ConfigPath, *IgnorePath, *Testing)
	confbytes, err := ioutil.ReadFile(*ConfigPath)
	if err != nil {
		log.Fatalf("error reading config file: %v", err)
	}

	conf := &Config{}
	err = json.Unmarshal(confbytes, conf)
	if err != nil {
		log.Fatalf("error reading config JSON: %v", err)
	}

	err = conf.Validate()
	if err != nil {
		log.Fatalf("error validating config: %v", err)
	}

	firstrun := true
	kc := NewKillCache()
	ignored := readignored(*IgnorePath)
	for {
		if !firstrun {
			time.Sleep(5 * time.Minute)
		}

		now := time.Now().UTC()
		fetchbefore := now.Add(-time.Hour)
		url := conf.ZKillboardUrl + fmt.Sprintf("startTime/%s/", fetchbefore.Format(ZkbTsFormat))

		kills, err := getanddecode(url)
		if err != nil {
			log.Printf("error retrieving kills: %v", err)
		}

		ignorebefore := now.Add(-time.Hour * 2)
		filtered := []KillData{}
		for _, k := range kills {

			if ignorebefore.After(k.KillTime) {
				log.Printf("ignoring kill: now: %v, kill time: %v", now, k.KillTime)
				continue
			}
			if _, ok := ignored[k.SystemID]; ok {
				// we post things over 1bil isk even if the system is ignored, because
				// people should know
				if k.Value < 1000000000.0 {
					log.Printf("ignored system. KillID:%s SystemID:%s", k.KillID, k.SystemID)
					continue
				}
			}
			if kc.Check(k.KillID, now) {
				log.Printf("in cache: %s", k.KillID)
				continue
			}
			log.Printf("not in cache or ignored. KillID:%s  SystemID:%s", k.KillID, k.SystemID)
			filtered = append(filtered, k)
		}

		if !firstrun && len(filtered) > 0 {
			if *Testing {
				printoutput(filtered)
			} else {
				err = output(conf, filtered)
				if err != nil {
					log.Printf("error outputting kill: %v", err)
				}
			}
		}
		firstrun = false
		kc.Evict(now.Add(-2 * time.Hour))
	}
}

type Config struct {
	ZKillboardUrl string `json:"zkurl"`
	// Channel to post to
	Channel string `json:"channel"`
	// Slackbot POST URL from the Slack integration configuration
	SlackbotUrl string `json:"slackbot_url"`
	// constructed URL
	sburl string
}

func (c *Config) Validate() error {
	u, err := url.Parse(c.SlackbotUrl)
	if err != nil {
		return fmt.Errorf("could not parse provided url: %s err=%v", c.SlackbotUrl, err)
	}
	q, _ := url.ParseQuery(u.RawQuery)
	q.Set("channel", c.Channel)
	u.RawQuery = q.Encode()
	c.sburl = u.String()
	return nil
}

func (c *Config) PostURL() string {
	v := url.Values{"channel": []string{c.Channel}}
	return c.SlackbotUrl + "?" + v.Encode()
}

type KillData struct {
	KillID   string
	KillTime time.Time
	SystemID string
	Value    float64
}

func NewKillCache() *KillCache {
	return &KillCache{make(map[string]time.Time, 0)}
}

// KillCache keeps around the kills we have already seen so we don't double-post
// We want to pull kills for a window before "now", and we want to keep that window overlapped so we
// don't miss previous kills. This lets use do that.
type KillCache struct {
	cache map[string]time.Time
}

func (kc *KillCache) Check(key string, ts time.Time) bool {
	_, ok := kc.cache[key]
	if ok {
		return true
	}

	kc.cache[key] = ts
	return false
}

func (kc *KillCache) Evict(before time.Time) {
	del := []string{}
	for k, v := range kc.cache {
		if v.Before(before) {
			del = append(del, k)
		}
	}

	if len(del) > 0 {
		log.Printf("dropped from cache: %d", len(del))
	}
	for _, k := range del {
		delete(kc.cache, k)
	}
}

// readignored reads a newline delimited list of system IDs to ignore
// these must be the system IDs, *not* the system names
func readignored(path string) map[string]bool {
	m := make(map[string]bool)
	if path == "" {
		return m
	}
	read, err := os.Open("ignored.txt")
	if err != nil {
		log.Printf("could not read 'ignored.txt': %v", err)
		return m
	}
	scan := bufio.NewScanner(read)
	for scan.Scan() {
		t := strings.TrimSpace(scan.Text())
		if t != "" {
			m[t] = true
		}
	}

	log.Printf("read %d ignored entries", len(m))
	return m
}
func printoutput(kills []KillData) {
	for _, k := range kills {
		log.Printf("Received new kill: %s", k)
	}
}

func output(conf *Config, kills []KillData) error {
	for i := len(kills) - 1; i >= 0; i-- {
		if i != 0 {
			time.Sleep(500 * time.Millisecond)
		}
		kurl := ZkbUrl + fmt.Sprintf("kill/%s/", kills[i].KillID)
		log.Printf("new kill: %s", kurl)
		resp, err := http.Post(conf.sburl, "text/plain", bytes.NewBufferString(kurl))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		b, _ := ioutil.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			return fmt.Errorf("Error sending Slackbot command %d: %s",
				resp.StatusCode, string(b))
		} else {
			log.Printf("received 200 from Slack: %s", string(b))
		}
	}
	return nil
}

// getanddecode pulls the latest kills from Zkillboard
// this monstrosity is mostly born out of seeing weird data from the zkillboard API;
// it is incredibly permissive/defensive
func getanddecode(url string) ([]KillData, error) {
	log.Printf("retrieving %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("non-200 status code: %s - Body:\n%s", resp.Status, string(body))
	}

	killmaps := []map[string]interface{}{}
	err = json.NewDecoder(resp.Body).Decode(&killmaps)
	if err != nil {
		return nil, err
	}

	kills := []KillData{}
	for _, k := range killmaps {
		kd, err := parseone(k)
		if err != nil {
			log.Printf("error parsing kill value. err=%v, data=%v", err, k)
			continue
		}

		kills = append(kills, kd)
	}

	return kills, nil
}

// parseone attempts to extract the data from a single value
// returned in the zkb results
// the zkb data seems to be somewhat dynamic, so we try to be extra
// permissive here
func parseone(k map[string]interface{}) (KillData, error) {
	kd := KillData{}
	v, ok := k["killID"]
	if !ok {
		return kd, fmt.Errorf("contained no killID")
	}

	vs, ok := valtostring(v)
	if !ok {
		return kd, fmt.Errorf("could not convert id to string: %v", v)
	}

	kd.KillID = vs

	ktv, ok := k["killTime"]
	if !ok {
		return kd, fmt.Errorf("no time available")
	}

	kts, ok := valtostring(ktv)
	if !ok {
		return kd, fmt.Errorf("could not convert time to string: %v", ktv)
	}

	kt, err := time.Parse(ZkbKillTimeFormat, kts)
	if err != nil {
		return kd, fmt.Errorf("could not parse time: %s", kts)
	}
	kd.KillTime = kt

	ssv, ok := k["solarSystemID"]
	if ok {
		ssvs, ok := valtostring(ssv)
		if ok {
			kd.SystemID = ssvs
		}
	}

	sub, ok := k["zkb"]
	if !ok {
		// no sub-data available
		return kd, nil
	}

	// attempts to find totalValue and extract it if possible
	// not always avaialble, so bail if we can't
	subm, ok := sub.(map[string]interface{})
	if !ok {
		return kd, nil
	}

	killval, ok := subm["totalValue"]
	if !ok {
		return kd, nil
	}

	price, ok := killval.(float64)
	if ok {
		kd.Value = price
	} else {
		vals, ok := killval.(string)
		if ok {
			price, err := strconv.ParseFloat(vals, 64)
			if err != nil {
				log.Printf("error parsing price. kill: %s, value: %s", kd.KillID, vals)
			} else {
				kd.Value = float64(price)
			}
		} else {
			log.Printf("unknown type for price: %T - %v", killval, killval)
		}
	}
	return kd, nil
}

// valtostring attempts to convert whatever type was deserialzied into a string
// zkb data is flexible, so this needs to be
func valtostring(v interface{}) (string, bool) {
	switch tv := v.(type) {
	case string:
		return tv, true
	case int:
		return fmt.Sprintf("%d", tv), true
	case float64:
		return fmt.Sprintf("%d", int(tv)), true
	}
	log.Printf("Ignoring unknown type: %T", v)
	return "", false
}
