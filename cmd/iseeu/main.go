/*
Copyright 2021, 2021 the ISeeU contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/NouamaneTazi/iseeu/internal/analyze"
	"github.com/NouamaneTazi/iseeu/internal/config"
	"github.com/NouamaneTazi/iseeu/internal/cui"
	"github.com/NouamaneTazi/iseeu/internal/inspect"
	"github.com/gizak/termui/v3"
)

// parseURL reassembles the URL into a valid URL string
func parseURL(uri string) string {
	if !strings.Contains(uri, "://") && !strings.HasPrefix(uri, "//") {
		uri = "//" + uri
	}

	url, err := url.Parse(uri)
	if err != nil {
		log.Panicf("could not parse url %q: %v", uri, err)
	}
	if url.Scheme == "" {
		url.Scheme = "http"
		if !strings.HasSuffix(url.Host, ":80") {
			url.Scheme += "s"
		}
	}

	return url.String()
}

// parse parses urls and validates command format
func parse() {
	flag.Parse()
	tail := flag.Args()
	if len(tail) > 0 && len(tail)%2 == 0 {
		for i := 0; i < len(tail); i += 2 {
			pollingInterval, err := strconv.Atoi(tail[i+1])
			if err != nil {
				fmt.Println("Error converting polling interval to int", err)
				os.Exit(2)
			}
			config.UrlsPollingsIntervals[parseURL(tail[i])] = time.Duration(pollingInterval) * time.Second
		}
	} else {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] URL1 POLLING_INTERVAL1 URL2 POLLING_INTERVAL2\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "OPTIONS:")
		flag.PrintDefaults() //TODO: better usage
		log.Fatal("Urls must be provided with their respective polling intervals.")
	}
}

func main() {

	// Parse urls and polling intervals and options
	flag.DurationVar(&config.ShortUIRefreshInterval, "sui", 2*time.Second, "Short refreshing UI interval (in seconds)")
	flag.DurationVar(&config.LongUIRefreshInterval, "lui", 10*time.Second, "Long refreshing UI interval (in seconds)")
	flag.DurationVar(&config.ShortStatsHistoryInterval, "sstats", 10*time.Second, "Short history interval (in minutes)")
	flag.DurationVar(&config.LongStatsHistoryInterval, "lstats", 60*time.Second, "Long history interval (in minutes)")
	parse()

	// Init the inspectors, where each inspector monitors a single URL
	inspectorsList := make([]*inspect.Inspector, 0, len(config.UrlsPollingsIntervals))
	for url, pollingInterval := range config.UrlsPollingsIntervals {
		inspector := inspect.NewInspector(url, inspect.IntervalInspection(pollingInterval, config.MaxHistoryPerURL))
		inspectorsList = append(inspectorsList, inspector)

		// Init website monitoring
		go inspector.StartLoop()
	}

	// Init UIData
	data := analyze.NewUIData(inspectorsList)

	// Start proper UI
	var ui cui.UI
	if err := ui.Init(); err != nil {
		// TODO: should i use log.Fatal
		log.Fatalf("Failed to start CLI %v", err)
	}
	defer ui.Close()

	// Ticker that refreshes UI
	shortTick := time.NewTicker(config.ShortUIRefreshInterval)

	var counter int
	uiEvents := termui.PollEvents()
	for {
		select {
		case <-shortTick.C:
			counter++
			lenRows := len(ui.Alerts.Rows)
			if counter%int(config.LongUIRefreshInterval/config.ShortUIRefreshInterval) != 0 {
				UpdateUI(ui, data, config.ShortUIRefreshInterval)
			} else {
				UpdateUI(ui, data, config.LongUIRefreshInterval)
			}
			if ui.Alerts.SelectedRow == lenRows-1 || counter < 2 {
				ui.Alerts.ScrollPageDown()
				termui.Render(ui.Alerts)
			}

		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				return
			case "j", "<Down>":
				ui.Alerts.ScrollDown()
			case "k", "<Up>":
				ui.Alerts.ScrollUp()
			case "<C-d>":
				ui.Alerts.ScrollHalfPageDown()
			case "<C-u>":
				ui.Alerts.ScrollHalfPageUp()
			case "<C-f>":
				ui.Alerts.ScrollPageDown()
			case "<C-b>":
				ui.Alerts.ScrollPageUp()
			case "<Home>":
				ui.Alerts.ScrollTop()
			case "G", "<End>":
				ui.Alerts.ScrollBottom()
			}

			termui.Render(ui.Alerts)
		}
	}

}

// UpdateUI collects data from inspectors and refreshes UI.
func UpdateUI(ui cui.UI, data *analyze.UIData, interval time.Duration) {
	data.UpdateData(interval)
	ui.Update(data, interval)
}
